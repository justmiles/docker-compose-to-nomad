package main

import (
	"bytes"
	"fmt"
	"strings"
	"syscall/js" // For WASM interaction

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

// --- Docker Compose Structures (Simplified) ---
type DockerCompose struct {
	Version  string             `yaml:"version"`
	Services map[string]Service `yaml:"services"`
	Volumes  map[string]any     `yaml:"volumes"`
}

type Service struct {
	Image       string            `yaml:"image"`
	Ports       []string          `yaml:"ports"`
	Environment map[string]string `yaml:"environment"`
	Volumes     []string          `yaml:"volumes"`
	Command     any               `yaml:"command"`
	Entrypoint  any               `yaml:"entrypoint"`
	Restart     string            `yaml:"restart"`
	Deploy      *Deploy           `yaml:"deploy"`
}

type Deploy struct {
	Replicas *int `yaml:"replicas"`
}

// Helper to create comment tokens (includes a newline after the comment line)
func createCommentTokens(commentText string) hclwrite.Tokens {
	return hclwrite.Tokens{
		{
			Type:  hclsyntax.TokenComment,
			Bytes: []byte("# " + commentText),
		},
		{
			Type:  hclsyntax.TokenNewline, // Newline after the comment text
			Bytes: []byte("\n"),
		},
	}
}

// --- Conversion Logic ---

func convertToNomadHCL(yamlInput string) (string, error) {
	var dc DockerCompose
	err := yaml.Unmarshal([]byte(yamlInput), &dc)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling YAML: %w", err)
	}

	if len(dc.Services) == 0 {
		return "", fmt.Errorf("no services found in Docker Compose file")
	}

	hclFile := hclwrite.NewEmptyFile()
	rootBody := hclFile.Body()

	jobName := "my-docker-compose-job"
	jobBlock := rootBody.AppendNewBlock("job", []string{jobName})
	jobBody := jobBlock.Body()

	dcStrings := []string{"dc1"}
	dcVals := make([]cty.Value, len(dcStrings))
	for i, s := range dcStrings {
		dcVals[i] = cty.StringVal(s)
	}
	jobBody.SetAttributeValue("datacenters", cty.ListVal(dcVals))
	jobBody.SetAttributeValue("type", cty.StringVal("service"))
	jobBody.AppendNewline()

	for serviceName, service := range dc.Services {
		groupBlock := jobBody.AppendNewBlock("group", []string{serviceName})
		groupBody := groupBlock.Body()

		if service.Deploy != nil && service.Deploy.Replicas != nil {
			groupBody.SetAttributeValue("count", cty.NumberIntVal(int64(*service.Deploy.Replicas)))
		} else {
			groupBody.SetAttributeValue("count", cty.NumberIntVal(int64(1)))
		}
		groupBody.AppendNewline()

		taskBlock := groupBody.AppendNewBlock("task", []string{serviceName})
		taskBody := taskBlock.Body()

		taskBody.SetAttributeValue("driver", cty.StringVal("docker"))
		taskBody.AppendNewline()

		configBlock := taskBody.AppendNewBlock("config", nil)
		configBody := configBlock.Body()
		configBody.SetAttributeValue("image", cty.StringVal(service.Image))

		var taskConfigPorts []cty.Value
		if len(service.Ports) > 0 {
			groupBody.AppendNewline()
			networkBlock := groupBody.AppendNewBlock("network", nil)
			networkBody := networkBlock.Body()
			for i, portMapping := range service.Ports {
				parts := strings.Split(portMapping, ":")
				containerPortStr := parts[len(parts)-1]
				hostPortStr := containerPortStr
				if len(parts) > 1 {
					hostPortStr = parts[0]
				}
				taskConfigPorts = append(taskConfigPorts, cty.StringVal(containerPortStr))
				portLabel := fmt.Sprintf("port%d_%s", i, containerPortStr)
				portBlockNt := networkBody.AppendNewBlock("port", []string{portLabel})
				portBodyNt := portBlockNt.Body()
				if hostPortStr != "" {
					parsedHostPort, err := parseInt64ForPort(hostPortStr)
					if err == nil {
						if hostPortStr != containerPortStr && len(parts) > 1 {
							portBodyNt.SetAttributeValue("to", cty.NumberIntVal(parsedHostPort))
						} else {
							portBodyNt.SetAttributeValue("static", cty.NumberIntVal(parsedHostPort))
						}
					} else {
						taskBody.AppendUnstructuredTokens(createCommentTokens(fmt.Sprintf("Could not parse host port '%s' for mapping '%s'.", hostPortStr, portMapping)))
					}
				}
				if i < len(service.Ports)-1 {
					networkBody.AppendNewline()
				}
			}
		}
		if len(taskConfigPorts) > 0 {
			configBody.SetAttributeValue("ports", cty.ListVal(taskConfigPorts))
		}

		// --- Volume Handling ---
		var dockerDriverVolumes []cty.Value
		hasAnyVolumesInSection := false

		if len(service.Volumes) > 0 {
			for _, volSpec := range service.Volumes {
				if !hasAnyVolumesInSection {
					// Only add newline before the first volume-related item (comment, block, or attribute)
					if len(configBlock.Body().Attributes()) > 1 || len(taskConfigPorts) > 0 { // if config already has ports or other attrs
						taskBody.AppendNewline()
					}
					hasAnyVolumesInSection = true
				}
				parts := strings.SplitN(volSpec, ":", 3)
				if len(parts) < 1 {
					taskBody.AppendUnstructuredTokens(createCommentTokens(fmt.Sprintf("Skipping invalid volume spec: %s.", volSpec)))
					continue
				}

				source := parts[0]
				var destination string
				options := ""

				if len(parts) == 1 {
					if strings.Contains(source, "/") && !strings.HasPrefix(source, "./") {
						taskBody.AppendUnstructuredTokens(createCommentTokens(fmt.Sprintf("Anonymous volume '%s' needs mapping to a host path or named Nomad volume.", source)))
						continue
					}
					destination = source
				} else {
					destination = parts[1]
					if len(parts) == 3 {
						options = parts[2]
					}
				}
				isReadOnly := strings.Contains(options, "ro")

				if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "/") {
					if strings.HasPrefix(source, "./") {
						taskBody.AppendUnstructuredTokens(createCommentTokens(fmt.Sprintf("Mapping relative host path '%s'. In Nomad, this is relative to task alloc dir.", source)))
					}
					volumeString := fmt.Sprintf("%s:%s", source, destination)
					if isReadOnly {
						volumeString += ":ro"
					}
					dockerDriverVolumes = append(dockerDriverVolumes, cty.StringVal(volumeString))
				} else {
					// Named volume for task.volume_mount
					commentText := fmt.Sprintf("Ensure Nomad volume '%s' is defined in the job or cluster.", source)
					taskBody.AppendUnstructuredTokens(createCommentTokens(commentText)) // Append comment first

					vmBlock := taskBody.AppendNewBlock("volume_mount", nil) // Then append the block
					vmBody := vmBlock.Body()
					vmBody.SetAttributeValue("volume", cty.StringVal(source))
					vmBody.SetAttributeValue("destination", cty.StringVal(destination))
					vmBody.SetAttributeValue("read_only", cty.BoolVal(isReadOnly))
					taskBody.AppendNewline() // After each volume_mount block
				}
			}
		}
		if len(dockerDriverVolumes) > 0 {
			configBody.SetAttributeValue("volumes", cty.ListVal(dockerDriverVolumes))
		}

		// Newline after the entire config block content only if it had more than just 'image' or if volumes were processed
		if len(configBlock.Body().Attributes()) > 1 || hasAnyVolumesInSection { // >1 because 'image' is always there
			taskBody.AppendNewline()
		}

		// Env block
		if len(service.Environment) > 0 {
			envBlock := taskBody.AppendNewBlock("env", nil)
			envBody := envBlock.Body()
			for key, val := range service.Environment {
				envBody.SetAttributeValue(key, cty.StringVal(val))
			}
			taskBody.AppendNewline()
		}

		// Command/Entrypoint (part of config block)
		var cmdVal cty.Value
		var argsVal cty.Value
		var entrypointParts []string
		if service.Entrypoint != nil {
			switch e := service.Entrypoint.(type) {
			case string:
				entrypointParts = []string{e}
			case []any:
				for _, item := range e {
					if s, ok := item.(string); ok {
						entrypointParts = append(entrypointParts, s)
					}
				}
			}
		}
		var commandParts []string
		if service.Command != nil {
			switch c := service.Command.(type) {
			case string:
				commandParts = strings.Fields(c)
			case []any:
				for _, item := range c {
					if s, ok := item.(string); ok {
						commandParts = append(commandParts, s)
					}
				}
			}
		}
		if len(entrypointParts) > 0 {
			cmdVal = cty.StringVal(entrypointParts[0])
			var finalArgs []string
			if len(entrypointParts) > 1 {
				finalArgs = append(finalArgs, entrypointParts[1:]...)
			}
			finalArgs = append(finalArgs, commandParts...)
			if len(finalArgs) > 0 {
				argCtyVals := make([]cty.Value, len(finalArgs))
				for i, arg := range finalArgs {
					argCtyVals[i] = cty.StringVal(arg)
				}
				argsVal = cty.ListVal(argCtyVals)
			}
		} else if len(commandParts) > 0 {
			cmdVal = cty.StringVal(commandParts[0])
			if len(commandParts) > 1 {
				argCtyVals := make([]cty.Value, len(commandParts)-1)
				for i, arg := range commandParts[1:] {
					argCtyVals[i] = cty.StringVal(arg)
				}
				argsVal = cty.ListVal(argCtyVals)
			}
		}
		if !cmdVal.IsNull() && cmdVal.IsKnown() {
			configBody.SetAttributeValue("command", cmdVal)
		}
		if !argsVal.IsNull() && argsVal.IsKnown() {
			configBody.SetAttributeValue("args", argsVal)
		}

		// Restart block
		if service.Restart != "" {
			// Ensure a newline before restart if previous blocks exist
			if len(service.Environment) > 0 || hasAnyVolumesInSection || len(configBlock.Body().Attributes()) > 1 {
				taskBody.AppendNewline()
			}
			restartBlock := taskBody.AppendNewBlock("restart", nil)
			restartBody := restartBlock.Body()
			switch service.Restart {
			case "always", "unless-stopped":
				restartBody.SetAttributeValue("attempts", cty.NumberIntVal(0))
				restartBody.SetAttributeValue("delay", cty.StringVal("15s"))
				restartBody.SetAttributeValue("mode", cty.StringVal("delay"))
			case "on-failure":
				restartBody.SetAttributeValue("attempts", cty.NumberIntVal(3))
				restartBody.SetAttributeValue("interval", cty.StringVal("1m"))
				restartBody.SetAttributeValue("mode", cty.StringVal("fail"))
			case "no":
				restartBody.SetAttributeValue("attempts", cty.NumberIntVal(0))
				restartBody.SetAttributeValue("mode", cty.StringVal("fail"))
			}
			taskBody.AppendNewline()
		}
		jobBody.AppendNewline()
	}

	formattedBytes := hclwrite.Format(hclFile.Bytes())
	var buf bytes.Buffer
	_, err = buf.Write(formattedBytes)
	if err != nil {
		return "", fmt.Errorf("error writing HCL: %w", err)
	}
	return buf.String(), nil
}

func parseInt64ForPort(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("port string is empty")
	}
	var i int64
	_, err := fmt.Sscan(strings.TrimSpace(s), &i)
	if err != nil {
		return 0, fmt.Errorf("could not parse port '%s' to int64: %w", s, err)
	}
	return i, nil
}

// --- WASM Export & main() --- (unchanged)
//
//export convert
func convert(this js.Value, args []js.Value) interface{} {
	if len(args) != 1 {
		errorConstructor := js.Global().Get("Error")
		return errorConstructor.New("Invalid number of arguments. Expected 1 (yamlInput string).")
	}
	yamlInput := args[0].String()
	handler := js.FuncOf(func(this js.Value, pArgs []js.Value) interface{} {
		resolve := pArgs[0]
		reject := pArgs[1]
		go func() {
			hclOutput, err := convertToNomadHCL(yamlInput)
			if err != nil {
				errorConstructor := js.Global().Get("Error")
				errorObject := errorConstructor.New(err.Error())
				reject.Invoke(errorObject)
			} else {
				resolve.Invoke(hclOutput)
			}
		}()
		return nil
	})
	promiseConstructor := js.Global().Get("Promise")
	return promiseConstructor.New(handler)
}

func main() {
	c := make(chan struct{}, 0)
	fmt.Println("Go WASM Initialized for Docker Compose to Nomad HCL Conversion")
	js.Global().Set("golangConvertToNomad", js.FuncOf(convert))
	<-c
}

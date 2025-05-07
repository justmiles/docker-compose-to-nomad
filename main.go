package main

import (
	"bytes"
	"fmt"
	"regexp" // Added for label sanitization
	"strings"
	"syscall/js"

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

// --- Helper Types and Functions for Port Processing ---

type ProcessedPortInfo struct {
	OriginalHostPort      string // The host port string as parsed (can be empty)
	OriginalContainerPort string // The container port string as parsed
	Comment               string // Raw comment text, if any
	ProtocolStrippedPort  string // Container port number after stripping /tcp or /udp, used for consolidation key
}

var nonAlphanumericUnderscoreRegex = regexp.MustCompile(`[^a-z0-9_]+`)

func sanitizeCommentToLabel(comment string) string {
	if comment == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(comment))
	// Replace spaces and hyphens with underscores first
	intermediate := strings.ReplaceAll(lower, " ", "_")
	intermediate = strings.ReplaceAll(intermediate, "-", "_")
	// Remove all other non-alphanumeric characters (keeps underscores)
	sanitized := nonAlphanumericUnderscoreRegex.ReplaceAllString(intermediate, "")
	// Remove leading/trailing underscores that might result
	sanitized = strings.Trim(sanitized, "_")
	// Prevent multiple underscores together if they form due to replacements
	if strings.Contains(sanitized, "__") { // Only compile if needed
		sanitized = regexp.MustCompile(`_+`).ReplaceAllString(sanitized, "_")
	}
	return sanitized
}

var wellKnownPorts = map[string]string{
	"80":   "http",
	"443":  "https",
	"21":   "ftp",
	"22":   "ssh",
	"23":   "telnet",
	"25":   "smtp",
	"53":   "dns",
	"110":  "pop3",
	"143":  "imap",
	"3306": "mysql",
	"5432": "postgresql",
}

func getWellKnownPortLabel(portStr string) string {
	// portStr is assumed to be just the number, already stripped of /tcp, /udp
	return wellKnownPorts[portStr]
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

		var generatedPortLabels []string // For task.config.ports

		if len(service.Ports) > 0 {
			groupBody.AppendNewline() // Newline before the network block
			networkBlock := groupBody.AppendNewBlock("network", nil)
			networkBody := networkBlock.Body()

			// Step 1: Parse all port definitions, extract comments, strip protocols for consolidation key
			parsedPortInfos := []ProcessedPortInfo{}
			for _, portMappingWithComment := range service.Ports {
				portSpecPart := portMappingWithComment
				comment := ""

				// Check if there's a comment in the port mapping
				if strings.Contains(portMappingWithComment, "#") {
					parts := strings.SplitN(portMappingWithComment, "#", 2)
					portSpecPart = strings.TrimSpace(parts[0])
					if len(parts) > 1 {
						comment = strings.TrimSpace(parts[1])
					}
				}

				// Strip protocol suffix (/tcp or /udp) for port consolidation
				containerPortWithoutProtocol := portSpecPart
				if strings.Contains(portSpecPart, "/") {
					containerPortWithoutProtocol = strings.Split(portSpecPart, "/")[0]
				}

				hostPortStr := ""
				containerPortStr := "" // This will be the key for consolidation

				if strings.Contains(containerPortWithoutProtocol, ":") {
					parts := strings.SplitN(containerPortWithoutProtocol, ":", 2)
					hostPortStr = parts[0]
					containerPortStr = parts[1]
				} else {
					containerPortStr = containerPortWithoutProtocol // If no ':', it's the container port (for 'to')
				}

				if containerPortStr == "" {
					taskBody.AppendUnstructuredTokens(createCommentTokens(fmt.Sprintf("Skipping invalid port spec: %s", portMappingWithComment)))
					continue
				}

				// For consolidation, we need to strip any protocol suffix from the container port
				strippedContainerPort := containerPortStr
				if strings.Contains(containerPortStr, "/") {
					strippedContainerPort = strings.Split(containerPortStr, "/")[0]
				}

				parsedPortInfos = append(parsedPortInfos, ProcessedPortInfo{
					OriginalHostPort:      hostPortStr,      // Will be parsed later for 'static'
					OriginalContainerPort: containerPortStr, // Will be parsed later for 'to' or if host is also container
					Comment:               comment,
					ProtocolStrippedPort:  strippedContainerPort, // Key for consolidation
				})
			}

			// Step 2: Consolidate ports based on the (protocol-stripped) container port number
			consolidatedPortMap := make(map[string]ProcessedPortInfo) // Key: ProtocolStrippedPort
			for _, pInfo := range parsedPortInfos {
				if _, exists := consolidatedPortMap[pInfo.ProtocolStrippedPort]; !exists {
					consolidatedPortMap[pInfo.ProtocolStrippedPort] = pInfo // First one wins (for comment)
				}
			}

			// Step 3: Generate HCL for consolidated ports
			isFirstPortInBlock := true
			// To ensure somewhat stable output order, we can sort keys, but map iteration is fine for now
			for _, finalPInfo := range consolidatedPortMap {
				var portLabel string
				sanitizedComment := sanitizeCommentToLabel(finalPInfo.Comment)
				if sanitizedComment != "" {
					portLabel = sanitizedComment
				} else {
					wellKnownLabel := getWellKnownPortLabel(finalPInfo.ProtocolStrippedPort)
					if wellKnownLabel != "" {
						portLabel = wellKnownLabel
					} else {
						portLabel = "port_" + finalPInfo.ProtocolStrippedPort
					}
				}

				if !isFirstPortInBlock {
					networkBody.AppendNewline()
				}
				isFirstPortInBlock = false

				nomadPortBlock := networkBody.AppendNewBlock("port", []string{portLabel})
				nomadPortBody := nomadPortBlock.Body()

				if finalPInfo.OriginalHostPort != "" { // Case: "host:container" -> static = host, to = container
					hostPortVal, err := parseInt64ForPort(finalPInfo.OriginalHostPort)
					if err != nil {
						errMsg := fmt.Sprintf("Error parsing host port '%s' for label '%s': %s", finalPInfo.OriginalHostPort, portLabel, err.Error())
						taskBody.AppendUnstructuredTokens(createCommentTokens(errMsg))
						continue // Skip this port block
					}

					containerPortVal, err := parseInt64ForPort(finalPInfo.OriginalContainerPort)
					if err != nil {
						errMsg := fmt.Sprintf("Error parsing container port '%s' for label '%s': %s", finalPInfo.OriginalContainerPort, portLabel, err.Error())
						taskBody.AppendUnstructuredTokens(createCommentTokens(errMsg))
						continue // Skip this port block
					}

					// Set static to the host port
					nomadPortBody.SetAttributeValue("static", cty.NumberIntVal(hostPortVal))

					// If host port and container port are different, also set 'to' for the container port
					if hostPortVal != containerPortVal {
						nomadPortBody.SetAttributeValue("to", cty.NumberIntVal(containerPortVal))
					}

				} else { // Case: "container" -> to = container
					containerPortVal, err := parseInt64ForPort(finalPInfo.ProtocolStrippedPort) // Use the clean one
					if err != nil {
						errMsg := fmt.Sprintf("Error parsing container port '%s' for label '%s': %s", finalPInfo.ProtocolStrippedPort, portLabel, err.Error())
						taskBody.AppendUnstructuredTokens(createCommentTokens(errMsg))
						continue // Skip this port block
					}
					nomadPortBody.SetAttributeValue("to", cty.NumberIntVal(containerPortVal))
				}
				generatedPortLabels = append(generatedPortLabels, portLabel)
			}
		}

		// Update task.config.ports with generated labels (this is correct for Nomad)
		if len(generatedPortLabels) > 0 {
			configBody := configBlock.Body() // configBlock is defined at line 97
			portLabelCtyValues := make([]cty.Value, len(generatedPortLabels))
			for i, label := range generatedPortLabels {
				portLabelCtyValues[i] = cty.StringVal(label)
			}
			configBody.SetAttributeValue("ports", cty.ListVal(portLabelCtyValues))
		}

		// --- Volume Handling ---
		var dockerDriverVolumes []cty.Value
		hasAnyVolumesInSection := false

		if len(service.Volumes) > 0 {
			for _, volSpec := range service.Volumes {
				if !hasAnyVolumesInSection {
					// Only add newline before the first volume-related item (comment, block, or attribute)
					if len(configBlock.Body().Attributes()) > 1 || len(generatedPortLabels) > 0 { // if config already has ports or other attrs
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

// export convert
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

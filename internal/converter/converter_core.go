package converter

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"

	"github.com/justmiles/docker-compose-to-nomad/internal/dockercompose"
	"github.com/justmiles/docker-compose-to-nomad/internal/portutils"
)

// ConvertToNomadHCL converts a Docker Compose YAML string to Nomad HCL string.
// This is the core logic, shared between WASM and native builds.
func ConvertToNomadHCL(yamlInput string) (string, error) {
	var dc dockercompose.DockerCompose
	err := yaml.Unmarshal([]byte(yamlInput), &dc)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling YAML: %w", err)
	}

	if len(dc.Services) == 0 {
		return "", fmt.Errorf("no services found in Docker Compose file")
	}

	hclFile := hclwrite.NewEmptyFile()
	rootBody := hclFile.Body()

	jobName := "my-docker-compose-job" // Consider making this configurable or derived
	jobBlock := rootBody.AppendNewBlock("job", []string{jobName})
	jobBody := jobBlock.Body()

	// Default datacenters, can be customized
	dcStrings := []string{"dc1"}
	dcVals := make([]cty.Value, len(dcStrings))
	for i, s := range dcStrings {
		dcVals[i] = cty.StringVal(s)
	}
	jobBody.SetAttributeValue("datacenters", cty.ListVal(dcVals))
	jobBody.SetAttributeValue("type", cty.StringVal("service")) // Default job type
	jobBody.AppendNewline()

	for serviceName, service := range dc.Services {
		groupBlock := jobBody.AppendNewBlock("group", []string{serviceName})
		groupBody := groupBlock.Body()

		if service.Deploy != nil && service.Deploy.Replicas != nil {
			groupBody.SetAttributeValue("count", cty.NumberIntVal(int64(*service.Deploy.Replicas)))
		} else {
			groupBody.SetAttributeValue("count", cty.NumberIntVal(int64(1))) // Default to 1 replica
		}
		groupBody.AppendNewline()

		taskBlock := groupBody.AppendNewBlock("task", []string{serviceName})
		taskBody := taskBlock.Body()

		taskBody.SetAttributeValue("driver", cty.StringVal("docker"))
		taskBody.AppendNewline()

		configBlock := taskBody.AppendNewBlock("config", nil)
		configBody := configBlock.Body()
		configBody.SetAttributeValue("image", cty.StringVal(service.Image))

		var generatedPortLabels []string

		if len(service.Ports) > 0 {
			groupBody.AppendNewline()
			networkBlock := groupBody.AppendNewBlock("network", nil)
			networkBody := networkBlock.Body()

			parsedPortInfos := []portutils.ProcessedPortInfo{}
			for _, portMappingWithComment := range service.Ports {
				portSpecPart := portMappingWithComment
				comment := ""

				if strings.Contains(portMappingWithComment, "#") {
					parts := strings.SplitN(portMappingWithComment, "#", 2)
					portSpecPart = strings.TrimSpace(parts[0])
					if len(parts) > 1 {
						comment = strings.TrimSpace(parts[1])
					}
				}

				containerPortWithoutProtocol := portSpecPart
				if strings.Contains(portSpecPart, "/") {
					containerPortWithoutProtocol = strings.Split(portSpecPart, "/")[0]
				}

				hostPortStr := ""
				containerPortStr := ""

				if strings.Contains(containerPortWithoutProtocol, ":") {
					parts := strings.SplitN(containerPortWithoutProtocol, ":", 2)
					hostPortStr = parts[0]
					containerPortStr = parts[1]
				} else {
					containerPortStr = containerPortWithoutProtocol
				}

				if containerPortStr == "" {
					taskBody.AppendUnstructuredTokens(portutils.CreateCommentTokens(fmt.Sprintf("Skipping invalid port spec: %s", portMappingWithComment)))
					continue
				}

				strippedContainerPort := containerPortStr
				if strings.Contains(containerPortStr, "/") {
					strippedContainerPort = strings.Split(containerPortStr, "/")[0]
				}

				parsedPortInfos = append(parsedPortInfos, portutils.ProcessedPortInfo{
					OriginalHostPort:      hostPortStr,
					OriginalContainerPort: containerPortStr,
					Comment:               comment,
					ProtocolStrippedPort:  strippedContainerPort,
				})
			}

			consolidatedPortMap := make(map[string]portutils.ProcessedPortInfo)
			for _, pInfo := range parsedPortInfos {
				if _, exists := consolidatedPortMap[pInfo.ProtocolStrippedPort]; !exists {
					consolidatedPortMap[pInfo.ProtocolStrippedPort] = pInfo
				}
			}

			isFirstPortInBlock := true
			for _, finalPInfo := range consolidatedPortMap {
				var portLabel string
				sanitizedComment := portutils.SanitizeCommentToLabel(finalPInfo.Comment)
				if sanitizedComment != "" {
					portLabel = sanitizedComment
				} else {
					wellKnownLabel := portutils.GetWellKnownPortLabel(finalPInfo.ProtocolStrippedPort)
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

				if finalPInfo.OriginalHostPort != "" {
					hostPortVal, err := portutils.ParseInt64ForPort(finalPInfo.OriginalHostPort)
					if err != nil {
						errMsg := fmt.Sprintf("Error parsing host port '%s' for label '%s': %s", finalPInfo.OriginalHostPort, portLabel, err.Error())
						taskBody.AppendUnstructuredTokens(portutils.CreateCommentTokens(errMsg))
						continue
					}

					containerPortVal, err := portutils.ParseInt64ForPort(finalPInfo.OriginalContainerPort)
					if err != nil {
						errMsg := fmt.Sprintf("Error parsing container port '%s' for label '%s': %s", finalPInfo.OriginalContainerPort, portLabel, err.Error())
						taskBody.AppendUnstructuredTokens(portutils.CreateCommentTokens(errMsg))
						continue
					}

					nomadPortBody.SetAttributeValue("static", cty.NumberIntVal(hostPortVal))
					if hostPortVal != containerPortVal {
						nomadPortBody.SetAttributeValue("to", cty.NumberIntVal(containerPortVal))
					}
				} else {
					containerPortVal, err := portutils.ParseInt64ForPort(finalPInfo.ProtocolStrippedPort)
					if err != nil {
						errMsg := fmt.Sprintf("Error parsing container port '%s' for label '%s': %s", finalPInfo.ProtocolStrippedPort, portLabel, err.Error())
						taskBody.AppendUnstructuredTokens(portutils.CreateCommentTokens(errMsg))
						continue
					}
					nomadPortBody.SetAttributeValue("to", cty.NumberIntVal(containerPortVal))
				}
				generatedPortLabels = append(generatedPortLabels, portLabel)
			}
		}

		if len(generatedPortLabels) > 0 {
			portLabelCtyValues := make([]cty.Value, len(generatedPortLabels))
			for i, label := range generatedPortLabels {
				portLabelCtyValues[i] = cty.StringVal(label)
			}
			configBody.SetAttributeValue("ports", cty.ListVal(portLabelCtyValues))
		}

		var dockerDriverVolumes []cty.Value
		hasAnyVolumesInSection := false

		if len(service.Volumes) > 0 {
			for _, volSpec := range service.Volumes {
				if !hasAnyVolumesInSection {
					if len(configBlock.Body().Attributes()) > 1 || len(generatedPortLabels) > 0 {
						taskBody.AppendNewline()
					}
					hasAnyVolumesInSection = true
				}
				parts := strings.SplitN(volSpec, ":", 3)
				if len(parts) < 1 {
					taskBody.AppendUnstructuredTokens(portutils.CreateCommentTokens(fmt.Sprintf("Skipping invalid volume spec: %s.", volSpec)))
					continue
				}

				source := parts[0]
				var destination string
				options := ""

				if len(parts) == 1 {
					if strings.Contains(source, "/") && !strings.HasPrefix(source, "./") {
						taskBody.AppendUnstructuredTokens(portutils.CreateCommentTokens(fmt.Sprintf("Anonymous volume '%s' needs mapping to a host path or named Nomad volume.", source)))
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
						taskBody.AppendUnstructuredTokens(portutils.CreateCommentTokens(fmt.Sprintf("Mapping relative host path '%s'. In Nomad, this is relative to task alloc dir.", source)))
					}
					volumeString := fmt.Sprintf("%s:%s", source, destination)
					if isReadOnly {
						volumeString += ":ro"
					}
					dockerDriverVolumes = append(dockerDriverVolumes, cty.StringVal(volumeString))
				} else {
					commentText := fmt.Sprintf("Ensure Nomad volume '%s' is defined in the job or cluster.", source)
					taskBody.AppendUnstructuredTokens(portutils.CreateCommentTokens(commentText))

					vmBlock := taskBody.AppendNewBlock("volume_mount", nil)
					vmBody := vmBlock.Body()
					vmBody.SetAttributeValue("volume", cty.StringVal(source))
					vmBody.SetAttributeValue("destination", cty.StringVal(destination))
					vmBody.SetAttributeValue("read_only", cty.BoolVal(isReadOnly))
					taskBody.AppendNewline()
				}
			}
		}
		if len(dockerDriverVolumes) > 0 {
			configBody.SetAttributeValue("volumes", cty.ListVal(dockerDriverVolumes))
		}

		if len(configBlock.Body().Attributes()) > 1 || hasAnyVolumesInSection {
			taskBody.AppendNewline()
		}

		envVarsToAdd := make(map[string]string)
		if service.Environment != nil {
			switch envTyped := service.Environment.(type) {
			case map[string]string: // Handles environments explicitly defined as map[string]string
				for key, val := range envTyped {
					envVarsToAdd[key] = val
				}
			case map[any]any: // Handles environments unmarshalled as map[interface{}]interface{} by go-yaml
				for k, v := range envTyped {
					keyStr, keyOk := k.(string)
					if !keyOk {
						// Silently ignore non-string keys
						continue
					}

					var valStr string
					if v == nil { // Handles VAR: (null value in YAML)
						valStr = ""
					} else {
						vStr, valIsString := v.(string)
						if !valIsString {
							// Attempt to convert to string if it's not nil and not a string (e.g. int, bool)
							// For Nomad env vars, values are ultimately strings.
							valStr = fmt.Sprintf("%v", v)
						} else {
							valStr = vStr
						}
					}
					envVarsToAdd[keyStr] = valStr
				}
			case map[string]any: // Handles environments unmarshalled as map[string]interface{}
				for key, v := range envTyped {
					var valStr string
					if v == nil {
						valStr = ""
					} else {
						vStr, valIsString := v.(string)
						if !valIsString {
							valStr = fmt.Sprintf("%v", v)
						} else {
							valStr = vStr
						}
					}
					envVarsToAdd[key] = valStr
				}
			case []any: // Likely []interface{} from YAML unmarshal for list format
				for _, item := range envTyped {
					if s, ok := item.(string); ok {
						parts := strings.SplitN(s, "=", 2)
						if len(parts) == 2 {
							envVarsToAdd[parts[0]] = parts[1]
						} else if len(parts) == 1 { // Handle VAR (no =VALUE)
							// Docker Compose would take this from the shell.
							// Nomad env block sets vars directly. Set to empty string.
							envVarsToAdd[parts[0]] = ""
						}
						// Silently ignore malformed entries like "=VAL" or "KEY=VAL=EXTRA" (SplitN handles this well for KEY=VAL)
					}
				}
				// Note: `case []string:` is covered by `[]any` due to how `yaml.Unmarshal` works with `any`.
			}
		}

		if len(envVarsToAdd) > 0 {
			envBlock := taskBody.AppendNewBlock("env", nil)
			envBody := envBlock.Body()
			// Sort keys for consistent output (optional, but good for testability)
			// For now, iterate directly as order isn't strictly critical for functionality
			for key, val := range envVarsToAdd {
				envBody.SetAttributeValue(key, cty.StringVal(val))
			}
			taskBody.AppendNewline()
		}

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

		if service.Restart != "" {
			if len(envVarsToAdd) > 0 || hasAnyVolumesInSection || len(configBlock.Body().Attributes()) > 1 {
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
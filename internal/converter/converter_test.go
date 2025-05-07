//go:build !js

package converter_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/justmiles/docker-compose-to-nomad/internal/converter"
)

const sampleDockerComposeYAML = `
version: '3.8'
services:
  web:
    image: nginx:latest
    ports:
      - "80:80" # HTTP
      - "443:443" # HTTPS
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - logs:/var/log/nginx
    environment:
      NGINX_HOST: example.com
    restart: unless-stopped
    deploy:
      replicas: 2
  api:
    image: myapi:1.0
    ports:
      - "3000" # API port
    command: ["/app/start", "--port", "3000"]
    restart: "on-failure"
volumes:
  logs:
`

func TestConvertToNomadHCL_Basic(t *testing.T) {
	hclOutput, err := converter.ConvertToNomadHCL(sampleDockerComposeYAML)
	if err != nil {
		t.Fatalf("ConvertToNomadHCL failed: %v", err)
	}

	fmt.Println("--- Generated Nomad HCL (Test) ---")
	fmt.Println(hclOutput)
	fmt.Println("----------------------------------")

	if hclOutput == "" {
		t.Errorf("Expected HCL output, but got empty string")
	}

	// Add more specific assertions based on expected HCL structure
	if !strings.Contains(hclOutput, "job \"my-docker-compose-job\"") {
		t.Errorf("HCL output does not contain job block")
	}
	if !strings.Contains(hclOutput, "group \"web\"") {
		t.Errorf("HCL output does not contain group block for 'web' service")
	}
	if !strings.Contains(hclOutput, "count = 2") {
		t.Errorf("HCL output does not contain correct replica count for 'web' service")
	}
	if !regexp.MustCompile(`image\s*=\s*"nginx:latest"`).MatchString(hclOutput) {
		t.Errorf("HCL output does not contain correct image for 'web' service (image = \"nginx:latest\")")
	}
	if !strings.Contains(hclOutput, "port \"http\"") {
		t.Errorf("HCL output does not contain http port for 'web' service")
	}
    if !strings.Contains(hclOutput, "static = 80") {
        t.Errorf("HCL output does not contain static port 80 for 'web' service's http port")
    }
	if !strings.Contains(hclOutput, "port \"https\"") {
		t.Errorf("HCL output does not contain https port for 'web' service")
	}
    if !strings.Contains(hclOutput, "static = 443") {
        t.Errorf("HCL output does not contain static port 443 for 'web' service's https port")
    }
	if !strings.Contains(hclOutput, "volumes = [\"./nginx.conf:/etc/nginx/nginx.conf:ro\"]") {
		t.Errorf("HCL output does not contain host volume for 'web' service")
	}
	// Check for web service's named volume mount for 'logs'
	// This regex looks for group "web", then a volume_mount block within it containing volume = "logs"
	webLogsVolumePattern := `group "web"[\s\S]*?volume_mount\s*{[\s\S]*?volume\s*=\s*"logs"[\s\S]*?destination\s*=\s*"/var/log/nginx"[\s\S]*?read_only\s*=\s*false`
	if !regexp.MustCompile(webLogsVolumePattern).MatchString(hclOutput) {
		t.Errorf("HCL output does not contain correctly configured named volume_mount for 'logs' in service 'web'")
	}
	if !strings.Contains(hclOutput, "NGINX_HOST = \"example.com\"") {
		t.Errorf("HCL output does not contain environment variable for 'web' service")
	}
	if !strings.Contains(hclOutput, "group \"api\"") {
		t.Errorf("HCL output does not contain group block for 'api' service")
	}
	if !regexp.MustCompile(`image\s*=\s*"myapi:1.0"`).MatchString(hclOutput) {
		t.Errorf("HCL output does not contain correct image for 'api' service (image = \"myapi:1.0\")")
	}
	if !strings.Contains(hclOutput, "port \"port_3000\"") { // default label if not http/s
		t.Errorf("HCL output does not contain port for 'api' service")
	}
    if !strings.Contains(hclOutput, "to = 3000") {
        t.Errorf("HCL output does not contain 'to = 3000' for 'api' service")
    }
	if !strings.Contains(hclOutput, "command = \"/app/start\"") {
		t.Errorf("HCL output does not contain correct command for 'api' service")
	}
	if !regexp.MustCompile(`args\s*=\s*\[\s*"--port",\s*"3000"\s*\]`).MatchString(hclOutput) {
		t.Errorf("HCL output does not contain correct args for 'api' service (args = [\"--port\", \"3000\"])")
	}
	webRestartPattern := `group "web"[\s\S]*?restart\s*\{\s*attempts\s*=\s*0\s*delay\s*=\s*"15s"\s*mode\s*=\s*"delay"\s*\}`
	if !regexp.MustCompile(webRestartPattern).MatchString(hclOutput) {
		t.Errorf("HCL output does not contain correct restart policy for 'web' service (unless-stopped: attempts=0, delay=15s, mode=delay)")
	}
	apiRestartPattern := `group "api"[\s\S]*?restart\s*\{\s*attempts\s*=\s*3\s*interval\s*=\s*"1m"\s*mode\s*=\s*"fail"\s*\}`
	if !regexp.MustCompile(apiRestartPattern).MatchString(hclOutput) {
		t.Errorf("HCL output does not contain correct restart policy for 'api' service (on-failure: attempts=3, interval=1m, mode=fail)")
	}

}

func TestConvertToNomadHCL_NoServices(t *testing.T) {
	yamlInput := `version: '3.8'`
	_, err := converter.ConvertToNomadHCL(yamlInput)
	if err == nil {
		t.Errorf("Expected error for no services, but got nil")
	} else if !strings.Contains(err.Error(), "no services found") {
		t.Errorf("Expected 'no services found' error, but got: %v", err)
	}
}

func TestConvertToNomadHCL_InvalidYAML(t *testing.T) {
	yamlInput := `version: '3.8
services:
  web: image: nginx` // Malformed YAML
	_, err := converter.ConvertToNomadHCL(yamlInput)
	if err == nil {
		t.Errorf("Expected error for invalid YAML, but got nil")
	} else if !strings.Contains(err.Error(), "error unmarshalling YAML") {
		t.Errorf("Expected 'error unmarshalling YAML' error, but got: %v", err)
	}
}
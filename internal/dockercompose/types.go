package dockercompose

// DockerCompose represents the top-level structure of a docker-compose.yml file.
type DockerCompose struct {
	Version  string             `yaml:"version"`
	Services map[string]Service `yaml:"services"`
	Volumes  map[string]any     `yaml:"volumes"` // Keep as any for now, can be more specific if needed
}

// Service represents a single service defined in docker-compose.yml.
type Service struct {
	Image       string            `yaml:"image"`
	Ports       []string          `yaml:"ports"`
	Environment any               `yaml:"environment"` // Can be map[string]string or []string
	Volumes     []string          `yaml:"volumes"`
	Command     any               `yaml:"command"`    // Can be string or list
	Entrypoint  any               `yaml:"entrypoint"` // Can be string or list
	Restart     string            `yaml:"restart"`
	Deploy      *Deploy           `yaml:"deploy"`
}

// Deploy represents the deployment configuration for a service.
type Deploy struct {
	Replicas *int `yaml:"replicas"`
}
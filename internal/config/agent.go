package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type AgentClientConfig struct {
	ControllerURL     string            `yaml:"controller_url"`
	Agent             AgentIdentity     `yaml:"agent"`
	HeartbeatInterval time.Duration     `yaml:"heartbeat_interval"`
	PollInterval      time.Duration     `yaml:"poll_interval"`
}

type AgentIdentity struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels"`
}

func DefaultAgentClientConfig() *AgentClientConfig {
	return &AgentClientConfig{
		ControllerURL:     "http://localhost:8080",
		Agent: AgentIdentity{
			Name: "remote-agent",
		},
		HeartbeatInterval: 30 * time.Second,
		PollInterval:      10 * time.Second,
	}
}

func LoadAgentConfig(path string) (*AgentClientConfig, error) {
	cfg := DefaultAgentClientConfig()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

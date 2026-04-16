package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Mode      string          `yaml:"mode"` // "standalone" (default), "hub", "agent"
	Server    ServerConfig    `yaml:"server"`
	Storage   StorageConfig   `yaml:"storage"`
	Retention RetentionConfig `yaml:"retention"`
	Agent     AgentConfig     `yaml:"agent"`
	Runner    RunnerConfig    `yaml:"runner"`
	Probe     ProbeConfig     `yaml:"probe"`
	Hub       HubConfig       `yaml:"hub"`
	Connect   ConnectConfig   `yaml:"connect"` // agent mode: connect to hub
}

type HubConfig struct {
	Token string `yaml:"token"` // shared secret for agent authentication
}

type ConnectConfig struct {
	HubURL            string            `yaml:"hub_url"`    // ws(s)://hub:8080/api/v1/ws/agent
	Token             string            `yaml:"token"`
	Name              string            `yaml:"name"`
	Labels            map[string]string `yaml:"labels"`
	ReconnectInterval time.Duration     `yaml:"reconnect_interval"`
	LocalHTTPAddr     string            `yaml:"local_http_addr"` // optional local UI port
}

type ProbeConfig struct {
	ScriptDir string `yaml:"script_dir"` // directory for script probes
}

type ServerConfig struct {
	HTTPAddr        string   `yaml:"http_addr"`
	GRPCAddr        string   `yaml:"grpc_addr"`
	AllowedNetworks []string `yaml:"allowed_networks"` // CIDR list, e.g. ["192.168.70.0/24","10.147.20.0/24","127.0.0.1/8"]; empty = allow all
}

type StorageConfig struct {
	Driver string       `yaml:"driver"`
	SQLite SQLiteConfig `yaml:"sqlite"`
}

type SQLiteConfig struct {
	Path string `yaml:"path"`
}

type RetentionConfig struct {
	RawResults time.Duration `yaml:"raw_results"`
	Aggregated time.Duration `yaml:"aggregated"`
}

type AgentConfig struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels"`
}

type RunnerConfig struct {
	MaxConcurrent int `yaml:"max_concurrent"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPAddr: ":8080",
			GRPCAddr: ":9090",
		},
		Storage: StorageConfig{
			Driver: "sqlite",
			SQLite: SQLiteConfig{Path: "./data/netprobe.db"},
		},
		Retention: RetentionConfig{
			RawResults: 30 * 24 * time.Hour,
		},
		Agent: AgentConfig{
			Name: "local",
		},
		Runner: RunnerConfig{
			MaxConcurrent: 50,
		},
		Probe: ProbeConfig{
			ScriptDir: "./scripts/probes",
		},
		Hub: HubConfig{},
		Connect: ConnectConfig{
			ReconnectInterval: 5 * time.Second,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil // use defaults if file not found
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

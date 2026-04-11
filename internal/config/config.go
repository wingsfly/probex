package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Storage   StorageConfig   `yaml:"storage"`
	Retention RetentionConfig `yaml:"retention"`
	Agent     AgentConfig     `yaml:"agent"`
	Runner    RunnerConfig    `yaml:"runner"`
	Probe     ProbeConfig     `yaml:"probe"`
}

type ProbeConfig struct {
	ScriptDir string `yaml:"script_dir"` // directory for script probes
}

type ServerConfig struct {
	HTTPAddr string `yaml:"http_addr"`
	GRPCAddr string `yaml:"grpc_addr"`
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

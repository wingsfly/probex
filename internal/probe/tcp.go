package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type TCPConfig struct {
	Port int `json:"port"`
}

type TCPProber struct{}

func NewTCPProber() *TCPProber { return &TCPProber{} }

func (p *TCPProber) Name() string { return "tcp" }

func (p *TCPProber) Metadata() ProbeMetadata {
	return ProbeMetadata{
		Name:        "tcp",
		Kind:        ProbeKindBuiltin,
		Description: "TCP Connect — measures connection establishment latency",
		ParameterSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"port": {"type":"integer","title":"Port","default":80,"minimum":1,"maximum":65535}
			}
		}`),
		OutputSchema: &OutputSchema{
			StandardFields: []string{"latency_ms"},
		},
	}
}

func (p *TCPProber) Probe(ctx context.Context, target string, rawConfig json.RawMessage) (*Result, error) {
	cfg := TCPConfig{Port: 80}
	if len(rawConfig) > 0 {
		json.Unmarshal(rawConfig, &cfg)
	}

	addr := fmt.Sprintf("%s:%d", target, cfg.Port)
	start := time.Now()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	latency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &Result{
			Success:   false,
			LatencyMs: latency,
			Error:     fmt.Sprintf("connect: %v", err),
		}, nil
	}
	conn.Close()

	return &Result{
		Success:   true,
		LatencyMs: latency,
	}, nil
}

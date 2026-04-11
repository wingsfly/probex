package probe

import (
	"context"
	"encoding/json"
)

type Result struct {
	Success        bool            `json:"success"`
	LatencyMs      float64         `json:"latency_ms"`
	JitterMs       *float64        `json:"jitter_ms,omitempty"`
	PacketLossPct  *float64        `json:"packet_loss_pct,omitempty"`
	DNSResolveMs   *float64        `json:"dns_resolve_ms,omitempty"`
	TLSHandshakeMs *float64        `json:"tls_handshake_ms,omitempty"`
	StatusCode     *int            `json:"status_code,omitempty"`
	DownloadBps    *float64        `json:"download_bps,omitempty"`
	UploadBps      *float64        `json:"upload_bps,omitempty"`
	Error          string          `json:"error,omitempty"`
	Extra          json.RawMessage `json:"extra,omitempty"`
}

type Prober interface {
	Name() string
	Probe(ctx context.Context, target string, config json.RawMessage) (*Result, error)
}

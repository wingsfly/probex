package model

import (
	"encoding/json"
	"time"
)

// ProbeRegistration stores metadata for externally registered probes.
type ProbeRegistration struct {
	Name            string          `json:"name"`
	Kind            string          `json:"kind"`
	Description     string          `json:"description"`
	ParameterSchema json.RawMessage `json:"parameter_schema"`
	OutputSchema    json.RawMessage `json:"output_schema"`
	RegisteredAt    time.Time       `json:"registered_at"`
	LastPushAt      *time.Time      `json:"last_push_at,omitempty"`
}

// ProbeRegisterRequest is the request body for POST /probes/register.
type ProbeRegisterRequest struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	ParameterSchema json.RawMessage `json:"parameter_schema,omitempty"`
	OutputSchema    json.RawMessage `json:"output_schema,omitempty"`
}

// ProbePushRequest is the request body for POST /probes/{name}/push.
type ProbePushRequest struct {
	TaskID  string `json:"task_id,omitempty"`
	AgentID string `json:"agent_id,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
	Results []struct {
		Timestamp      *string         `json:"timestamp,omitempty"`
		Success        bool            `json:"success"`
		LatencyMs      *float64        `json:"latency_ms,omitempty"`
		JitterMs       *float64        `json:"jitter_ms,omitempty"`
		PacketLossPct  *float64        `json:"packet_loss_pct,omitempty"`
		DNSResolveMs   *float64        `json:"dns_resolve_ms,omitempty"`
		TLSHandshakeMs *float64        `json:"tls_handshake_ms,omitempty"`
		StatusCode     *int            `json:"status_code,omitempty"`
		DownloadBps    *float64        `json:"download_bps,omitempty"`
		UploadBps      *float64        `json:"upload_bps,omitempty"`
		Error          string          `json:"error,omitempty"`
		Extra          json.RawMessage `json:"extra,omitempty"`
	} `json:"results"`
}

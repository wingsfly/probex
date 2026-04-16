package model

import (
	"encoding/json"
	"time"
)

type ProbeResult struct {
	ID             string          `json:"id"`
	TaskID         string          `json:"task_id"`
	AgentID        string          `json:"agent_id"`
	NodeID         string          `json:"node_id,omitempty"`
	Timestamp      time.Time       `json:"timestamp"`
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
}

type ResultFilter struct {
	TaskID  string
	AgentID string
	From    time.Time
	To      time.Time
	Limit   int
	Offset  int
}

type ResultSummary struct {
	TaskID       string  `json:"task_id"`
	AgentID      string  `json:"agent_id"`
	Count        int64   `json:"count"`
	SuccessRate  float64 `json:"success_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
	P99LatencyMs float64 `json:"p99_latency_ms"`
	MinLatencyMs float64 `json:"min_latency_ms"`
	MaxLatencyMs float64 `json:"max_latency_ms"`
	AvgJitterMs  float64 `json:"avg_jitter_ms"`
	AvgLossPct   float64 `json:"avg_loss_pct"`
}

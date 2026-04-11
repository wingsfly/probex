package probe

import "encoding/json"

// ProbeKind identifies how a probe is provided.
type ProbeKind string

const (
	ProbeKindBuiltin  ProbeKind = "builtin"
	ProbeKindScript   ProbeKind = "script"
	ProbeKindExternal ProbeKind = "external"
)

// ProbeMetadata describes a probe's identity, parameters, and outputs.
// This is the universal schema for built-in, script, and external probes.
type ProbeMetadata struct {
	Name            string          `json:"name"`
	Kind            ProbeKind       `json:"kind"`
	Description     string          `json:"description"`
	Version         string          `json:"version,omitempty"`
	ParameterSchema json.RawMessage `json:"parameter_schema"` // JSON Schema draft-07 subset
	OutputSchema    *OutputSchema   `json:"output_schema,omitempty"`
}

// OutputSchema declares what metrics a probe produces.
type OutputSchema struct {
	// StandardFields lists which standard Result fields this probe populates.
	// Valid values: "latency_ms", "jitter_ms", "packet_loss_pct", "dns_resolve_ms",
	// "tls_handshake_ms", "status_code", "download_bps", "upload_bps"
	StandardFields []string `json:"standard_fields"`

	// ExtraFields describes custom metrics stored in Result.Extra.
	ExtraFields []ExtraField `json:"extra_fields,omitempty"`
}

// ExtraField describes a custom metric in Result.Extra.
type ExtraField struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "number", "string", "boolean"
	Unit        string `json:"unit,omitempty"`
	Description string `json:"description,omitempty"`
	Chartable   bool   `json:"chartable,omitempty"` // hint: can be plotted
}

// MetadataProber is an optional interface. Probes that implement it
// provide rich metadata for UI generation and documentation.
// Probes that only implement Prober get minimal auto-generated metadata.
type MetadataProber interface {
	Prober
	Metadata() ProbeMetadata
}

// EmptySchema is a minimal JSON Schema for probes without parameters.
var EmptySchema = json.RawMessage(`{"type":"object","properties":{}}`)

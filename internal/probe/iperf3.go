package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// Iperf3Config maps to all common iperf3 CLI parameters.
// Users can configure any combination via the task's config JSON.
type Iperf3Config struct {
	Port     int    `json:"port"`      // -p, server port (default 5201)
	Duration int    `json:"duration"`  // -t, test duration in seconds (default 10)
	Parallel int    `json:"parallel"`  // -P, number of parallel streams (default 1)
	Reverse  bool   `json:"reverse"`   // -R, reverse mode (server sends, client receives) for download test
	Bidir    bool   `json:"bidir"`     // --bidir, bidirectional test (simultaneous up+down)
	Protocol string `json:"protocol"`  // "tcp" or "udp" (default "tcp")
	Bandwidth string `json:"bandwidth"` // -b, target bandwidth for UDP e.g. "10M" (default unlimited for TCP)
	Window   string `json:"window"`    // -w, socket buffer size e.g. "256K"
	MSS      int    `json:"mss"`       // -M, TCP maximum segment size
	Interval int    `json:"interval"`  // -i, reporting interval in seconds (default 1)
	Bytes    string `json:"bytes"`     // -n, number of bytes to transmit instead of duration e.g. "100M"
	Omit     int    `json:"omit"`      // -O, omit first N seconds from statistics
	ZeroCopy bool   `json:"zerocopy"`  // -Z, use zero-copy method
	NoDelay  bool   `json:"nodelay"`   // -N, set TCP no delay
	DSCP     string `json:"dscp"`      // -S, DSCP value
	Cport    int    `json:"cport"`     // --cport, bind to specific client port
	Affinity string `json:"affinity"`  // -A, CPU affinity
}

// Iperf3Extra stores detailed results from iperf3 JSON output for later analysis.
type Iperf3Extra struct {
	// Summary stats
	SentBps     float64 `json:"sent_bps"`
	SentBytes   float64 `json:"sent_bytes"`
	RecvBps     float64 `json:"recv_bps"`
	RecvBytes   float64 `json:"recv_bytes"`
	Retransmits int     `json:"retransmits"`
	// UDP-specific stats
	JitterMs       float64 `json:"jitter_ms"`
	LostPackets    int     `json:"lost_packets"`
	TotalPackets   int     `json:"total_packets"`
	LostPercent    float64 `json:"lost_percent"`
	OutOfOrder     int     `json:"out_of_order"`
	OutOfOrderPct  float64 `json:"out_of_order_pct"`
	// Per-interval data for detailed analysis
	Intervals []Iperf3Interval `json:"intervals,omitempty"`
	// TCP info
	SenderMaxSndCwnd int     `json:"sender_max_snd_cwnd,omitempty"`
	SenderMinRTT     float64 `json:"sender_min_rtt_ms,omitempty"`
	SenderMeanRTT    float64 `json:"sender_mean_rtt_ms,omitempty"`
	// TCP-derived jitter (throughput variation across intervals)
	ThroughputJitterMbps float64 `json:"throughput_jitter_mbps,omitempty"`
	// Config echo
	Protocol string `json:"protocol"`
	Duration int    `json:"duration"`
	Streams  int    `json:"streams"`
}

type Iperf3Interval struct {
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
	Bps         float64 `json:"bps"`
	Bytes       int64   `json:"bytes"`
	Retransmits int     `json:"retransmits"`
	JitterMs    float64 `json:"jitter_ms,omitempty"`
	LostPackets int     `json:"lost_packets,omitempty"`
	Packets     int     `json:"packets,omitempty"`
}

type Iperf3Prober struct{}

func NewIperf3Prober() *Iperf3Prober { return &Iperf3Prober{} }

func (p *Iperf3Prober) Name() string { return "iperf3" }

func (p *Iperf3Prober) Metadata() ProbeMetadata {
	return ProbeMetadata{
		Name:        "iperf3",
		Kind:        ProbeKindBuiltin,
		Description: "iPerf3 bandwidth test — measures throughput, jitter, loss, reorder (UDP/TCP)",
		ParameterSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"port":      {"type":"integer","title":"Server Port","default":5201,"minimum":1,"maximum":65535},
				"duration":  {"type":"integer","title":"Duration (sec)","default":10,"minimum":1,"maximum":3600},
				"parallel":  {"type":"integer","title":"Parallel Streams","default":1,"minimum":1,"maximum":128},
				"protocol":  {"type":"string","title":"Protocol","enum":["tcp","udp"],"default":"tcp"},
				"reverse":   {"type":"boolean","title":"Reverse (-R, download)","default":false},
				"bidir":     {"type":"boolean","title":"Bidirectional (--bidir)","default":false},
				"bandwidth": {"type":"string","title":"Bandwidth Limit","x-ui-placeholder":"e.g. 100M (empty=unlimited)"},
				"window":    {"type":"string","title":"Window Size","x-ui-placeholder":"e.g. 256K (empty=default)"},
				"omit":      {"type":"integer","title":"Omit (sec)","default":0,"minimum":0},
				"nodelay":   {"type":"boolean","title":"TCP No Delay (-N)","default":false},
				"zerocopy":  {"type":"boolean","title":"Zero Copy (-Z)","default":false},
				"dscp":      {"type":"string","title":"DSCP Value","x-ui-placeholder":"e.g. EF, AF41"},
				"cport":     {"type":"integer","title":"Client Port","minimum":0,"maximum":65535},
				"mss":       {"type":"integer","title":"TCP MSS","minimum":0}
			},
			"x-ui-order": ["port","duration","parallel","protocol","bandwidth","reverse","bidir","window","omit","nodelay","zerocopy","dscp","cport","mss"]
		}`),
		OutputSchema: &OutputSchema{
			StandardFields: []string{"latency_ms", "jitter_ms", "packet_loss_pct", "download_bps", "upload_bps"},
			ExtraFields: []ExtraField{
				{Name: "retransmits", Type: "number", Description: "TCP retransmits", Chartable: true},
				{Name: "out_of_order", Type: "number", Description: "Out-of-order packets"},
				{Name: "out_of_order_pct", Type: "number", Unit: "%", Description: "Out-of-order percentage", Chartable: true},
				{Name: "jitter_ms", Type: "number", Unit: "ms", Description: "UDP jitter", Chartable: true},
				{Name: "lost_percent", Type: "number", Unit: "%", Description: "UDP loss percentage", Chartable: true},
				{Name: "sent_bps", Type: "number", Unit: "bps", Description: "Sent bitrate"},
				{Name: "recv_bps", Type: "number", Unit: "bps", Description: "Received bitrate"},
			},
		},
	}
}

func (p *Iperf3Prober) Probe(ctx context.Context, target string, rawConfig json.RawMessage) (*Result, error) {
	// iperf3 needs more time than its test duration (connection setup + result collection).
	// Override the parent context timeout to duration + 30s buffer.
	cfg := Iperf3Config{
		Port:     5201,
		Duration: 10,
		Parallel: 1,
		Protocol: "tcp",
		Interval: 1,
	}
	if len(rawConfig) > 0 {
		json.Unmarshal(rawConfig, &cfg)
	}

	// Build iperf3 command with JSON output
	args := []string{"-c", target, "-J"} // -J for JSON output
	args = append(args, "-p", strconv.Itoa(cfg.Port))

	if cfg.Bytes != "" {
		args = append(args, "-n", cfg.Bytes)
	} else {
		args = append(args, "-t", strconv.Itoa(cfg.Duration))
	}

	args = append(args, "-P", strconv.Itoa(cfg.Parallel))
	args = append(args, "-i", strconv.Itoa(cfg.Interval))

	if cfg.Reverse {
		args = append(args, "-R")
	}
	if cfg.Bidir {
		args = append(args, "--bidir")
	}
	if cfg.Protocol == "udp" {
		args = append(args, "-u")
	}
	if cfg.Bandwidth != "" {
		args = append(args, "-b", cfg.Bandwidth)
	}
	if cfg.Window != "" {
		args = append(args, "-w", cfg.Window)
	}
	if cfg.MSS > 0 {
		args = append(args, "-M", strconv.Itoa(cfg.MSS))
	}
	if cfg.Omit > 0 {
		args = append(args, "-O", strconv.Itoa(cfg.Omit))
	}
	if cfg.ZeroCopy {
		args = append(args, "-Z")
	}
	if cfg.NoDelay {
		args = append(args, "-N")
	}
	if cfg.DSCP != "" {
		args = append(args, "-S", cfg.DSCP)
	}
	if cfg.Cport > 0 {
		args = append(args, "--cport", strconv.Itoa(cfg.Cport))
	}
	if cfg.Affinity != "" {
		args = append(args, "-A", cfg.Affinity)
	}

	// Execute iperf3
	// Use CombinedOutput because iperf3 with -J writes JSON to stdout even on error (exit code 1).
	// The JSON may contain an "error" field with the actual message.
	iperf3Bin := findBinary("iperf3")
	cmd := exec.CommandContext(ctx, iperf3Bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try parsing the JSON output first — iperf3 often returns valid JSON with an error field
		if len(output) > 0 {
			result, parseErr := parseIperf3Output(output, cfg)
			if parseErr == nil && result != nil {
				return result, nil
			}
		}
		errMsg := fmt.Sprintf("iperf3: %v", err)
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			errMsg = fmt.Sprintf("iperf3: %s", string(exitErr.Stderr))
		}
		return &Result{Success: false, Error: errMsg}, nil
	}

	// Parse iperf3 JSON output
	return parseIperf3Output(output, cfg)
}

func parseIperf3Output(output []byte, cfg Iperf3Config) (*Result, error) {
	var raw map[string]any
	if err := json.Unmarshal(output, &raw); err != nil {
		return &Result{Success: false, Error: fmt.Sprintf("parse iperf3 json: %v", err)}, nil
	}

	// Check for iperf3-level error
	if errMsg, ok := raw["error"].(string); ok {
		return &Result{Success: false, Error: errMsg}, nil
	}

	end, ok := raw["end"].(map[string]any)
	if !ok {
		return &Result{Success: false, Error: "missing 'end' in iperf3 output"}, nil
	}

	extra := Iperf3Extra{
		Protocol: cfg.Protocol,
		Duration: cfg.Duration,
		Streams:  cfg.Parallel,
	}

	var downloadBps, uploadBps float64

	if cfg.Protocol == "udp" {
		// UDP summary — jitter, lost_packets, packets from end.sum
		if sum, ok := end["sum"].(map[string]any); ok {
			bps := getFloat(sum, "bits_per_second")
			bytes := getFloat(sum, "bytes")
			extra.JitterMs = getFloat(sum, "jitter_ms")
			extra.LostPackets = getInt(sum, "lost_packets")
			extra.TotalPackets = getInt(sum, "packets")
			if extra.TotalPackets > 0 {
				extra.LostPercent = float64(extra.LostPackets) / float64(extra.TotalPackets) * 100
			}
			if cfg.Reverse {
				downloadBps = bps
				extra.RecvBps = bps
				extra.RecvBytes = bytes
			} else {
				uploadBps = bps
				extra.SentBps = bps
				extra.SentBytes = bytes
			}
		}
		// out_of_order is only in end.streams[].udp, not in end.sum — accumulate from all streams
		if streams, ok := end["streams"].([]any); ok {
			for _, s := range streams {
				sMap, ok := s.(map[string]any)
				if !ok {
					continue
				}
				if udp, ok := sMap["udp"].(map[string]any); ok {
					extra.OutOfOrder += getInt(udp, "out_of_order")
				}
			}
		}
		if extra.TotalPackets > 0 {
			extra.OutOfOrderPct = float64(extra.OutOfOrder) / float64(extra.TotalPackets) * 100
		}
	} else {
		// TCP summary — sent and received
		if sumSent, ok := end["sum_sent"].(map[string]any); ok {
			extra.SentBps = getFloat(sumSent, "bits_per_second")
			extra.SentBytes = getFloat(sumSent, "bytes")
			extra.Retransmits = getInt(sumSent, "retransmits")
		}
		if sumRecv, ok := end["sum_received"].(map[string]any); ok {
			extra.RecvBps = getFloat(sumRecv, "bits_per_second")
			extra.RecvBytes = getFloat(sumRecv, "bytes")
		}

		if cfg.Bidir {
			// Bidir: sent = upload, received = download
			uploadBps = extra.SentBps
			downloadBps = extra.RecvBps
		} else if cfg.Reverse {
			// Reverse: server sends to client = download
			downloadBps = extra.RecvBps
		} else {
			// Normal: client sends to server = upload
			uploadBps = extra.SentBps
		}

		// TCP sender stats (cwnd, rtt)
		if streams, ok := end["streams"].([]any); ok && len(streams) > 0 {
			if s, ok := streams[0].(map[string]any); ok {
				if sender, ok := s["sender"].(map[string]any); ok {
					extra.SenderMaxSndCwnd = getInt(sender, "max_snd_cwnd")
					extra.SenderMinRTT = getFloat(sender, "min_rtt") / 1000.0  // us -> ms
					extra.SenderMeanRTT = getFloat(sender, "mean_rtt") / 1000.0
				}
			}
		}
	}

	// Parse per-interval data
	if intervals, ok := raw["intervals"].([]any); ok {
		for _, iv := range intervals {
			ivMap, ok := iv.(map[string]any)
			if !ok {
				continue
			}
			if sum, ok := ivMap["sum"].(map[string]any); ok {
				extra.Intervals = append(extra.Intervals, Iperf3Interval{
					Start:       getFloat(sum, "start"),
					End:         getFloat(sum, "end"),
					Bps:         getFloat(sum, "bits_per_second"),
					Bytes:       int64(getFloat(sum, "bytes")),
					Retransmits: getInt(sum, "retransmits"),
					JitterMs:    getFloat(sum, "jitter_ms"),
					LostPackets: getInt(sum, "lost_packets"),
					Packets:     getInt(sum, "packets"),
				})
			}
		}
	}

	// For TCP: compute throughput jitter from interval-to-interval bandwidth variation
	// This measures how stable the throughput is over time
	if cfg.Protocol == "tcp" && len(extra.Intervals) >= 2 {
		bpsValues := make([]float64, len(extra.Intervals))
		for i, iv := range extra.Intervals {
			bpsValues[i] = iv.Bps
		}
		var sumDiff float64
		for i := 1; i < len(bpsValues); i++ {
			diff := bpsValues[i] - bpsValues[i-1]
			if diff < 0 {
				diff = -diff
			}
			sumDiff += diff
		}
		extra.ThroughputJitterMbps = (sumDiff / float64(len(bpsValues)-1)) / 1e6

		// TCP "packet loss" proxy: retransmits relative to estimated total segments
		// Estimate total segments from bytes / typical MSS (1460)
		totalBytes := extra.SentBytes
		if totalBytes > 0 {
			estSegments := totalBytes / 1460
			if estSegments > 0 {
				extra.LostPercent = float64(extra.Retransmits) / estSegments * 100
				extra.LostPackets = extra.Retransmits
				extra.TotalPackets = int(estSegments)
			}
		}
	}

	extraJSON, _ := json.Marshal(extra)

	// Map to standard Result fields
	latency := extra.SenderMeanRTT
	var jitter *float64
	if cfg.Protocol == "udp" && extra.JitterMs > 0 {
		jitter = &extra.JitterMs
	} else if cfg.Protocol == "tcp" && extra.ThroughputJitterMbps > 0 {
		// Store throughput jitter in jitter field (in Mbps, noted in extra)
		jitter = &extra.ThroughputJitterMbps
	}
	var lossPct *float64
	if extra.LostPercent > 0 {
		lossPct = &extra.LostPercent
	}

	return &Result{
		Success:       true,
		LatencyMs:     latency,
		JitterMs:      jitter,
		PacketLossPct: lossPct,
		DownloadBps:   nilIfZero(downloadBps),
		UploadBps:     nilIfZero(uploadBps),
		Extra:         extraJSON,
	}, nil
}

func getFloat(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func getInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// findBinary searches common paths for a binary that may not be in the current PATH.
func findBinary(name string) string {
	// Try PATH first
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	// Search common locations
	candidates := []string{
		"/opt/homebrew/bin/" + name,
		"/usr/local/bin/" + name,
		"/usr/bin/" + name,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return name // fallback to bare name
}

func nilIfZero(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

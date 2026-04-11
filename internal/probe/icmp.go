package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type ICMPConfig struct {
	Count    int    `json:"count"`
	Interval string `json:"interval"`
}

type ICMPProber struct{}

func NewICMPProber() *ICMPProber { return &ICMPProber{} }

func (p *ICMPProber) Name() string { return "icmp" }

func (p *ICMPProber) Metadata() ProbeMetadata {
	return ProbeMetadata{
		Name:        "icmp",
		Kind:        ProbeKindBuiltin,
		Description: "ICMP Ping — measures RTT, jitter, and packet loss",
		ParameterSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"count":    {"type":"integer","title":"Ping Count","default":5,"minimum":1,"maximum":100},
				"interval": {"type":"string","title":"Interval","default":"200ms","x-ui-placeholder":"e.g. 200ms, 1s"}
			},
			"x-ui-order": ["count","interval"]
		}`),
		OutputSchema: &OutputSchema{
			StandardFields: []string{"latency_ms", "jitter_ms", "packet_loss_pct"},
		},
	}
}

func (p *ICMPProber) Probe(ctx context.Context, target string, rawConfig json.RawMessage) (*Result, error) {
	cfg := ICMPConfig{Count: 5, Interval: "200ms"}
	if len(rawConfig) > 0 {
		json.Unmarshal(rawConfig, &cfg)
	}
	if cfg.Count < 1 {
		cfg.Count = 5
	}
	interval, _ := time.ParseDuration(cfg.Interval)
	if interval == 0 {
		interval = 200 * time.Millisecond
	}

	addr, err := net.ResolveIPAddr("ip4", target)
	if err != nil {
		return &Result{Success: false, Error: fmt.Sprintf("resolve: %v", err)}, nil
	}

	conn, err := icmp.ListenPacket("udp4", "")
	if err != nil {
		return &Result{Success: false, Error: fmt.Sprintf("listen: %v", err)}, nil
	}
	defer conn.Close()

	var rtts []float64
	lost := 0
	pid := os.Getpid() & 0xffff

	for i := 0; i < cfg.Count; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return &Result{Success: false, Error: "timeout"}, nil
			case <-time.After(interval):
			}
		}

		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{ID: pid, Seq: i, Data: []byte("netprobe")},
		}
		wb, _ := msg.Marshal(nil)

		start := time.Now()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := conn.WriteTo(wb, &net.UDPAddr{IP: addr.IP}); err != nil {
			lost++
			continue
		}

		rb := make([]byte, 1500)
		n, _, err := conn.ReadFrom(rb)
		elapsed := time.Since(start)
		if err != nil {
			lost++
			continue
		}

		rm, err := icmp.ParseMessage(1, rb[:n])
		if err != nil || rm.Type != ipv4.ICMPTypeEchoReply {
			lost++
			continue
		}

		rtts = append(rtts, float64(elapsed.Microseconds()) / 1000.0)
	}

	if len(rtts) == 0 {
		lossPct := 100.0
		return &Result{Success: false, LatencyMs: 0, PacketLossPct: &lossPct, Error: "all packets lost"}, nil
	}

	avgLatency := avg(rtts)
	jitter := calcJitter(rtts)
	lossPct := float64(lost) / float64(cfg.Count) * 100

	return &Result{
		Success:       lossPct < 100,
		LatencyMs:     avgLatency,
		JitterMs:      &jitter,
		PacketLossPct: &lossPct,
	}, nil
}

func avg(vals []float64) float64 {
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func calcJitter(rtts []float64) float64 {
	if len(rtts) < 2 {
		return 0
	}
	var sumDiff float64
	for i := 1; i < len(rtts); i++ {
		sumDiff += math.Abs(rtts[i] - rtts[i-1])
	}
	return sumDiff / float64(len(rtts)-1)
}

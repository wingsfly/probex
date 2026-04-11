package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type DNSConfig struct {
	Server     string `json:"server"`
	RecordType string `json:"record_type"`
}

type DNSProber struct{}

func NewDNSProber() *DNSProber { return &DNSProber{} }

func (p *DNSProber) Name() string { return "dns" }

func (p *DNSProber) Metadata() ProbeMetadata {
	return ProbeMetadata{
		Name:        "dns",
		Kind:        ProbeKindBuiltin,
		Description: "DNS resolution probe — measures query latency",
		ParameterSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"server":      {"type":"string","title":"DNS Server","x-ui-placeholder":"e.g. 8.8.8.8 (empty=system default)"},
				"record_type": {"type":"string","title":"Record Type","enum":["A","AAAA","MX","NS","TXT","CNAME"],"default":"A"}
			},
			"x-ui-order": ["record_type","server"]
		}`),
		OutputSchema: &OutputSchema{
			StandardFields: []string{"latency_ms", "dns_resolve_ms"},
			ExtraFields:    []ExtraField{{Name: "records", Type: "string", Description: "Resolved records"}},
		},
	}
}

func (p *DNSProber) Probe(ctx context.Context, target string, rawConfig json.RawMessage) (*Result, error) {
	cfg := DNSConfig{RecordType: "A"}
	if len(rawConfig) > 0 {
		json.Unmarshal(rawConfig, &cfg)
	}

	resolver := net.DefaultResolver
	if cfg.Server != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, "udp", cfg.Server+":53")
			},
		}
	}

	start := time.Now()
	var err error
	var records []string

	switch cfg.RecordType {
	case "A", "AAAA", "":
		var addrs []string
		addrs, err = resolver.LookupHost(ctx, target)
		records = addrs
	case "MX":
		var mxs []*net.MX
		mxs, err = resolver.LookupMX(ctx, target)
		for _, mx := range mxs {
			records = append(records, fmt.Sprintf("%s %d", mx.Host, mx.Pref))
		}
	case "NS":
		var nss []*net.NS
		nss, err = resolver.LookupNS(ctx, target)
		for _, ns := range nss {
			records = append(records, ns.Host)
		}
	case "TXT":
		records, err = resolver.LookupTXT(ctx, target)
	case "CNAME":
		var cname string
		cname, err = resolver.LookupCNAME(ctx, target)
		records = []string{cname}
	default:
		return &Result{Success: false, Error: fmt.Sprintf("unsupported record type: %s", cfg.RecordType)}, nil
	}

	latency := float64(time.Since(start).Microseconds()) / 1000.0
	dnsMs := latency

	if err != nil {
		return &Result{
			Success:      false,
			LatencyMs:    latency,
			DNSResolveMs: &dnsMs,
			Error:        fmt.Sprintf("dns lookup: %v", err),
		}, nil
	}

	extra, _ := json.Marshal(map[string]any{"records": records, "record_type": cfg.RecordType})

	return &Result{
		Success:      true,
		LatencyMs:    latency,
		DNSResolveMs: &dnsMs,
		Extra:        extra,
	}, nil
}

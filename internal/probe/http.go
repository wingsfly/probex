package probe

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"time"
)

type HTTPConfig struct {
	Method         string            `json:"method"`
	Headers        map[string]string `json:"headers"`
	ExpectedStatus int               `json:"expected_status"`
	SkipTLSVerify  bool              `json:"skip_tls_verify"`
}

type HTTPProber struct{}

func NewHTTPProber() *HTTPProber { return &HTTPProber{} }

func (p *HTTPProber) Name() string { return "http" }

func (p *HTTPProber) Metadata() ProbeMetadata {
	return ProbeMetadata{
		Name:        "http",
		Kind:        ProbeKindBuiltin,
		Description: "HTTP(S) endpoint probe with DNS/TLS timing breakdown",
		ParameterSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"method":          {"type":"string","title":"HTTP Method","enum":["GET","POST","PUT","DELETE","HEAD"],"default":"GET"},
				"headers":         {"type":"object","title":"Custom Headers","additionalProperties":{"type":"string"},"x-ui-widget":"key-value"},
				"expected_status": {"type":"integer","title":"Expected Status Code","default":200,"minimum":100,"maximum":599},
				"skip_tls_verify": {"type":"boolean","title":"Skip TLS Verification","default":false}
			},
			"x-ui-order": ["method","expected_status","headers","skip_tls_verify"]
		}`),
		OutputSchema: &OutputSchema{
			StandardFields: []string{"latency_ms", "dns_resolve_ms", "tls_handshake_ms", "status_code"},
		},
	}
}

func (p *HTTPProber) Probe(ctx context.Context, target string, rawConfig json.RawMessage) (*Result, error) {
	cfg := HTTPConfig{Method: "GET", ExpectedStatus: 200}
	if len(rawConfig) > 0 {
		json.Unmarshal(rawConfig, &cfg)
	}
	if cfg.Method == "" {
		cfg.Method = "GET"
	}

	url := target
	if len(url) > 0 && url[0] != 'h' {
		url = "http://" + url
	}

	req, err := http.NewRequestWithContext(ctx, cfg.Method, url, nil)
	if err != nil {
		return &Result{Success: false, Error: fmt.Sprintf("build request: %v", err)}, nil
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	var dnsStart, dnsEnd, tlsStart, tlsEnd, connStart time.Time

	trace := &httptrace.ClientTrace{
		DNSStart:             func(_ httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:              func(_ httptrace.DNSDoneInfo) { dnsEnd = time.Now() },
		ConnectStart:         func(_, _ string) { connStart = time.Now() },
		TLSHandshakeStart:   func() { tlsStart = time.Now() },
		TLSHandshakeDone:    func(_ tls.ConnectionState, _ error) { tlsEnd = time.Now() },
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.SkipTLSVerify},
	}
	client := &http.Client{Transport: transport}

	start := time.Now()
	resp, err := client.Do(req)
	totalLatency := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		return &Result{
			Success:   false,
			LatencyMs: totalLatency,
			Error:     fmt.Sprintf("request: %v", err),
		}, nil
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	var dnsMs, tlsMs *float64
	if !dnsStart.IsZero() && !dnsEnd.IsZero() {
		d := float64(dnsEnd.Sub(dnsStart).Microseconds()) / 1000.0
		dnsMs = &d
	}
	if !tlsStart.IsZero() && !tlsEnd.IsZero() {
		t := float64(tlsEnd.Sub(tlsStart).Microseconds()) / 1000.0
		tlsMs = &t
	}

	_ = connStart // reserved for future connect latency breakdown

	statusCode := resp.StatusCode
	success := true
	errMsg := ""
	if cfg.ExpectedStatus > 0 && statusCode != cfg.ExpectedStatus {
		success = false
		errMsg = fmt.Sprintf("unexpected status: %d", statusCode)
	}

	return &Result{
		Success:        success,
		LatencyMs:      totalLatency,
		DNSResolveMs:   dnsMs,
		TLSHandshakeMs: tlsMs,
		StatusCode:     &statusCode,
		Error:          errMsg,
	}, nil
}

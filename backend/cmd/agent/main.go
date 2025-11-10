package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Target struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	TimeoutMs int    `json:"timeout_ms"`
}

type Check struct {
	TargetID   int64  `json:"target_id"`
	TS         string `json:"ts"`
	StatusCode int    `json:"status_code"`
	OK         bool   `json:"ok"`
	LatencyMs  int    `json:"latency_ms"`
	Error      string `json:"error"`
}

func main() {
	base := mustEnv("CENTRAL_BASE_URL")
	apiKey := mustEnv("API_KEY")
	poll := getenvInt("POLL_INTERVAL_SEC", 15)
	var targets []Target
	mustLoadJSON(getenv("TARGETS_FILE", "./targets.json"), &targets)

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
	}

	ticker := time.NewTicker(time.Duration(poll) * time.Second)
	defer ticker.Stop()
	ctx := context.Background()

	for {
		<-ticker.C
		var batch []Check
		for _, t := range targets {
			status, ok, reason, latency := probe(ctx, client, t.URL, t.TimeoutMs)
			batch = append(batch, Check{
				TargetID:   t.ID,
				TS:         time.Now().UTC().Format(time.RFC3339),
				StatusCode: status,
				OK:         ok,
				LatencyMs:  latency,
				Error:      reason,
			})
		}
		postJSON(client, base+"/api/ingest/checks", apiKey, map[string]any{"checks": batch})
	}
}

func probe(ctx context.Context, hc *http.Client, url string, timeoutMs int) (int, bool, string, int) {
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "status-agent/0.1")
	hc.Timeout = time.Duration(timeoutMs) * time.Millisecond

	resp, err := hc.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return 0, false, classify(err), latency
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return resp.StatusCode, true, "", latency
	}
	return resp.StatusCode, false, "non_2xx", latency
}

func classify(err error) string {
	e := err.Error()
	switch {
	case strings.Contains(e, "timeout") || strings.Contains(e, "DeadlineExceeded") || strings.Contains(e, "Client.Timeout"):
		return "timeout"
	case strings.Contains(e, "no such host") || strings.Contains(e, "lookup "):
		return "dns_error"
	case strings.Contains(e, "x509") || strings.Contains(e, "tls:"):
		return "tls_error"
	default:
		return "dns_error"
	}
}

func postJSON(hc *http.Client, url, apiKey string, body any) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", apiKey)
	resp, err := hc.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		panic(k + " missing")
	}
	return v
}
func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func getenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			return n
		}
	}
	return def
}
func mustLoadJSON(path string, v any) {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		panic(err)
	}
}

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

type CheckLog struct {
	TS    string `json:"ts"`    // RFC3339
	Level string `json:"level"` // trace|info|warn|error
	Line  string `json:"line"`
}
type Check struct {
	TargetID   int64      `json:"target_id"`
	TS         string     `json:"ts"`
	StatusCode int        `json:"status_code"`
	OK         bool       `json:"ok"`
	LatencyMs  int        `json:"latency_ms"`
	Error      string     `json:"error"`
	Logs       []CheckLog `json:"logs,omitempty"`
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
			status, ok, reason, latency, logs := probe(ctx, client, t.URL, t.TimeoutMs, t.Name)
			batch = append(batch, Check{
				TargetID:   t.ID,
				TS:         time.Now().UTC().Format(time.RFC3339),
				StatusCode: status,
				OK:         ok,
				LatencyMs:  latency,
				Error:      reason,
				Logs:       logs,
			})
		}
		postJSON(client, base+"/api/ingest/checks", apiKey, map[string]any{"checks": batch})
	}
}

func probe(ctx context.Context, hc *http.Client, url string, timeoutMs int, name string) (int, bool, string, int, []CheckLog) {
	var logs []CheckLog
	log := func(level, line string) {
		l := CheckLog{TS: time.Now().UTC().Format(time.RFC3339), Level: level, Line: line}
		logs = append(logs, l)
		fmt.Printf("[%s] %s: %s\n", l.Level, name, l.Line)
	}

	start := time.Now()
	log("trace", "probe start → "+url)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "status-agent/0.1")
	hc.Timeout = time.Duration(timeoutMs) * time.Millisecond

	resp, err := hc.Do(req)
	latency := int(time.Since(start).Milliseconds())

	if err != nil {
		r := classify(err)
		log("error", "transport error: "+err.Error()+" → "+r)
		return 0, false, r, latency, logs
	}
	defer resp.Body.Close()

	log("info", fmt.Sprintf("resp %d in %dms", resp.StatusCode, latency))
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return resp.StatusCode, true, "", latency, logs
	}
	return resp.StatusCode, false, "non_2xx", latency, logs
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

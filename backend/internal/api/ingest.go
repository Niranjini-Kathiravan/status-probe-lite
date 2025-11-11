package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/niranjini-kathiravan/status-probe-lite/backend/internal/store"
)

type IngestHandler struct {
	Store *store.Store
	Logs  *LogsHandler
}

func NewIngestHandler(st *store.Store, logs *LogsHandler) *IngestHandler {
	return &IngestHandler{Store: st, Logs: logs}
}

func (h *IngestHandler) Register(r *gin.Engine) {
	g := r.Group("/api/ingest", RequireAgentKey(h.Store))
	g.POST("/checks", h.checks)
}

// ----- DTOs from agent -----

type checkLogDTO struct {
	TS    string `json:"ts"`    // RFC3339
	Level string `json:"level"` // trace|info|warn|error
	Line  string `json:"line"`
}

type checkDTO struct {
	TargetID   int64         `json:"target_id"`
	TS         string        `json:"ts"` // RFC3339
	StatusCode int           `json:"status_code"`
	OK         bool          `json:"ok"`
	LatencyMs  int           `json:"latency_ms"`
	Error      string        `json:"error"`
	Logs       []checkLogDTO `json:"logs,omitempty"` // NEW
}

type checksReq struct {
	Checks []checkDTO `json:"checks"`
}

func (h *IngestHandler) checks(c *gin.Context) {
	var req checksReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Checks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	for _, x := range req.Checks {
		ts, err := time.Parse(time.RFC3339, x.TS)
		if err != nil {
			continue
		}

		_ = h.Store.InsertCheck(c.Request.Context(), x.TargetID, ts, x.StatusCode, x.OK, x.LatencyMs, x.Error)

		for _, lg := range x.Logs {
			lts, err := time.Parse(time.RFC3339, lg.TS)
			if err != nil {
				continue
			}
			level := normLevel(lg.Level)
			line := truncate(lg.Line, 2000)

			_ = h.Store.InsertCheckLog(c.Request.Context(), x.TargetID, nil, lts, level, line)

			// Live tail via SSE
			if h.Logs != nil {
				b, _ := json.Marshal(struct {
					TS    string `json:"ts"`
					Level string `json:"level"`
					Line  string `json:"line"`
				}{
					TS:    lts.UTC().Format(time.RFC3339),
					Level: level,
					Line:  line,
				})
				h.Logs.Publish(x.TargetID, string(b))
			}
		}

		// Outage stabilization
		h.handleOutage(c, x.TargetID, ts, x.OK, x.Error)
	}

	c.JSON(http.StatusOK, gin.H{"ingested": len(req.Checks)})
}

// open after 2 consecutive fails, close after 2 consecutive ok
func (h *IngestHandler) handleOutage(c *gin.Context, targetID int64, ts time.Time, ok bool, reason string) {
	open, err := h.Store.GetOpenOutage(c.Request.Context(), targetID)
	if err != nil {
		return
	}
	if ok {
		if open != nil {
			recent, err := h.Store.GetRecentChecks(c.Request.Context(), targetID, 2)
			if err == nil && len(recent) >= 2 && recent[0].OK && recent[1].OK {
				_ = h.Store.CloseOutage(c.Request.Context(), open.ID, ts)
			}
		}
		return
	}
	if open == nil {
		recent, err := h.Store.GetRecentChecks(c.Request.Context(), targetID, 2)
		if err == nil && len(recent) >= 2 && !recent[0].OK && !recent[1].OK {
			_ = h.Store.OpenOutage(c.Request.Context(), targetID, ts, reason)
		}
	}
}

// ----- helpers -----

func normLevel(s string) string {
	switch strings.ToLower(s) {
	case "trace", "info", "warn", "warning", "error":
		if s == "warning" {
			return "warn"
		}
		return strings.ToLower(s)
	default:
		return "trace"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n > 10 {
		return s[:n-10] + "...(trunc)"
	}
	return s[:n]
}

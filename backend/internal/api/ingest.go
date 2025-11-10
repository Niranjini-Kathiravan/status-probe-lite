package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/niranjini-kathiravan/status-probe-lite/backend/internal/store"
)

type IngestHandler struct{ Store *store.Store }

func NewIngestHandler(st *store.Store) *IngestHandler { return &IngestHandler{Store: st} }

func (h *IngestHandler) Register(r *gin.Engine) {
	g := r.Group("/api/ingest", RequireAgentKey(h.Store))
	g.POST("/checks", h.checks)
}

type checkDTO struct {
	TargetID   int64  `json:"target_id"`
	TS         string `json:"ts"` // RFC3339
	StatusCode int    `json:"status_code"`
	OK         bool   `json:"ok"`
	LatencyMs  int    `json:"latency_ms"`
	Error      string `json:"error"`
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

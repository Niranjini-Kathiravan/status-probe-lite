package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/niranjini-kathiravan/status-probe-lite/backend/internal/store"
)

type MetricsHandler struct{ Store *store.Store }

func NewMetricsHandler(st *store.Store) *MetricsHandler { return &MetricsHandler{Store: st} }

func (h *MetricsHandler) Register(r *gin.Engine) {
	g := r.Group("/api")
	g.GET("/metrics", h.get)
}

func (h *MetricsHandler) get(c *gin.Context) {
	tidStr := c.Query("target_id")
	if tidStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_id required"})
		return
	}
	tid, err := strconv.ParseInt(tidStr, 10, 64)
	if err != nil || tid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_id"})
		return
	}

	var from, to time.Time
	if c.Query("from") == "" || c.Query("to") == "" {
		to = time.Now().UTC()
		from = to.Add(-60 * time.Minute)
	} else {
		from, err = time.Parse(time.RFC3339, c.Query("from"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad from"})
			return
		}
		to, err = time.Parse(time.RFC3339, c.Query("to"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad to"})
			return
		}
		if !to.After(from) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "to must be after from"})
			return
		}
	}

	total, success, err := h.Store.CountChecksAgg(c.Request.Context(), tid, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count failed"})
		return
	}
	avg, err := h.Store.AvgLatencyOK(c.Request.Context(), tid, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "avg failed"})
		return
	}
	reasons, err := h.Store.FailuresByReason(c.Request.Context(), tid, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reasons failed"})
		return
	}
	outs, err := h.Store.ListOutagesOverlapping(c.Request.Context(), tid, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "outages failed"})
		return
	}

	// clamp outages to window + compute downtime
	type Out struct {
		StartedAt  string  `json:"started_at"`
		EndedAt    *string `json:"ended_at,omitempty"`
		DurationMs int64   `json:"duration_ms"`
		Reason     string  `json:"reason"`
	}
	var outArr []Out
	var downtimeMs int64
	now := time.Now().UTC()
	for _, o := range outs {
		start := o.StartedAt
		if start.Before(from) {
			start = from
		}
		realEnd := to
		if o.EndedAt.Valid && o.EndedAt.Time.Before(to) {
			realEnd = o.EndedAt.Time
		} else if !o.EndedAt.Valid && now.Before(to) {
			realEnd = now
		}
		dur := realEnd.Sub(start)
		if dur < 0 {
			dur = 0
		}
		downtimeMs += dur.Milliseconds()
		var endStr *string
		if o.EndedAt.Valid {
			s := o.EndedAt.Time.UTC().Format(time.RFC3339)
			endStr = &s
		}
		outArr = append(outArr, Out{StartedAt: o.StartedAt.UTC().Format(time.RFC3339), EndedAt: endStr, DurationMs: dur.Milliseconds(), Reason: o.Reason})
	}

	var availPtr *float64
	if total > 0 {
		v := float64(success) / float64(total) * 100
		availPtr = &v
	}
	windowMs := to.Sub(from).Milliseconds()
	var availTimePtr *float64
	if windowMs > 0 {
		v := float64(windowMs-downtimeMs) / float64(windowMs) * 100
		availTimePtr = &v
	}

	failMap := map[string]int64{}
	for _, r := range reasons {
		failMap[r.Reason] = r.Count
	}
	avgLatency := 0
	if avg.Valid {
		avgLatency = int(avg.Float64)
	}

	c.JSON(http.StatusOK, gin.H{
		"target_id":                   tid,
		"from":                        from.UTC().Format(time.RFC3339),
		"to":                          to.UTC().Format(time.RFC3339),
		"availability_percent_checks": availPtr,
		"availability_percent_time":   availTimePtr,
		"total_checks":                total,
		"successful_checks":           success,
		"failed_checks":               total - success,
		"average_latency_ms":          avgLatency,
		"failures_by_reason":          failMap,
		"outages":                     outArr,
		"downtime_ms":                 downtimeMs,
	})
}

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/niranjini-kathiravan/status-probe-lite/backend/internal/store"
)

type LogsHandler struct {
	Store *store.Store
	Hub   *LogHub
}

func NewLogsHandler(st *store.Store) *LogsHandler {
	return &LogsHandler{Store: st, Hub: NewLogHub()}
}

func (h *LogsHandler) Register(r *gin.Engine) {
	g := r.Group("/api")
	g.GET("/logs", h.listLogs)          // history:  GET /api/logs?target_id=1&limit=200&before=RFC3339
	g.GET("/logs/stream", h.streamLogs) // SSE:      GET /api/logs/stream?target_id=1
}

// ----- history -----
func (h *LogsHandler) listLogs(c *gin.Context) {
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
	limit := 200
	if s := c.Query("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	var before *time.Time
	if b := c.Query("before"); b != "" {
		if t, err := time.Parse(time.RFC3339, b); err == nil {
			before = &t
		}
	}

	rows, err := h.Store.ListLogs(c.Request.Context(), tid, limit, before)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list logs failed"})
		return
	}
	type item struct {
		TS    string `json:"ts"`
		Level string `json:"level"`
		Line  string `json:"line"`
	}
	out := make([]item, 0, len(rows))
	for i := range rows {
		out = append(out, item{
			TS:    rows[i].TS.UTC().Format(time.RFC3339),
			Level: rows[i].Level,
			Line:  rows[i].Line,
		})
	}
	c.JSON(http.StatusOK, out)
}

// ----- SSE hub + endpoint -----
type LogEvent struct {
	TargetID int64
	Data     string // JSON line string
}
type LogHub struct {
	subs map[int64]map[chan string]struct{}
	add  chan subReq
	del  chan subReq
	pub  chan LogEvent
}
type subReq struct {
	TargetID int64
	Ch       chan string
}

func NewLogHub() *LogHub {
	h := &LogHub{
		subs: map[int64]map[chan string]struct{}{},
		add:  make(chan subReq),
		del:  make(chan subReq),
		pub:  make(chan LogEvent, 1024),
	}
	go func() {
		for {
			select {
			case r := <-h.add:
				if h.subs[r.TargetID] == nil {
					h.subs[r.TargetID] = map[chan string]struct{}{}
				}
				h.subs[r.TargetID][r.Ch] = struct{}{}
			case r := <-h.del:
				if m := h.subs[r.TargetID]; m != nil {
					delete(m, r.Ch)
				}
			case e := <-h.pub:
				for ch := range h.subs[e.TargetID] {
					select {
					case ch <- e.Data:
					default:
					}
				}
			}
		}
	}()
	return h
}

func (h *LogsHandler) streamLogs(c *gin.Context) {
	tidStr := c.Query("target_id")
	if tidStr == "" {
		c.String(http.StatusBadRequest, "target_id required")
		return
	}
	tid, err := strconv.ParseInt(tidStr, 10, 64)
	if err != nil || tid <= 0 {
		c.String(http.StatusBadRequest, "invalid target_id")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)

	ch := make(chan string, 64)
	h.Hub.add <- subReq{TargetID: tid, Ch: ch}
	defer func() { h.Hub.del <- subReq{TargetID: tid, Ch: ch} }()

	// initial comment to establish stream
	_, _ = c.Writer.Write([]byte(": ok\n\n"))
	flusher.Flush()

	notify := c.Writer.CloseNotify()
	tick := time.NewTicker(25 * time.Second) // keep-alive
	defer tick.Stop()

	for {
		select {
		case <-notify:
			return
		case <-tick.C:
			_, _ = c.Writer.Write([]byte("event: ping\ndata: {}\n\n"))
			flusher.Flush()
		case line := <-ch:
			_, _ = c.Writer.Write([]byte("event: log\ndata: " + line + "\n\n"))
			flusher.Flush()
		}
	}
}

// Publish is called by ingest after inserting a log line.
func (h *LogsHandler) Publish(targetID int64, jsonLine string) {
	h.Hub.pub <- LogEvent{TargetID: targetID, Data: jsonLine}
}

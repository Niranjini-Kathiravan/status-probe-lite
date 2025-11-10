package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/niranjini-kathiravan/status-probe-lite/backend/internal/store"
)

type TargetsHandler struct {
	Store *store.Store
}

func NewTargetsHandler(st *store.Store) *TargetsHandler {
	return &TargetsHandler{Store: st}
}

func (h *TargetsHandler) Register(r *gin.Engine) {
	g := r.Group("/api/targets")
	g.GET("", h.listTargets)
	g.POST("", h.createTarget)
	g.DELETE("/:id", h.deleteTarget)
}

// -------- Handlers --------

func (h *TargetsHandler) listTargets(c *gin.Context) {
	rows, err := h.Store.ListTargets(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list targets"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (h *TargetsHandler) createTarget(c *gin.Context) {
	var req struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		TimeoutMs int    `json:"timeout_ms"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	if !(strings.HasPrefix(req.URL, "http://") || strings.HasPrefix(req.URL, "https://")) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL must start with http:// or https://"})
		return
	}
	if req.TimeoutMs <= 0 {
		req.TimeoutMs = 4000
	}

	id, err := h.Store.InsertTarget(c.Request.Context(), req.Name, req.URL, req.TimeoutMs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create target"})
		return
	}

	out := targetFileEntry{
		ID:        id,
		Name:      req.Name,
		URL:       req.URL,
		TimeoutMs: req.TimeoutMs,
	}

	appendTargetToFile(filepath.Join("cmd", "agent", "targets.json"), out)

	c.JSON(http.StatusCreated, out)
}

func (h *TargetsHandler) deleteTarget(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing id"})
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.Store.DeleteTarget(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}

	removeTargetFromFile(filepath.Join("cmd", "agent", "targets.json"), id)

	c.Status(http.StatusNoContent)
}

type targetFileEntry struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	TimeoutMs int    `json:"timeout_ms"`
}

func appendTargetToFile(path string, newT targetFileEntry) {

	var items []targetFileEntry
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &items)
	}

	items = append(items, newT)

	_ = ensureDir(path)
	data, _ := json.MarshalIndent(items, "", "  ")
	_ = os.WriteFile(path, data, 0644)
	fmt.Printf("→ Added target #%d to %s\n", newT.ID, path)
}

func removeTargetFromFile(path string, id int64) {
	var items []targetFileEntry
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &items)
	}

	filtered := make([]targetFileEntry, 0, len(items))
	for _, t := range items {
		if t.ID != id {
			filtered = append(filtered, t)
		}
	}

	_ = ensureDir(path)
	data, _ := json.MarshalIndent(filtered, "", "  ")
	_ = os.WriteFile(path, data, 0644)
	fmt.Printf("→ Removed target #%d from %s\n", id, path)
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

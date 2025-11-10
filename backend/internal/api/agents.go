package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/niranjini-kathiravan/status-probe-lite/backend/internal/store"
)

type AgentsHandler struct{ Store *store.Store }

func NewAgentsHandler(st *store.Store) *AgentsHandler { return &AgentsHandler{Store: st} }

func (h *AgentsHandler) Register(r *gin.Engine) {
	g := r.Group("/api/agents")
	g.POST("/register", h.register)
}

func (h *AgentsHandler) register(c *gin.Context) {
	type req struct {
		Name string `json:"name"`
	}
	var body req
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	key := randKey(32)
	id, err := h.Store.CreateAgent(c.Request.Context(), body.Name, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agent_id": id, "api_key": key})
}

func RequireAgentKey(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-Api-Key")
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing api key"})
			return
		}
		ag, err := st.FindAgentByKey(c.Request.Context(), key)
		if err != nil || ag == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}
		c.Set("agent_id", ag.ID)
		c.Next()
	}
}

func randKey(n int) string { b := make([]byte, n); _, _ = rand.Read(b); return hex.EncodeToString(b) }

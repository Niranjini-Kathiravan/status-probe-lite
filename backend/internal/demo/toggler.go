package demo

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// simulating outages (switch 200 <-> 503).
type Toggler struct {
	mu  sync.RWMutex
	url string
}

// NewToggler creates a new demo toggler pointing initially to a healthy URL.
func NewToggler(initial string) *Toggler {
	return &Toggler{url: initial}
}

func (t *Toggler) Register(r *gin.Engine) {
	r.GET("/demo/set", func(c *gin.Context) {
		to := c.Query("to")
		if to == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing ?to="})
			return
		}
		t.mu.Lock()
		t.url = to
		t.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{"now_pointing_to": to})
	})

	r.GET("/demo/current", func(c *gin.Context) {
		t.mu.RLock()
		defer t.mu.RUnlock()
		c.JSON(http.StatusOK, gin.H{"current_url": t.url})
	})

	r.GET("/demo/target", func(c *gin.Context) {
		t.mu.RLock()
		defer t.mu.RUnlock()
		c.Redirect(http.StatusTemporaryRedirect, t.url)
	})
}

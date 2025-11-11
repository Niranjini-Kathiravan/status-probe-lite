package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/niranjini-kathiravan/status-probe-lite/backend/internal/api"
	"github.com/niranjini-kathiravan/status-probe-lite/backend/internal/config"
	"github.com/niranjini-kathiravan/status-probe-lite/backend/internal/demo"
	"github.com/niranjini-kathiravan/status-probe-lite/backend/internal/store"
)

func main() {
	cfg := config.Load()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		panic(err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r := gin.Default()
	r.SetTrustedProxies(nil)

	// base
	r.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/version", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"version": cfg.Version}) })

	// core APIs
	api.NewTargetsHandler(st).Register(r)
	api.NewMetricsHandler(st).Register(r)

	// logs: history + SSE
	logs := api.NewLogsHandler(st)
	logs.Register(r)

	// agent registration & ingest
	api.NewAgentsHandler(st).Register(r)       // POST /api/agents/register
	api.NewIngestHandler(st, logs).Register(r) // POST /api/ingest/checks (X-Api-Key)

	// static dashboard
	r.Static("/dashboard", "./internal/web/static")

	// Demo toggler for simulating outages
	demo.NewToggler("https://httpbin.org/status/200").Register(r)

	addr := ":" + cfg.Port
	fmt.Printf("Central on %s (db=%s)\n", addr, cfg.DBPath)
	if err := r.Run(addr); err != nil && err != http.ErrServerClosed {
		panic(err)
	}

	<-ctx.Done()
}

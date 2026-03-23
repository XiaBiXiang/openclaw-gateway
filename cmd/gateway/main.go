package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asleak/openclaw-gateway/internal/config"
	"github.com/asleak/openclaw-gateway/internal/providers"
	cloudprovider "github.com/asleak/openclaw-gateway/internal/providers/cloud"
	localprovider "github.com/asleak/openclaw-gateway/internal/providers/local"
	"github.com/asleak/openclaw-gateway/internal/router"
	"github.com/asleak/openclaw-gateway/internal/server"
	"github.com/asleak/openclaw-gateway/internal/session"
	"github.com/asleak/openclaw-gateway/internal/telemetry"
)

func main() {
	configPath := flag.String("config", "configs/config.example.json", "path to the gateway config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		telemetry.New("error").Error("failed to load config", map[string]any{
			"config_path": *configPath,
			"error":       err.Error(),
		})
		os.Exit(1)
	}

	logger := telemetry.New(cfg.Observability.LogLevel)
	store := session.NewStore()
	decider := router.NewDecider(cfg.Routing, store)

	var localUpstream providers.Provider
	if cfg.Providers.Local.Enabled {
		localUpstream = localprovider.NewProvider(cfg.Providers.Local)
	}

	var cloudUpstream providers.Provider
	if cfg.Providers.Cloud.Enabled {
		cloudUpstream = cloudprovider.NewProvider(cfg.Providers.Cloud)
	}

	httpServer := server.New(cfg, decider, localUpstream, cloudUpstream, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Info("starting gateway", map[string]any{
		"addr":             httpServer.Addr,
		"default_mode":     cfg.Routing.DefaultMode,
		"local_enabled":    cfg.Providers.Local.Enabled,
		"cloud_enabled":    cfg.Providers.Cloud.Enabled,
		"sticky_ttl":       time.Duration(cfg.Routing.StickyTTL).String(),
		"cloud_dwell_time": time.Duration(cfg.Routing.CloudDwellTime).String(),
	})

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, server.ErrServerClosed()) {
		logger.Error("gateway stopped with error", map[string]any{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	logger.Info("gateway stopped", nil)
}

package main

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	agentpkg "github.com/hjma/probex/internal/agent"
	"github.com/hjma/probex/internal/alert"
	"github.com/hjma/probex/internal/api"
	"github.com/hjma/probex/internal/config"
	"github.com/hjma/probex/internal/hub"
	"github.com/hjma/probex/internal/probe"
	"github.com/hjma/probex/internal/report"
	"github.com/hjma/probex/internal/retention"
	"github.com/hjma/probex/internal/store/sqlite"
	"github.com/spf13/cobra"
)

var hubToken string

func hubCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Run in hub mode — accepts agent connections, distributes tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHub()
		},
	}
	cmd.Flags().StringVar(&hubToken, "token", "", "Agent authentication token")
	return cmd
}

func runHub() error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	os.MkdirAll("./data", 0755)

	store, err := sqlite.New(cfg.Storage.SQLite.Path)
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}
	defer store.Close()

	// Registry for metadata only (no local execution in hub mode)
	registry := probe.NewRegistry()
	registry.Register(probe.NewICMPProber())
	registry.Register(probe.NewTCPProber())
	registry.Register(probe.NewHTTPProber())
	registry.Register(probe.NewDNSProber())
	registry.Register(probe.NewIperf3Prober())
	probe.LoadScripts(cfg.Probe.ScriptDir, registry, logger)

	// Restore external probes
	extProbes, _ := store.ListProbes(context.Background())
	for _, ep := range extProbes {
		var outSchema *probe.OutputSchema
		json.Unmarshal(ep.OutputSchema, &outSchema)
		registry.RegisterExternal(probe.ProbeMetadata{
			Name: ep.Name, Kind: probe.ProbeKindExternal,
			Description: ep.Description, ParameterSchema: ep.ParameterSchema, OutputSchema: outSchema,
		})
	}

	reportGen := report.NewGenerator(store, "./data", logger)
	alertEval := alert.NewEvaluator(store, logger)

	// Token from flag or config
	token := hubToken
	if token == "" {
		token = cfg.Hub.Token
	}
	if token == "" {
		// Generate a random token if not configured
		token = generateRandomToken()
		logger.Info("generated hub token (use this for agent connections)", "token", token)
	}

	// WebSocket manager for agent connections
	wsManager := hub.NewWSManager(store, alertEval, registry, token, logger)

	// Hub uses HubNotifier: task changes pushed to agents via WebSocket
	notifier := hub.NewHubNotifier(wsManager)

	srv := api.NewServer(store, notifier, registry, reportGen, alertEval,
		api.WithMode("hub"),
		api.WithAllowedNetworks(cfg.Server.AllowedNetworks),
	)
	srv.SetWSManager(wsManager)

	// Background workers
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	go agentpkg.NewMonitor(store, logger).Run(bgCtx)
	go retention.NewWorker(store, cfg.Retention.RawResults, logger).Run(bgCtx)

	httpServer := &http.Server{Addr: cfg.Server.HTTPAddr, Handler: srv.Handler()}
	go func() {
		logger.Info("starting hub", "addr", cfg.Server.HTTPAddr, "mode", "hub")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server", "error", err)
			os.Exit(1)
		}
	}()

	printBanner(cfg.Server.HTTPAddr, "hub")
	fmt.Printf("  Token: %s\n\n", token)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down hub...")
	bgCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(ctx)
}

func generateRandomToken() string {
	b := make([]byte, 16)
	crand.Read(b)
	return fmt.Sprintf("pxhub_%x", b)
}

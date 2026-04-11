package main

import (
	"context"
	"encoding/json"
	"flag"
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
	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/probe"
	"github.com/hjma/probex/internal/report"
	"github.com/hjma/probex/internal/retention"
	"github.com/hjma/probex/internal/store/sqlite"
)

func main() {
	cfgPath := flag.String("config", "", "config file path")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	// Ensure data directory exists
	os.MkdirAll("./data", 0755)

	// Initialize store
	store, err := sqlite.New(cfg.Storage.SQLite.Path)
	if err != nil {
		logger.Error("init store", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize probe registry
	registry := probe.NewRegistry()
	registry.Register(probe.NewICMPProber())
	registry.Register(probe.NewTCPProber())
	registry.Register(probe.NewHTTPProber())
	registry.Register(probe.NewDNSProber())
	registry.Register(probe.NewIperf3Prober())

	// Load script probes from configured directory
	probe.LoadScripts(cfg.Probe.ScriptDir, registry, logger)

	// Restore externally registered probes from DB
	extProbes, _ := store.ListProbes(context.Background())
	for _, ep := range extProbes {
		var outSchema *probe.OutputSchema
		json.Unmarshal(ep.OutputSchema, &outSchema)
		registry.RegisterExternal(probe.ProbeMetadata{
			Name:            ep.Name,
			Kind:            probe.ProbeKindExternal,
			Description:     ep.Description,
			ParameterSchema: ep.ParameterSchema,
			OutputSchema:    outSchema,
		})
	}
	if len(extProbes) > 0 {
		logger.Info("restored external probes", "count", len(extProbes))
	}

	// Register local agent
	localAgent := &model.Agent{
		ID:            "local",
		Name:          cfg.Agent.Name,
		Labels:        json.RawMessage("{}"),
		Address:       "localhost",
		Plugins:       registry.List(),
		Status:        model.AgentStatusHealthy,
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
	}
	store.UpsertAgent(context.Background(), localAgent)

	// Initialize report generator
	reportGen := report.NewGenerator(store, "./data", logger)

	// Initialize alert evaluator
	alertEval := alert.NewEvaluator(store, logger)

	// Initialize runner with result handler that persists to store and evaluates alerts
	runner := probe.NewRunner(registry, "local", cfg.Runner.MaxConcurrent, func(result *model.ProbeResult) {
		if err := store.InsertResult(context.Background(), result); err != nil {
			logger.Error("insert result", "error", err)
		}
		alertEval.Evaluate(result)
	}, logger)
	defer runner.Stop()

	// Restore enabled tasks from DB
	enabled := true
	tasks, _, err := store.ListTasks(context.Background(), model.TaskFilter{Enabled: &enabled, Limit: 10000})
	if err != nil {
		logger.Error("load tasks", "error", err)
	}
	for _, t := range tasks {
		runner.AddTask(t)
	}
	logger.Info("restored tasks", "count", len(tasks))

	// Start background workers
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	// Agent health monitor
	agentMonitor := agentpkg.NewMonitor(store, logger)
	go agentMonitor.Run(bgCtx)
	logger.Info("agent health monitor started")

	// Data retention worker
	retentionWorker := retention.NewWorker(store, cfg.Retention.RawResults, logger)
	go retentionWorker.Run(bgCtx)
	logger.Info("retention worker started", "retention", cfg.Retention.RawResults)

	// Start API server
	srv := api.NewServer(store, runner, registry, reportGen, alertEval)
	httpServer := &http.Server{
		Addr:    cfg.Server.HTTPAddr,
		Handler: srv.Handler(),
	}

	go func() {
		logger.Info("starting HTTP server", "addr", cfg.Server.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server", "error", err)
			os.Exit(1)
		}
	}()

	fmt.Printf(`
  ____            _         __  __
 |  _ \ _ __ ___ | |__   ___\ \/ /
 | |_) | '__/ _ \| '_ \ / _ \\  /
 |  __/| | | (_) | |_) |  __//  \
 |_|   |_|  \___/|_.__/ \___/_/\_\

  Controller running on %s
  API: http://localhost%s/api/v1
  Health: http://localhost%s/health

`, cfg.Server.HTTPAddr, cfg.Server.HTTPAddr, cfg.Server.HTTPAddr)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
	bgCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
}

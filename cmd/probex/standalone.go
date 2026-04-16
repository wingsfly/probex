package main

import (
	"context"
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
	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/probe"
	"github.com/hjma/probex/internal/report"
	"github.com/hjma/probex/internal/retention"
	"github.com/hjma/probex/internal/store/sqlite"
	"github.com/spf13/cobra"
)

func standaloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "standalone",
		Short: "Run in standalone mode (default) — single-node hub + local agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStandalone()
		},
	}
}

func runStandalone() error {
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

	registry := probe.NewRegistry()
	registry.Register(probe.NewICMPProber())
	registry.Register(probe.NewTCPProber())
	registry.Register(probe.NewHTTPProber())
	registry.Register(probe.NewDNSProber())
	registry.Register(probe.NewIperf3Prober())

	probe.LoadScripts(cfg.Probe.ScriptDir, registry, logger)

	extProbes, _ := store.ListProbes(context.Background())
	for _, ep := range extProbes {
		var outSchema *probe.OutputSchema
		json.Unmarshal(ep.OutputSchema, &outSchema)
		registry.RegisterExternal(probe.ProbeMetadata{
			Name: ep.Name, Kind: probe.ProbeKindExternal,
			Description: ep.Description, ParameterSchema: ep.ParameterSchema, OutputSchema: outSchema,
		})
	}

	localAgent := &model.Agent{
		ID: "local", Name: cfg.Agent.Name, Labels: json.RawMessage("{}"),
		Address: "localhost", Plugins: registry.List(),
		Status: model.AgentStatusHealthy, LastHeartbeat: time.Now(), RegisteredAt: time.Now(),
	}
	store.UpsertAgent(context.Background(), localAgent)

	reportGen := report.NewGenerator(store, "./data", logger)
	alertEval := alert.NewEvaluator(store, logger)

	runner := probe.NewRunner(registry, "local", cfg.Runner.MaxConcurrent, func(result *model.ProbeResult) {
		if err := store.InsertResult(context.Background(), result); err != nil {
			logger.Error("insert result", "error", err)
		}
		alertEval.Evaluate(result)
	}, logger)
	defer runner.Stop()

	enabled := true
	tasks, _, _ := store.ListTasks(context.Background(), model.TaskFilter{Enabled: &enabled, Limit: 10000})
	for _, t := range tasks {
		runner.AddTask(t)
	}
	logger.Info("restored tasks", "count", len(tasks))

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	go agentpkg.NewMonitor(store, logger).Run(bgCtx)
	go retention.NewWorker(store, cfg.Retention.RawResults, logger).Run(bgCtx)

	notifier := api.NewRunnerNotifier(runner)
	srv := api.NewServer(store, notifier, registry, reportGen, alertEval,
		api.WithMode("standalone"),
		api.WithAllowedNetworks(cfg.Server.AllowedNetworks),
	)

	httpServer := &http.Server{Addr: cfg.Server.HTTPAddr, Handler: srv.Handler()}
	go func() {
		logger.Info("starting HTTP server", "addr", cfg.Server.HTTPAddr, "mode", "standalone")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server", "error", err)
			os.Exit(1)
		}
	}()

	printBanner(cfg.Server.HTTPAddr, "standalone")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
	bgCancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(ctx)
}

func printBanner(addr, mode string) {
	fmt.Printf(`
  ____            _         __  __
 |  _ \ _ __ ___ | |__   ___\ \/ /
 | |_) | '__/ _ \| '_ \ / _ \\  /
 |  __/| | | (_) | |_) |  __//  \
 |_|   |_|  \___/|_.__/ \___/_/\_\

  Mode: %s
  API:        http://localhost%s/api/v1
  Health:     http://localhost%s/health
  Frontend:   http://localhost:3000  (run: cd web && npm run dev)
  Note:       :8080 serves backend API only (no web page at "/")

`, mode, addr, addr)
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	agentpkg "github.com/hjma/probex/internal/agent"
	"github.com/hjma/probex/internal/config"
	"github.com/hjma/probex/internal/probe"
	"github.com/spf13/cobra"
)

func agentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run in agent mode — connects to hub, executes probes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDistAgent(cmd)
		},
	}
	cmd.Flags().String("hub", "", "Hub WebSocket URL (ws://host:8080/api/v1/ws/agent)")
	cmd.Flags().String("token", "", "Authentication token")
	cmd.Flags().String("name", "", "Agent name")
	cmd.Flags().String("labels", "{}", "Agent labels as JSON")
	return cmd
}

func runDistAgent(cmd *cobra.Command) error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// CLI flags override config
	if v, _ := cmd.Flags().GetString("hub"); v != "" {
		cfg.Connect.HubURL = v
	}
	if v, _ := cmd.Flags().GetString("token"); v != "" {
		cfg.Connect.Token = v
	}
	if v, _ := cmd.Flags().GetString("name"); v != "" {
		cfg.Connect.Name = v
	}
	if v, _ := cmd.Flags().GetString("labels"); v != "" && v != "{}" {
		json.Unmarshal([]byte(v), &cfg.Connect.Labels)
	}

	if cfg.Connect.HubURL == "" {
		return fmt.Errorf("hub URL is required (--hub or connect.hub_url in config)")
	}
	if cfg.Connect.Name == "" {
		hostname, _ := os.Hostname()
		cfg.Connect.Name = hostname
	}
	if cfg.Connect.Labels == nil {
		cfg.Connect.Labels = map[string]string{}
	}

	// Initialize probe registry
	registry := probe.NewRegistry()
	registry.Register(probe.NewICMPProber())
	registry.Register(probe.NewTCPProber())
	registry.Register(probe.NewHTTPProber())
	registry.Register(probe.NewDNSProber())
	registry.Register(probe.NewIperf3Prober())
	probe.LoadScripts(cfg.Probe.ScriptDir, registry, logger)

	// Create WebSocket client
	wsClient := agentpkg.NewWSClient(
		cfg.Connect.HubURL, cfg.Connect.Token, cfg.Connect.Name, cfg.Connect.Labels,
		nil, // runner created below
		registry, logger,
	)

	// Create runner with result handler that feeds into WS client
	runner := probe.NewRunner(registry, cfg.Connect.Name, cfg.Runner.MaxConcurrent, wsClient.ResultHandler(), logger)
	defer runner.Stop()

	// Set runner on client (circular dependency resolved here)
	wsClient.SetRunner(runner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Printf(`
  ProbeX Agent
  Name: %s
  Hub:  %s

`, cfg.Connect.Name, cfg.Connect.HubURL)

	// Run WebSocket client (reconnects automatically)
	go wsClient.Run(ctx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down agent...")
	cancel()
	return nil
}

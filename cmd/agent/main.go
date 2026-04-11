package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hjma/probex/internal/config"
	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/probe"
)

func main() {
	cfgPath := flag.String("config", "", "agent config file path")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.LoadAgentConfig(*cfgPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	// Initialize probe registry
	registry := probe.NewRegistry()
	registry.Register(probe.NewICMPProber())
	registry.Register(probe.NewTCPProber())
	registry.Register(probe.NewHTTPProber())
	registry.Register(probe.NewDNSProber())
	registry.Register(probe.NewIperf3Prober())

	// Load script probes
	probe.LoadScripts("./scripts/probes", registry, logger)

	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := cfg.ControllerURL + "/api/v1"

	// Register with controller
	agentID, err := register(client, baseURL, cfg, registry.List())
	if err != nil {
		logger.Error("register failed", "error", err)
		os.Exit(1)
	}
	logger.Info("registered with controller", "agent_id", agentID, "name", cfg.Agent.Name)

	fmt.Printf(`
  ProbeX Agent: %s
  Controller:   %s
  Agent ID:     %s

`, cfg.Agent.Name, cfg.ControllerURL, agentID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agent := &agentRunner{
		id:       agentID,
		cfg:      cfg,
		client:   client,
		baseURL:  baseURL,
		registry: registry,
		logger:   logger,
		tasks:    make(map[string]*runningTask),
	}

	// Start heartbeat loop
	go agent.heartbeatLoop(ctx)

	// Start task polling loop
	go agent.pollLoop(ctx)

	// Wait for shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down agent...")
	cancel()
}

type agentRunner struct {
	id       string
	cfg      *config.AgentClientConfig
	client   *http.Client
	baseURL  string
	registry *probe.Registry
	logger   *slog.Logger

	mu    sync.Mutex
	tasks map[string]*runningTask
}

type runningTask struct {
	task   *model.Task
	cancel context.CancelFunc
}

func register(client *http.Client, baseURL string, cfg *config.AgentClientConfig, plugins []string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"name":    cfg.Agent.Name,
		"address": getLocalIP(),
		"labels":  cfg.Agent.Labels,
		"plugins": plugins,
	})
	resp, err := client.Post(baseURL+"/agents/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("POST register: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("register error: %s", result.Error)
	}
	return result.Data.ID, nil
}

func (a *agentRunner) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resp, err := a.client.Post(a.baseURL+"/agents/"+a.id+"/heartbeat", "application/json", nil)
			if err != nil {
				a.logger.Error("heartbeat failed", "error", err)
				continue
			}
			resp.Body.Close()
		}
	}
}

func (a *agentRunner) pollLoop(ctx context.Context) {
	// Initial poll
	a.syncTasks(ctx)

	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			for id, rt := range a.tasks {
				rt.cancel()
				delete(a.tasks, id)
			}
			a.mu.Unlock()
			return
		case <-ticker.C:
			a.syncTasks(ctx)
		}
	}
}

func (a *agentRunner) syncTasks(ctx context.Context) {
	resp, err := a.client.Get(a.baseURL + "/agents/" + a.id + "/tasks")
	if err != nil {
		a.logger.Error("poll tasks failed", "error", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Data []*model.Task `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		a.logger.Error("decode tasks", "error", err)
		return
	}

	remoteTasks := make(map[string]*model.Task)
	for _, t := range result.Data {
		remoteTasks[t.ID] = t
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Remove tasks no longer assigned
	for id, rt := range a.tasks {
		if _, ok := remoteTasks[id]; !ok {
			rt.cancel()
			delete(a.tasks, id)
			a.logger.Info("task removed", "task_id", id)
		}
	}

	// Add/update tasks
	for id, t := range remoteTasks {
		if _, ok := a.tasks[id]; !ok {
			taskCtx, cancel := context.WithCancel(ctx)
			a.tasks[id] = &runningTask{task: t, cancel: cancel}
			go a.runTaskLoop(taskCtx, t)
			a.logger.Info("task started", "task_id", id, "target", t.Target, "probe", t.ProbeType)
		}
	}
}

func (a *agentRunner) runTaskLoop(ctx context.Context, task *model.Task) {
	// Run immediately
	a.executeAndPush(ctx, task)

	ticker := time.NewTicker(task.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.executeAndPush(ctx, task)
		}
	}
}

func (a *agentRunner) executeAndPush(ctx context.Context, task *model.Task) {
	prober, err := a.registry.Get(task.ProbeType)
	if err != nil {
		a.logger.Error("probe not found", "task_id", task.ID, "probe_type", task.ProbeType)
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, task.Timeout)
	defer cancel()

	result, err := prober.Probe(probeCtx, task.Target, task.Config)
	if err != nil {
		a.logger.Error("probe error", "task_id", task.ID, "error", err)
		result = &probe.Result{Success: false, Error: err.Error()}
	}

	pr := &model.ProbeResult{
		ID:             generateID(),
		TaskID:         task.ID,
		AgentID:        a.id,
		Timestamp:      time.Now(),
		Success:        result.Success,
		LatencyMs:      &result.LatencyMs,
		JitterMs:       result.JitterMs,
		PacketLossPct:  result.PacketLossPct,
		DNSResolveMs:   result.DNSResolveMs,
		TLSHandshakeMs: result.TLSHandshakeMs,
		StatusCode:     result.StatusCode,
		DownloadBps:    result.DownloadBps,
		UploadBps:      result.UploadBps,
		Error:          result.Error,
		Extra:          result.Extra,
	}

	body, _ := json.Marshal([]*model.ProbeResult{pr})
	resp, err := a.client.Post(a.baseURL+"/agents/"+a.id+"/results", "application/json", bytes.NewReader(body))
	if err != nil {
		a.logger.Error("push result failed", "task_id", task.ID, "error", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "unknown"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "unknown"
}

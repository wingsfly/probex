package agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/probe"
	"github.com/hjma/probex/internal/protocol"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// WSClient connects to a hub and executes tasks received via WebSocket.
type WSClient struct {
	hubURL   string
	token    string
	agentID  string
	name     string
	labels   map[string]string
	runner   *probe.Runner
	registry *probe.Registry
	logger   *slog.Logger

	resultCh    chan *model.ProbeResult
	startTime   time.Time
	reconnectMs int64

	mu    sync.Mutex
	tasks map[string]bool // track assigned task IDs
}

func NewWSClient(hubURL, token, name string, labels map[string]string,
	runner *probe.Runner, registry *probe.Registry, logger *slog.Logger) *WSClient {

	return &WSClient{
		hubURL:      hubURL,
		token:       token,
		agentID:     generateAgentID(),
		name:        name,
		labels:      labels,
		runner:      runner,
		registry:    registry,
		logger:      logger,
		resultCh:    make(chan *model.ProbeResult, 256),
		startTime:   time.Now(),
		reconnectMs: 1000,
		tasks:       make(map[string]bool),
	}
}

// SetRunner sets the runner (resolves circular init dependency).
func (c *WSClient) SetRunner(r *probe.Runner) {
	c.runner = r
}

// ResultHandler returns a probe.ResultHandler that enqueues results for batch sending.
func (c *WSClient) ResultHandler() probe.ResultHandler {
	return func(result *model.ProbeResult) {
		select {
		case c.resultCh <- result:
		default:
			c.logger.Warn("result channel full, dropping result")
		}
	}
}

// Run connects to the hub and maintains the connection with exponential backoff.
func (c *WSClient) Run(ctx context.Context) {
	for {
		err := c.connect(ctx)
		if ctx.Err() != nil {
			return // context canceled, shutting down
		}
		c.logger.Error("hub connection lost", "error", err)

		// Exponential backoff
		delay := time.Duration(c.reconnectMs) * time.Millisecond
		c.logger.Info("reconnecting", "delay", delay)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
		c.reconnectMs = min(c.reconnectMs*2, 60000)
	}
}

func (c *WSClient) connect(ctx context.Context) error {
	// Build URL with auth params
	u, err := url.Parse(c.hubURL)
	if err != nil {
		return fmt.Errorf("parse hub url: %w", err)
	}
	q := u.Query()
	q.Set("token", c.token)
	q.Set("name", c.name)
	labelsJSON, _ := json.Marshal(c.labels)
	q.Set("labels", string(labelsJSON))
	u.RawQuery = q.Encode()

	conn, _, err := websocket.Dial(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial hub: %w", err)
	}
	defer conn.CloseNow()

	c.logger.Info("connected to hub", "url", c.hubURL, "name", c.name)
	c.reconnectMs = 1000 // reset backoff on successful connect

	// Start writer goroutine
	writerCtx, writerCancel := context.WithCancel(ctx)
	defer writerCancel()
	go c.writeLoop(writerCtx, conn)

	// Read loop (blocking)
	return c.readLoop(ctx, conn)
}

func (c *WSClient) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		var env protocol.Envelope
		err := wsjson.Read(ctx, conn, &env)
		if err != nil {
			return err
		}
		c.handleMessage(env)
	}
}

func (c *WSClient) writeLoop(ctx context.Context, conn *websocket.Conn) {
	heartbeatTicker := time.NewTicker(30 * time.Second)
	batchTicker := time.NewTicker(5 * time.Second)
	defer heartbeatTicker.Stop()
	defer batchTicker.Stop()

	var resultBuf []*model.ProbeResult

	for {
		select {
		case r := <-c.resultCh:
			resultBuf = append(resultBuf, r)
			// Flush immediately if buffer is large
			if len(resultBuf) >= 50 {
				c.sendResults(ctx, conn, resultBuf)
				resultBuf = nil
			}

		case <-batchTicker.C:
			if len(resultBuf) > 0 {
				c.sendResults(ctx, conn, resultBuf)
				resultBuf = nil
			}

		case <-heartbeatTicker.C:
			c.sendHeartbeat(ctx, conn)

		case <-ctx.Done():
			// Flush remaining
			if len(resultBuf) > 0 {
				c.sendResults(context.Background(), conn, resultBuf)
			}
			return
		}
	}
}

func (c *WSClient) sendResults(ctx context.Context, conn *websocket.Conn, results []*model.ProbeResult) {
	payload := protocol.ResultBatchPayload{Results: results}
	env, _ := protocol.NewEnvelope(protocol.MsgResultBatch, payload)
	if err := wsjson.Write(ctx, conn, env); err != nil {
		c.logger.Error("send results", "count", len(results), "error", err)
	}
}

func (c *WSClient) sendHeartbeat(ctx context.Context, conn *websocket.Conn) {
	c.mu.Lock()
	taskCount := len(c.tasks)
	c.mu.Unlock()

	payload := protocol.HeartbeatPayload{
		AgentID:   c.agentID,
		Name:      c.name,
		UptimeSec: int64(time.Since(c.startTime).Seconds()),
		TaskCount: taskCount,
		Timestamp: time.Now(),
	}
	env, _ := protocol.NewEnvelope(protocol.MsgHeartbeat, payload)
	wsjson.Write(ctx, conn, env)
}

func (c *WSClient) handleMessage(env protocol.Envelope) {
	switch env.Type {
	case protocol.MsgTaskSync:
		var payload protocol.TaskSyncPayload
		if err := env.DecodePayload(&payload); err != nil {
			c.logger.Error("decode task_sync", "error", err)
			return
		}
		c.handleTaskSync(payload)

	case protocol.MsgTaskUpdate:
		var payload protocol.TaskUpdatePayload
		if err := env.DecodePayload(&payload); err != nil {
			return
		}
		c.runner.AddTask(payload.Task)
		c.mu.Lock()
		c.tasks[payload.Task.ID] = true
		c.mu.Unlock()
		c.logger.Info("task updated", "id", payload.Task.ID, "name", payload.Task.Name)

	case protocol.MsgTaskDelete:
		var payload protocol.TaskDeletePayload
		if err := env.DecodePayload(&payload); err != nil {
			return
		}
		c.runner.RemoveTask(payload.TaskID)
		c.mu.Lock()
		delete(c.tasks, payload.TaskID)
		c.mu.Unlock()
		c.logger.Info("task removed", "id", payload.TaskID)

	case protocol.MsgPing:
		// Pong is handled automatically by writeLoop heartbeat

	case protocol.MsgScriptSync:
		// TODO: script manager integration
		c.logger.Debug("script_sync received (not yet implemented)")
	}
}

func (c *WSClient) handleTaskSync(payload protocol.TaskSyncPayload) {
	c.mu.Lock()
	// Remove tasks no longer assigned
	newIDs := make(map[string]bool)
	for _, t := range payload.Tasks {
		newIDs[t.ID] = true
	}
	for id := range c.tasks {
		if !newIDs[id] {
			c.runner.RemoveTask(id)
			delete(c.tasks, id)
		}
	}
	// Add/update tasks
	for _, t := range payload.Tasks {
		c.tasks[t.ID] = true
		c.runner.AddTask(t) // AddTask handles update (cancels old, starts new)
	}
	c.mu.Unlock()
	c.logger.Info("task sync", "count", len(payload.Tasks))
}

func generateAgentID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

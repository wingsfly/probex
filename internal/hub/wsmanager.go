package hub

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/probe"
	"github.com/hjma/probex/internal/protocol"
	"github.com/hjma/probex/internal/store"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// AlertEvaluator evaluates probe results against alert rules.
type AlertEvaluator interface {
	Evaluate(result *model.ProbeResult)
}

// AgentConn represents a connected remote agent.
type AgentConn struct {
	AgentID string
	Name    string
	Labels  map[string]string
	Conn    *websocket.Conn
	Send    chan protocol.Envelope
}

// WSManager manages WebSocket connections from remote agents.
type WSManager struct {
	store     store.Store
	alertEval AlertEvaluator
	registry  *probe.Registry
	token     string
	logger    *slog.Logger

	mu     sync.RWMutex
	agents map[string]*AgentConn // agentID -> conn

	// SSE broker (optional, set after creation)
	onResult func(*model.ProbeResult)
}

func NewWSManager(s store.Store, alertEval AlertEvaluator, reg *probe.Registry, token string, logger *slog.Logger) *WSManager {
	return &WSManager{
		store:     s,
		alertEval: alertEval,
		registry:  reg,
		token:     token,
		logger:    logger,
		agents:    make(map[string]*AgentConn),
	}
}

// SetOnResult sets a callback for when results are received (e.g., SSE broadcast).
func (m *WSManager) SetOnResult(fn func(*model.ProbeResult)) {
	m.onResult = fn
}

// HandleUpgrade is the HTTP handler for agent WebSocket connections.
// Route: GET /api/v1/ws/agent?token=...&name=...&labels=...
func (m *WSManager) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Authenticate
	if m.token != "" && q.Get("token") != m.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	name := q.Get("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	var labels map[string]string
	if l := q.Get("labels"); l != "" {
		json.Unmarshal([]byte(l), &labels)
	}
	if labels == nil {
		labels = map[string]string{}
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		m.logger.Error("ws accept", "error", err)
		return
	}

	// Register agent in DB
	labelsJSON, _ := json.Marshal(labels)
	agentID := generateID()
	now := time.Now()
	dbAgent := &model.Agent{
		ID: agentID, Name: name, Labels: labelsJSON,
		Address: r.RemoteAddr, Plugins: m.registry.List(),
		Status: model.AgentStatusHealthy, LastHeartbeat: now, RegisteredAt: now,
	}
	m.store.UpsertAgent(r.Context(), dbAgent)

	ac := &AgentConn{
		AgentID: agentID,
		Name:    name,
		Labels:  labels,
		Conn:    conn,
		Send:    make(chan protocol.Envelope, 64),
	}

	m.mu.Lock()
	m.agents[agentID] = ac
	m.mu.Unlock()

	m.logger.Info("agent connected", "id", agentID, "name", name, "addr", r.RemoteAddr)

	// Send initial task sync
	go m.sendTaskSync(context.Background(), ac)

	// Start read/write pumps
	ctx := r.Context()
	go m.writePump(ctx, ac)
	m.readPump(ctx, ac) // blocking

	// Cleanup on disconnect
	m.mu.Lock()
	delete(m.agents, agentID)
	m.mu.Unlock()
	close(ac.Send)

	m.logger.Info("agent disconnected", "id", agentID, "name", name)
}

func (m *WSManager) readPump(ctx context.Context, ac *AgentConn) {
	defer ac.Conn.CloseNow()
	for {
		var env protocol.Envelope
		err := wsjson.Read(ctx, ac.Conn, &env)
		if err != nil {
			if websocket.CloseStatus(err) != -1 {
				m.logger.Debug("agent ws closed", "id", ac.AgentID, "status", websocket.CloseStatus(err))
			} else {
				m.logger.Error("agent ws read", "id", ac.AgentID, "error", err)
			}
			return
		}
		m.handleAgentMessage(ctx, ac, env)
	}
}

func (m *WSManager) writePump(ctx context.Context, ac *AgentConn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case env, ok := <-ac.Send:
			if !ok {
				return
			}
			if err := wsjson.Write(ctx, ac.Conn, env); err != nil {
				m.logger.Error("agent ws write", "id", ac.AgentID, "error", err)
				return
			}
		case <-ticker.C:
			// Send ping
			env, _ := protocol.NewEnvelope(protocol.MsgPing, nil)
			if err := wsjson.Write(ctx, ac.Conn, env); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (m *WSManager) handleAgentMessage(ctx context.Context, ac *AgentConn, env protocol.Envelope) {
	switch env.Type {
	case protocol.MsgResultBatch:
		var payload protocol.ResultBatchPayload
		if err := env.DecodePayload(&payload); err != nil {
			m.logger.Error("decode result_batch", "error", err)
			return
		}
		// Set agent_id on all results
		for _, r := range payload.Results {
			r.AgentID = ac.AgentID
		}
		if err := m.store.InsertResults(ctx, payload.Results); err != nil {
			m.logger.Error("insert results", "agent", ac.Name, "error", err)
			return
		}
		// Evaluate alerts + notify SSE
		for _, r := range payload.Results {
			if m.alertEval != nil {
				m.alertEval.Evaluate(r)
			}
			if m.onResult != nil {
				m.onResult(r)
			}
		}

	case protocol.MsgHeartbeat:
		var payload protocol.HeartbeatPayload
		if err := env.DecodePayload(&payload); err != nil {
			return
		}
		m.store.UpdateAgentStatus(ctx, ac.AgentID, model.AgentStatusHealthy)

	case protocol.MsgPong:
		// No action needed
	}
}

// sendTaskSync sends the full task + script state to a newly connected agent.
func (m *WSManager) sendTaskSync(ctx context.Context, ac *AgentConn) {
	enabled := true
	tasks, _, err := m.store.ListTasks(ctx, model.TaskFilter{Enabled: &enabled, Limit: 10000})
	if err != nil {
		m.logger.Error("list tasks for sync", "error", err)
		return
	}

	// Build agent model for matching
	labelsJSON, _ := json.Marshal(ac.Labels)
	agent := &model.Agent{ID: ac.AgentID, Name: ac.Name, Labels: labelsJSON}
	matched := probe.FilterTasksForAgent(tasks, agent)

	// Build script list
	scripts := m.buildScriptList()

	payload := protocol.TaskSyncPayload{Tasks: matched, Scripts: scripts}
	env, _ := protocol.NewEnvelope(protocol.MsgTaskSync, payload)

	select {
	case ac.Send <- env:
	default:
		m.logger.Warn("agent send buffer full", "id", ac.AgentID)
	}
}

// buildScriptList returns ScriptInfo for all script probes in the registry.
func (m *WSManager) buildScriptList() []protocol.ScriptInfo {
	var scripts []protocol.ScriptInfo
	for _, meta := range m.registry.ListMetadata() {
		if meta.Kind == probe.ProbeKindScript {
			scripts = append(scripts, protocol.ScriptInfo{
				Name: meta.Name,
				// TODO: read file content and compute hash for distribution
			})
		}
	}
	return scripts
}

// BroadcastTaskUpdate sends a task update to all matching connected agents.
func (m *WSManager) BroadcastTaskUpdate(task *model.Task) {
	env, _ := protocol.NewEnvelope(protocol.MsgTaskUpdate, protocol.TaskUpdatePayload{Task: task})
	m.broadcastToMatching(task, env)
}

// BroadcastTaskDelete notifies all agents to stop a task.
func (m *WSManager) BroadcastTaskDelete(taskID string) {
	env, _ := protocol.NewEnvelope(protocol.MsgTaskDelete, protocol.TaskDeletePayload{TaskID: taskID})
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ac := range m.agents {
		select {
		case ac.Send <- env:
		default:
		}
	}
}

func (m *WSManager) broadcastToMatching(task *model.Task, env protocol.Envelope) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ac := range m.agents {
		labelsJSON, _ := json.Marshal(ac.Labels)
		agent := &model.Agent{ID: ac.AgentID, Name: ac.Name, Labels: labelsJSON}
		if probe.MatchAgent(task, agent) {
			select {
			case ac.Send <- env:
			default:
			}
		}
	}
}

// ConnectedCount returns the number of connected agents.
func (m *WSManager) ConnectedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.agents)
}

func generateID() string {
	b := make([]byte, 16)
	crand.Read(b)
	return fmt.Sprintf("%x", b)
}

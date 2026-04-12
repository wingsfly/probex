package protocol

import (
	"encoding/json"
	"time"

	"github.com/hjma/probex/internal/model"
)

// MsgType identifies the type of WebSocket message.
type MsgType string

const (
	// Hub → Agent
	MsgTaskSync     MsgType = "task_sync"     // Full task + script state on connect
	MsgTaskUpdate   MsgType = "task_update"   // Single task created/updated
	MsgTaskDelete   MsgType = "task_delete"   // Task removed
	MsgScriptSync   MsgType = "script_sync"   // Full script list
	MsgScriptUpdate MsgType = "script_update" // Single script push
	MsgPing         MsgType = "ping"          // Keepalive

	// Agent → Hub
	MsgResultBatch MsgType = "result_batch" // Batch of probe results
	MsgHeartbeat   MsgType = "heartbeat"    // Agent status
	MsgPong        MsgType = "pong"         // Keepalive reply
)

// Envelope is the top-level WebSocket message wrapper.
type Envelope struct {
	Type    MsgType         `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// --- Hub → Agent payloads ---

// TaskSyncPayload is sent when an agent first connects.
// Contains all enabled tasks matching this agent + all script probes.
type TaskSyncPayload struct {
	Tasks   []*model.Task `json:"tasks"`
	Scripts []ScriptInfo  `json:"scripts"`
}

// TaskUpdatePayload notifies agent of a single task change.
type TaskUpdatePayload struct {
	Task *model.Task `json:"task"`
}

// TaskDeletePayload notifies agent to stop a task.
type TaskDeletePayload struct {
	TaskID string `json:"task_id"`
}

// ScriptInfo describes a script probe for distribution.
type ScriptInfo struct {
	Name    string `json:"name"`
	Hash    string `json:"hash"`              // sha256 of content
	Content string `json:"content,omitempty"` // only sent when agent needs update
}

// ScriptSyncPayload is the full list of available scripts.
type ScriptSyncPayload struct {
	Scripts []ScriptInfo `json:"scripts"`
}

// --- Agent → Hub payloads ---

// ResultBatchPayload carries probe results from agent to hub.
type ResultBatchPayload struct {
	Results []*model.ProbeResult `json:"results"`
}

// HeartbeatPayload carries agent status.
type HeartbeatPayload struct {
	AgentID         string    `json:"agent_id"`
	Name            string    `json:"name"`
	UptimeSec       int64     `json:"uptime_sec"`
	TaskCount       int       `json:"task_count"`
	CPUPercent      float64   `json:"cpu_pct"`
	MemMB           float64   `json:"mem_mb"`
	BufferedResults int       `json:"buffered_results"`
	Timestamp       time.Time `json:"timestamp"`
}

package model

import (
	"encoding/json"
	"time"
)

type AgentStatus string

const (
	AgentStatusHealthy      AgentStatus = "healthy"
	AgentStatusUnhealthy    AgentStatus = "unhealthy"
	AgentStatusDisconnected AgentStatus = "disconnected"
)

type Agent struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Labels        json.RawMessage `json:"labels"`
	Address       string          `json:"address"`
	Plugins       []string        `json:"plugins"`
	Status        AgentStatus     `json:"status"`
	LastHeartbeat time.Time       `json:"last_heartbeat"`
	RegisteredAt  time.Time       `json:"registered_at"`
}

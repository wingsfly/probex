package model

import (
	"encoding/json"
	"time"
)

type Task struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Target        string          `json:"target"`
	ProbeType     string          `json:"probe_type"`
	Interval      time.Duration   `json:"interval"`
	Timeout       time.Duration   `json:"timeout"`
	Config        json.RawMessage `json:"config"`
	AgentSelector json.RawMessage `json:"agent_selector"`
	Enabled       bool            `json:"enabled"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type TaskCreate struct {
	Name          string          `json:"name"`
	Target        string          `json:"target"`
	ProbeType     string          `json:"probe_type"`
	Interval      string          `json:"interval"`
	Timeout       string          `json:"timeout"`
	Config        json.RawMessage `json:"config,omitempty"`
	AgentSelector json.RawMessage `json:"agent_selector,omitempty"`
	Enabled       *bool           `json:"enabled,omitempty"`
}

type TaskUpdate struct {
	Name          *string          `json:"name,omitempty"`
	Target        *string          `json:"target,omitempty"`
	ProbeType     *string          `json:"probe_type,omitempty"`
	Interval      *string          `json:"interval,omitempty"`
	Timeout       *string          `json:"timeout,omitempty"`
	Config        *json.RawMessage `json:"config,omitempty"`
	AgentSelector *json.RawMessage `json:"agent_selector,omitempty"`
	Enabled       *bool            `json:"enabled,omitempty"`
}

type TaskFilter struct {
	ProbeType string
	Enabled   *bool
	AgentID   string
	Limit     int
	Offset    int
}

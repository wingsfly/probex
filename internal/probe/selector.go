package probe

import (
	"encoding/json"

	"github.com/hjma/probex/internal/model"
)

type agentSelector struct {
	AgentID string            `json:"agent_id,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// MatchAgent checks if a task's agent_selector matches the given agent.
// Empty selector matches all agents.
// Selector {"agent_id": "xxx"} matches only that agent.
// Selector {"labels": {"region": "us-east"}} matches agents with matching labels.
func MatchAgent(task *model.Task, agent *model.Agent) bool {
	if len(task.AgentSelector) == 0 || string(task.AgentSelector) == "{}" {
		return true
	}

	var sel agentSelector
	if err := json.Unmarshal(task.AgentSelector, &sel); err != nil {
		return true // invalid selector matches all
	}

	if sel.AgentID != "" {
		return sel.AgentID == agent.ID || sel.AgentID == agent.Name
	}

	if len(sel.Labels) > 0 {
		var agentLabels map[string]string
		if err := json.Unmarshal(agent.Labels, &agentLabels); err != nil {
			return false
		}
		for k, v := range sel.Labels {
			if agentLabels[k] != v {
				return false
			}
		}
		return true
	}

	return true
}

// FilterTasksForAgent returns tasks that match the given agent.
func FilterTasksForAgent(tasks []*model.Task, agent *model.Agent) []*model.Task {
	var matched []*model.Task
	for _, t := range tasks {
		if MatchAgent(t, agent) {
			matched = append(matched, t)
		}
	}
	return matched
}

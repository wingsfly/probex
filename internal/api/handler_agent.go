package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/probe"
	"github.com/hjma/probex/internal/store"
)

type AgentHandler struct {
	store    store.Store
	alertEval AlertEvaluator
}

// AlertEvaluator is a minimal interface to avoid circular imports.
type AlertEvaluator interface {
	Evaluate(result *model.ProbeResult)
}

func NewAgentHandler(s store.Store, alertEval AlertEvaluator) *AgentHandler {
	return &AgentHandler{store: s, alertEval: alertEval}
}

func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	agents, err := h.store.ListAgents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, agents)
}

func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agent, err := h.store.GetAgent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeData(w, agent)
}

func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteAgent(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, "deleted")
}

// Register handles remote agent self-registration.
// POST /agents/register
type agentRegisterRequest struct {
	Name    string            `json:"name"`
	Address string            `json:"address"`
	Labels  map[string]string `json:"labels"`
	Plugins []string          `json:"plugins"`
}

func (h *AgentHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req agentRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	labelsJSON, _ := json.Marshal(req.Labels)
	if req.Labels == nil {
		labelsJSON = json.RawMessage("{}")
	}

	now := time.Now()
	agent := &model.Agent{
		ID:            generateID(),
		Name:          req.Name,
		Labels:        labelsJSON,
		Address:       req.Address,
		Plugins:       req.Plugins,
		Status:        model.AgentStatusHealthy,
		LastHeartbeat: now,
		RegisteredAt:  now,
	}

	if err := h.store.UpsertAgent(r.Context(), agent); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, Response{Data: agent})
}

// Heartbeat updates agent's last heartbeat and returns assigned tasks.
// POST /agents/{id}/heartbeat
func (h *AgentHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agent, err := h.store.GetAgent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	h.store.UpdateAgentStatus(r.Context(), id, model.AgentStatusHealthy)

	// Return assigned tasks
	enabled := true
	tasks, _, err := h.store.ListTasks(r.Context(), model.TaskFilter{Enabled: &enabled, Limit: 10000})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	matched := probe.FilterTasksForAgent(tasks, agent)
	writeData(w, matched)
}

// GetTasks returns enabled tasks assigned to this agent.
// GET /agents/{id}/tasks
func (h *AgentHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	agent, err := h.store.GetAgent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	enabled := true
	tasks, _, err := h.store.ListTasks(r.Context(), model.TaskFilter{Enabled: &enabled, Limit: 10000})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	matched := probe.FilterTasksForAgent(tasks, agent)
	if matched == nil {
		matched = []*model.Task{}
	}
	writeData(w, matched)
}

// PushResults accepts probe results from a remote agent.
// POST /agents/{id}/results
func (h *AgentHandler) PushResults(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Verify agent exists
	_, err := h.store.GetAgent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	var results []*model.ProbeResult
	if err := json.NewDecoder(r.Body).Decode(&results); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	// Set agent_id on all results
	for _, res := range results {
		res.AgentID = id
	}

	if err := h.store.InsertResults(r.Context(), results); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Evaluate alerts for each result
	if h.alertEval != nil {
		for _, res := range results {
			h.alertEval.Evaluate(res)
		}
	}

	writeData(w, map[string]int{"accepted": len(results)})
}

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/store"
)

type AlertHandler struct {
	store store.Store
}

func NewAlertHandler(s store.Store) *AlertHandler {
	return &AlertHandler{store: s}
}

func (h *AlertHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var req model.AlertRuleCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" || req.TaskID == "" || req.Metric == "" || req.Operator == "" {
		writeError(w, http.StatusBadRequest, "name, task_id, metric, operator are required")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.ConsecutiveCount <= 0 {
		req.ConsecutiveCount = 1
	}
	if req.Severity == "" {
		req.Severity = model.AlertSeverityWarning
	}

	now := time.Now()
	rule := &model.AlertRule{
		ID:               generateID(),
		Name:             req.Name,
		TaskID:           req.TaskID,
		Metric:           req.Metric,
		Operator:         req.Operator,
		Threshold:        req.Threshold,
		ConsecutiveCount: req.ConsecutiveCount,
		Enabled:          enabled,
		Severity:         req.Severity,
		WebhookURL:       req.WebhookURL,
		SlackWebhookURL:  req.SlackWebhookURL,
		State:            model.AlertStateOK,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := h.store.CreateAlertRule(r.Context(), rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, Response{Data: rule})
}

func (h *AlertHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.store.ListAlertRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rules == nil {
		rules = []*model.AlertRule{}
	}
	writeData(w, rules)
}

func (h *AlertHandler) GetRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.store.GetAlertRule(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	writeData(w, rule)
}

func (h *AlertHandler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.store.GetAlertRule(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}

	var req model.AlertRuleUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.Name != nil {
		rule.Name = *req.Name
	}
	if req.TaskID != nil {
		rule.TaskID = *req.TaskID
	}
	if req.Metric != nil {
		rule.Metric = *req.Metric
	}
	if req.Operator != nil {
		rule.Operator = *req.Operator
	}
	if req.Threshold != nil {
		rule.Threshold = *req.Threshold
	}
	if req.ConsecutiveCount != nil {
		rule.ConsecutiveCount = *req.ConsecutiveCount
	}
	if req.Severity != nil {
		rule.Severity = *req.Severity
	}
	if req.WebhookURL != nil {
		rule.WebhookURL = *req.WebhookURL
	}
	if req.SlackWebhookURL != nil {
		rule.SlackWebhookURL = *req.SlackWebhookURL
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	rule.UpdatedAt = time.Now()

	if err := h.store.UpdateAlertRule(r.Context(), rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, rule)
}

func (h *AlertHandler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteAlertRule(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, Response{Data: "deleted"})
}

func (h *AlertHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ruleID := q.Get("rule_id")
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	events, err := h.store.ListAlertEvents(r.Context(), ruleID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []*model.AlertEvent{}
	}
	writeData(w, events)
}

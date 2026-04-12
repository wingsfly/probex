package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/store"
)

type TaskHandler struct {
	store    store.Store
	notifier TaskNotifier
}

func NewTaskHandler(s store.Store, n TaskNotifier) *TaskHandler {
	return &TaskHandler{store: s, notifier: n}
}

func (h *TaskHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.TaskCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Target = strings.TrimSpace(req.Target)
	req.ProbeType = strings.TrimSpace(req.ProbeType)
	if req.Name == "" || req.Target == "" || req.ProbeType == "" || req.Interval == "" {
		writeError(w, http.StatusBadRequest, "name, target, probe_type, interval are required")
		return
	}

	interval, err := time.ParseDuration(req.Interval)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid interval: "+err.Error())
		return
	}
	timeout := 10 * time.Second
	if req.Timeout != "" {
		timeout, err = time.ParseDuration(req.Timeout)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid timeout: "+err.Error())
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	now := time.Now()
	task := &model.Task{
		ID:            generateID(),
		Name:          req.Name,
		Target:        req.Target,
		ProbeType:     req.ProbeType,
		Interval:      interval,
		Timeout:       timeout,
		Config:        req.Config,
		AgentSelector: req.AgentSelector,
		Enabled:       enabled,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if len(task.Config) == 0 {
		task.Config = json.RawMessage("{}")
	}
	if len(task.AgentSelector) == 0 {
		task.AgentSelector = json.RawMessage("{}")
	}

	if err := h.store.CreateTask(r.Context(), task); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.notifier != nil {
		h.notifier.OnTaskCreated(task)
	}

	writeJSON(w, http.StatusCreated, Response{Data: task})
}

func (h *TaskHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	task, err := h.store.GetTask(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeData(w, task)
}

func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 20
	}

	filter := model.TaskFilter{
		ProbeType: q.Get("probe_type"),
		Limit:     limit,
		Offset:    offset,
	}
	if e := q.Get("enabled"); e != "" {
		v := e == "true" || e == "1"
		filter.Enabled = &v
	}

	tasks, total, err := h.store.ListTasks(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeList(w, tasks, total, limit, offset)
}

func (h *TaskHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	task, err := h.store.GetTask(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	var req model.TaskUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.Name != nil {
		task.Name = *req.Name
	}
	if req.Target != nil {
		task.Target = *req.Target
	}
	if req.ProbeType != nil {
		task.ProbeType = *req.ProbeType
	}
	if req.Interval != nil {
		d, err := time.ParseDuration(*req.Interval)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid interval")
			return
		}
		task.Interval = d
	}
	if req.Timeout != nil {
		d, err := time.ParseDuration(*req.Timeout)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid timeout")
			return
		}
		task.Timeout = d
	}
	if req.Config != nil {
		task.Config = *req.Config
	}
	if req.AgentSelector != nil {
		task.AgentSelector = *req.AgentSelector
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	task.UpdatedAt = time.Now()

	if err := h.store.UpdateTask(r.Context(), task); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.notifier != nil {
		h.notifier.OnTaskUpdated(task)
	}

	writeData(w, task)
}

func (h *TaskHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteTask(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.notifier != nil {
		h.notifier.OnTaskDeleted(id)
	}
	writeJSON(w, http.StatusOK, Response{Data: "deleted"})
}

func (h *TaskHandler) Pause(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	task, err := h.store.GetTask(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	task.Enabled = false
	task.UpdatedAt = time.Now()
	h.store.UpdateTask(r.Context(), task)
	if h.notifier != nil {
		h.notifier.OnTaskPaused(id)
	}
	writeData(w, task)
}

func (h *TaskHandler) Resume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	task, err := h.store.GetTask(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	task.Enabled = true
	task.UpdatedAt = time.Now()
	h.store.UpdateTask(r.Context(), task)
	if h.notifier != nil {
		h.notifier.OnTaskResumed(task)
	}
	writeData(w, task)
}

func (h *TaskHandler) RunOnce(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	task, err := h.store.GetTask(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if h.notifier != nil {
		h.notifier.RunOnce(task)
	}
	writeData(w, "triggered")
}

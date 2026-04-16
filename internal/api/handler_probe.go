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

type ProbeHandler struct {
	registry  *probe.Registry
	store     store.Store
	alertEval AlertEvaluator
}

func NewProbeHandler(reg *probe.Registry, s store.Store, alertEval AlertEvaluator) *ProbeHandler {
	return &ProbeHandler{registry: reg, store: s, alertEval: alertEval}
}

// List returns metadata for all registered probes (builtin + script + external).
func (h *ProbeHandler) List(w http.ResponseWriter, r *http.Request) {
	writeData(w, h.registry.ListMetadata())
}

// Get returns metadata for a single probe.
func (h *ProbeHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	meta, ok := h.registry.GetMetadata(name)
	if !ok {
		writeError(w, http.StatusNotFound, "probe not found: "+name)
		return
	}
	writeData(w, meta)
}

// Register handles external probe registration.
func (h *ProbeHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req model.ProbeRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Check name conflict with builtin/script probes
	if meta, ok := h.registry.GetMetadata(req.Name); ok && meta.Kind != probe.ProbeKindExternal {
		writeError(w, http.StatusConflict, "probe name conflicts with "+string(meta.Kind)+" probe: "+req.Name)
		return
	}

	if len(req.ParameterSchema) == 0 {
		req.ParameterSchema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	if len(req.OutputSchema) == 0 {
		req.OutputSchema = json.RawMessage(`{}`)
	}

	now := time.Now()
	reg := &model.ProbeRegistration{
		Name:            req.Name,
		Kind:            "external",
		Description:     req.Description,
		ParameterSchema: req.ParameterSchema,
		OutputSchema:    req.OutputSchema,
		RegisteredAt:    now,
	}

	if err := h.store.UpsertProbe(r.Context(), reg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Parse output_schema for registry
	var outSchema *probe.OutputSchema
	json.Unmarshal(req.OutputSchema, &outSchema)

	h.registry.RegisterExternal(probe.ProbeMetadata{
		Name:            req.Name,
		Kind:            probe.ProbeKindExternal,
		Description:     req.Description,
		ParameterSchema: req.ParameterSchema,
		OutputSchema:    outSchema,
	})

	writeJSON(w, http.StatusCreated, Response{Data: reg})
}

// Deregister removes an external probe.
func (h *ProbeHandler) Deregister(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	meta, ok := h.registry.GetMetadata(name)
	if !ok {
		writeError(w, http.StatusNotFound, "probe not found")
		return
	}
	if meta.Kind != probe.ProbeKindExternal {
		writeError(w, http.StatusBadRequest, "can only deregister external probes")
		return
	}

	h.registry.UnregisterExternal(name)
	h.store.DeleteProbe(r.Context(), name)
	writeJSON(w, http.StatusOK, Response{Data: "deregistered"})
}

// PushResults accepts probe results from an external program.
func (h *ProbeHandler) PushResults(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	if !h.registry.IsExternal(name) {
		writeError(w, http.StatusBadRequest, "probe is not external or not registered: "+name)
		return
	}

	var req model.ProbePushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	now := time.Now()
	taskID := req.TaskID
	if taskID == "" {
		taskID = "ext_" + name
	}
	agentID := req.AgentID
	if agentID == "" {
		agentID = "external"
	}
	nodeID := req.NodeID // optional — clients should generate via nodeid package

	var results []*model.ProbeResult
	for _, res := range req.Results {
		ts := now
		if res.Timestamp != nil {
			if t, err := time.Parse(time.RFC3339, *res.Timestamp); err == nil {
				ts = t
			}
		}
		pr := &model.ProbeResult{
			ID:             generateID(),
			TaskID:         taskID,
			AgentID:        agentID,
			NodeID:         nodeID,
			Timestamp:      ts,
			Success:        res.Success,
			LatencyMs:      res.LatencyMs,
			JitterMs:       res.JitterMs,
			PacketLossPct:  res.PacketLossPct,
			DNSResolveMs:   res.DNSResolveMs,
			TLSHandshakeMs: res.TLSHandshakeMs,
			StatusCode:     res.StatusCode,
			DownloadBps:    res.DownloadBps,
			UploadBps:      res.UploadBps,
			Error:          res.Error,
			Extra:          res.Extra,
		}
		results = append(results, pr)
	}

	if len(results) > 0 {
		if err := h.store.InsertResults(r.Context(), results); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Evaluate alerts
		if h.alertEval != nil {
			for _, pr := range results {
				h.alertEval.Evaluate(pr)
			}
		}
	}

	h.store.UpdateProbeLastPush(r.Context(), name, now)
	writeData(w, map[string]int{"accepted": len(results)})
}

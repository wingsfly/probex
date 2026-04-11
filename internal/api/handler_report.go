package api

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/report"
	"github.com/hjma/probex/internal/store"
)

type ReportHandler struct {
	store     store.Store
	generator *report.Generator
}

func NewReportHandler(s store.Store, gen *report.Generator) *ReportHandler {
	return &ReportHandler{store: s, generator: gen}
}

func (h *ReportHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.ReportCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Name == "" || len(req.TaskIDs) == 0 || req.TimeRangeStart == "" || req.TimeRangeEnd == "" {
		writeError(w, http.StatusBadRequest, "name, task_ids, time_range_start, time_range_end are required")
		return
	}
	if req.Format == "" {
		req.Format = model.ReportFormatHTML
	}

	start, err := time.Parse(time.RFC3339, req.TimeRangeStart)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid time_range_start: "+err.Error())
		return
	}
	end, err := time.Parse(time.RFC3339, req.TimeRangeEnd)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid time_range_end: "+err.Error())
		return
	}

	rpt := &model.Report{
		ID:             generateID(),
		Name:           req.Name,
		TaskIDs:        req.TaskIDs,
		AgentIDs:       req.AgentIDs,
		TimeRangeStart: start,
		TimeRangeEnd:   end,
		Format:         req.Format,
		Status:         model.ReportStatusPending,
		CreatedAt:      time.Now(),
	}
	if rpt.AgentIDs == nil {
		rpt.AgentIDs = []string{}
	}

	if err := h.store.CreateReport(r.Context(), rpt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go h.generator.Generate(rpt)

	writeJSON(w, http.StatusCreated, Response{Data: rpt})
}

func (h *ReportHandler) List(w http.ResponseWriter, r *http.Request) {
	reports, err := h.store.ListReports(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if reports == nil {
		reports = []*model.Report{}
	}
	writeData(w, reports)
}

func (h *ReportHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rpt, err := h.store.GetReport(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}
	writeData(w, rpt)
}

func (h *ReportHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rpt, err := h.store.GetReport(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}
	if rpt.FilePath != "" {
		os.Remove(rpt.FilePath)
	}
	if err := h.store.DeleteReport(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, Response{Data: "deleted"})
}

func (h *ReportHandler) Download(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rpt, err := h.store.GetReport(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "report not found")
		return
	}
	if rpt.Status != model.ReportStatusCompleted || rpt.FilePath == "" {
		writeError(w, http.StatusBadRequest, "report not ready")
		return
	}

	contentType := "application/octet-stream"
	switch rpt.Format {
	case model.ReportFormatHTML:
		contentType = "text/html"
	case model.ReportFormatJSON:
		contentType = "application/json"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+rpt.Name+"."+string(rpt.Format))
	http.ServeFile(w, r, rpt.FilePath)
}

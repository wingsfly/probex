package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/store"
)

type ResultHandler struct {
	store store.Store
}

func NewResultHandler(s store.Store) *ResultHandler {
	return &ResultHandler{store: s}
}

func (h *ResultHandler) Query(w http.ResponseWriter, r *http.Request) {
	filter := h.parseFilter(r)
	results, total, err := h.store.QueryResults(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeList(w, results, total, filter.Limit, filter.Offset)
}

func (h *ResultHandler) Summary(w http.ResponseWriter, r *http.Request) {
	filter := h.parseFilter(r)
	summary, err := h.store.GetResultSummary(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, summary)
}

func (h *ResultHandler) Latest(w http.ResponseWriter, r *http.Request) {
	results, err := h.store.GetLatestResults(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, results)
}

func (h *ResultHandler) Clear(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}
	deleted, err := h.store.DeleteResultsBefore(r.Context(), taskID, time.Now().UnixMilli()+1)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeData(w, map[string]int64{"deleted": deleted})
}

func (h *ResultHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	filter := h.parseFilter(r)
	filter.Limit = 0 // no limit for export
	results, _, err := h.store.QueryResults(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=results.csv")

	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "task_id", "agent_id", "timestamp", "success", "latency_ms", "jitter_ms", "packet_loss_pct", "dns_resolve_ms", "tls_handshake_ms", "status_code", "download_bps", "upload_bps", "error"})
	for _, res := range results {
		cw.Write([]string{
			res.ID, res.TaskID, res.AgentID,
			res.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%v", res.Success),
			fmtOptFloat(res.LatencyMs),
			fmtOptFloat(res.JitterMs),
			fmtOptFloat(res.PacketLossPct),
			fmtOptFloat(res.DNSResolveMs),
			fmtOptFloat(res.TLSHandshakeMs),
			fmtOptInt(res.StatusCode),
			fmtOptFloat(res.DownloadBps),
			fmtOptFloat(res.UploadBps),
			res.Error,
		})
	}
	cw.Flush()
}

func (h *ResultHandler) ExportJSON(w http.ResponseWriter, r *http.Request) {
	filter := h.parseFilter(r)
	filter.Limit = 0
	results, _, err := h.store.QueryResults(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=results.json")
	json.NewEncoder(w).Encode(results)
}

func (h *ResultHandler) parseFilter(r *http.Request) model.ResultFilter {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 50
	}

	f := model.ResultFilter{
		TaskID:  q.Get("task_id"),
		AgentID: q.Get("agent_id"),
		Limit:   limit,
		Offset:  offset,
	}
	if from := q.Get("from"); from != "" {
		t, _ := time.Parse(time.RFC3339, from)
		f.From = t
	}
	if to := q.Get("to"); to != "" {
		t, _ := time.Parse(time.RFC3339, to)
		f.To = t
	}
	return f
}

func fmtOptFloat(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *v)
}

func fmtOptInt(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

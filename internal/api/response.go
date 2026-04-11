package api

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Data  any    `json:"data,omitempty"`
	Meta  *Meta  `json:"meta,omitempty"`
	Error string `json:"error,omitempty"`
}

type Meta struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func writeJSON(w http.ResponseWriter, status int, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, Response{Error: msg})
}

func writeData(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, Response{Data: data})
}

func writeList(w http.ResponseWriter, data any, total, limit, offset int) {
	writeJSON(w, http.StatusOK, Response{
		Data: data,
		Meta: &Meta{Total: total, Limit: limit, Offset: offset},
	})
}

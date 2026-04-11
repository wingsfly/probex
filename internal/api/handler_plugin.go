package api

import (
	"net/http"

	"github.com/hjma/probex/internal/probe"
)

type PluginHandler struct {
	registry *probe.Registry
}

func NewPluginHandler(reg *probe.Registry) *PluginHandler {
	return &PluginHandler{registry: reg}
}

func (h *PluginHandler) List(w http.ResponseWriter, r *http.Request) {
	names := h.registry.List()
	writeData(w, names)
}

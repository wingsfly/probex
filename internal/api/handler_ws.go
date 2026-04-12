package api

import "github.com/hjma/probex/internal/hub"

// SetWSManager enables WebSocket agent connections on the server.
// Call this after NewServer() for hub mode.
func (s *Server) SetWSManager(ws *hub.WSManager) {
	s.router.Get("/api/v1/ws/agent", ws.HandleUpgrade)
}

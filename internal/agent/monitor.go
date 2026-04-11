package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/store"
)

type Monitor struct {
	store  store.Store
	logger *slog.Logger
}

func NewMonitor(s store.Store, logger *slog.Logger) *Monitor {
	return &Monitor{store: s, logger: logger}
}

// Run starts the agent health monitor. It checks agent heartbeats periodically.
// - >90s since heartbeat → unhealthy
// - >300s since heartbeat → disconnected
func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *Monitor) check(ctx context.Context) {
	agents, err := m.store.ListAgents(ctx)
	if err != nil {
		m.logger.Error("agent monitor: list agents", "error", err)
		return
	}

	now := time.Now()
	for _, a := range agents {
		if a.ID == "local" {
			continue // skip local agent
		}

		elapsed := now.Sub(a.LastHeartbeat)
		var newStatus model.AgentStatus

		switch {
		case elapsed > 5*time.Minute:
			newStatus = model.AgentStatusDisconnected
		case elapsed > 90*time.Second:
			newStatus = model.AgentStatusUnhealthy
		default:
			newStatus = model.AgentStatusHealthy
		}

		if newStatus != a.Status {
			m.store.UpdateAgentStatus(ctx, a.ID, newStatus)
			m.logger.Info("agent status changed", "agent", a.Name, "from", a.Status, "to", newStatus)
		}
	}
}

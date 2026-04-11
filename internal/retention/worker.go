package retention

import (
	"context"
	"log/slog"
	"time"

	"github.com/hjma/probex/internal/store"
)

type Worker struct {
	store     store.Store
	retention time.Duration
	logger    *slog.Logger
}

func NewWorker(s store.Store, retention time.Duration, logger *slog.Logger) *Worker {
	if retention <= 0 {
		retention = 30 * 24 * time.Hour // default 30 days
	}
	return &Worker{store: s, retention: retention, logger: logger}
}

// Run starts the retention cleanup loop. It runs every hour.
func (w *Worker) Run(ctx context.Context) {
	// Run once at startup
	w.cleanup(ctx)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.cleanup(ctx)
		}
	}
}

func (w *Worker) cleanup(ctx context.Context) {
	cutoff := time.Now().Add(-w.retention).UnixMilli()
	deleted, err := w.store.DeleteResultsBefore(ctx, "", cutoff)
	if err != nil {
		w.logger.Error("retention cleanup", "error", err)
		return
	}
	if deleted > 0 {
		w.logger.Info("retention cleanup", "deleted", deleted, "retention", w.retention)
	}
}

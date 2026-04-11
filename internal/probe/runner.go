package probe

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hjma/probex/internal/model"
)

type ResultHandler func(result *model.ProbeResult)

type Runner struct {
	registry *Registry
	agentID  string
	handler  ResultHandler
	maxConc  int

	mu      sync.Mutex
	tasks   map[string]*runningTask
	sem     chan struct{}
	logger  *slog.Logger
}

type runningTask struct {
	task   *model.Task
	cancel context.CancelFunc
}

func NewRunner(registry *Registry, agentID string, maxConcurrent int, handler ResultHandler, logger *slog.Logger) *Runner {
	if maxConcurrent <= 0 {
		maxConcurrent = 50
	}
	return &Runner{
		registry: registry,
		agentID:  agentID,
		handler:  handler,
		maxConc:  maxConcurrent,
		tasks:    make(map[string]*runningTask),
		sem:      make(chan struct{}, maxConcurrent),
		logger:   logger,
	}
}

func (r *Runner) AddTask(task *model.Task) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if rt, ok := r.tasks[task.ID]; ok {
		rt.cancel()
		delete(r.tasks, task.ID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rt := &runningTask{task: task, cancel: cancel}
	r.tasks[task.ID] = rt

	go r.runLoop(ctx, task)
	r.logger.Info("task added", "task_id", task.ID, "target", task.Target, "probe", task.ProbeType, "interval", task.Interval)
}

func (r *Runner) RemoveTask(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if rt, ok := r.tasks[taskID]; ok {
		rt.cancel()
		delete(r.tasks, taskID)
		r.logger.Info("task removed", "task_id", taskID)
	}
}

func (r *Runner) UpdateTask(task *model.Task) {
	r.AddTask(task) // cancel old, start new
}

func (r *Runner) PauseTask(taskID string) {
	r.RemoveTask(taskID) // just stop the loop
}

func (r *Runner) TaskIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.tasks))
	for id := range r.tasks {
		ids = append(ids, id)
	}
	return ids
}

func (r *Runner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, rt := range r.tasks {
		rt.cancel()
		delete(r.tasks, id)
	}
}

func (r *Runner) RunOnce(ctx context.Context, task *model.Task) {
	r.executeProbe(ctx, task)
}

func (r *Runner) runLoop(ctx context.Context, task *model.Task) {
	// Run immediately on first iteration
	r.executeProbe(ctx, task)

	ticker := time.NewTicker(task.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.executeProbe(ctx, task)
		}
	}
}

func (r *Runner) executeProbe(ctx context.Context, task *model.Task) {
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-r.sem }()

	prober, err := r.registry.Get(task.ProbeType)
	if err != nil {
		r.logger.Error("probe not found", "task_id", task.ID, "probe_type", task.ProbeType, "error", err)
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, task.Timeout)
	defer cancel()

	result, err := prober.Probe(probeCtx, task.Target, task.Config)
	if err != nil {
		r.logger.Error("probe error", "task_id", task.ID, "error", err)
		result = &Result{Success: false, Error: err.Error()}
	}

	now := time.Now()
	pr := &model.ProbeResult{
		ID:             generateID(),
		TaskID:         task.ID,
		AgentID:        r.agentID,
		Timestamp:      now,
		Success:        result.Success,
		LatencyMs:      &result.LatencyMs,
		JitterMs:       result.JitterMs,
		PacketLossPct:  result.PacketLossPct,
		DNSResolveMs:   result.DNSResolveMs,
		TLSHandshakeMs: result.TLSHandshakeMs,
		StatusCode:     result.StatusCode,
		DownloadBps:    result.DownloadBps,
		UploadBps:      result.UploadBps,
		Error:          result.Error,
		Extra:          result.Extra,
	}

	if r.handler != nil {
		r.handler(pr)
	}
}

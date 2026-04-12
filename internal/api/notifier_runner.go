package api

import (
	"context"

	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/probe"
)

// RunnerNotifier wraps a local probe.Runner as a TaskNotifier.
// Used in standalone mode where probes execute locally.
type RunnerNotifier struct {
	runner *probe.Runner
}

func NewRunnerNotifier(r *probe.Runner) *RunnerNotifier {
	return &RunnerNotifier{runner: r}
}

func (n *RunnerNotifier) OnTaskCreated(task *model.Task) {
	if task.Enabled {
		n.runner.AddTask(task)
	}
}

func (n *RunnerNotifier) OnTaskUpdated(task *model.Task) {
	if task.Enabled {
		n.runner.UpdateTask(task)
	} else {
		n.runner.RemoveTask(task.ID)
	}
}

func (n *RunnerNotifier) OnTaskDeleted(taskID string) {
	n.runner.RemoveTask(taskID)
}

func (n *RunnerNotifier) OnTaskPaused(taskID string) {
	n.runner.RemoveTask(taskID)
}

func (n *RunnerNotifier) OnTaskResumed(task *model.Task) {
	n.runner.AddTask(task)
}

func (n *RunnerNotifier) RunOnce(task *model.Task) {
	go n.runner.RunOnce(context.Background(), task)
}

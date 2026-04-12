package api

import "github.com/hjma/probex/internal/model"

// TaskNotifier is called by task handlers when tasks change.
// In standalone mode: wraps probe.Runner (local execution).
// In hub mode: wraps hub.WSManager (push to connected agents).
type TaskNotifier interface {
	OnTaskCreated(task *model.Task)
	OnTaskUpdated(task *model.Task)
	OnTaskDeleted(taskID string)
	OnTaskPaused(taskID string)
	OnTaskResumed(task *model.Task)
	RunOnce(task *model.Task)
}

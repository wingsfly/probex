package hub

import "github.com/hjma/probex/internal/model"

// HubNotifier implements api.TaskNotifier by pushing task changes
// to connected agents via WebSocket.
type HubNotifier struct {
	ws *WSManager
}

func NewHubNotifier(ws *WSManager) *HubNotifier {
	return &HubNotifier{ws: ws}
}

func (n *HubNotifier) OnTaskCreated(task *model.Task) {
	if task.Enabled {
		n.ws.BroadcastTaskUpdate(task)
	}
}

func (n *HubNotifier) OnTaskUpdated(task *model.Task) {
	if task.Enabled {
		n.ws.BroadcastTaskUpdate(task)
	} else {
		n.ws.BroadcastTaskDelete(task.ID)
	}
}

func (n *HubNotifier) OnTaskDeleted(taskID string) {
	n.ws.BroadcastTaskDelete(taskID)
}

func (n *HubNotifier) OnTaskPaused(taskID string) {
	n.ws.BroadcastTaskDelete(taskID)
}

func (n *HubNotifier) OnTaskResumed(task *model.Task) {
	n.ws.BroadcastTaskUpdate(task)
}

func (n *HubNotifier) RunOnce(task *model.Task) {
	n.ws.BroadcastTaskUpdate(task)
}

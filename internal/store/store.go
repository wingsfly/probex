package store

import (
	"context"
	"time"

	"github.com/hjma/probex/internal/model"
)

type Store interface {
	// Task operations
	CreateTask(ctx context.Context, task *model.Task) error
	GetTask(ctx context.Context, id string) (*model.Task, error)
	ListTasks(ctx context.Context, filter model.TaskFilter) ([]*model.Task, int, error)
	UpdateTask(ctx context.Context, task *model.Task) error
	DeleteTask(ctx context.Context, id string) error

	// Agent operations
	UpsertAgent(ctx context.Context, agent *model.Agent) error
	GetAgent(ctx context.Context, id string) (*model.Agent, error)
	ListAgents(ctx context.Context) ([]*model.Agent, error)
	DeleteAgent(ctx context.Context, id string) error
	UpdateAgentStatus(ctx context.Context, id string, status model.AgentStatus) error

	// Result operations
	InsertResult(ctx context.Context, result *model.ProbeResult) error
	InsertResults(ctx context.Context, results []*model.ProbeResult) error
	QueryResults(ctx context.Context, filter model.ResultFilter) ([]*model.ProbeResult, int, error)
	GetResultSummary(ctx context.Context, filter model.ResultFilter) (*model.ResultSummary, error)
	GetLatestResults(ctx context.Context) ([]*model.ProbeResult, error)
	DeleteResultsBefore(ctx context.Context, taskID string, before int64) (int64, error)

	// Report operations
	CreateReport(ctx context.Context, report *model.Report) error
	GetReport(ctx context.Context, id string) (*model.Report, error)
	ListReports(ctx context.Context) ([]*model.Report, error)
	UpdateReportStatus(ctx context.Context, id string, status model.ReportStatus, filePath string) error
	DeleteReport(ctx context.Context, id string) error

	// Alert operations
	CreateAlertRule(ctx context.Context, rule *model.AlertRule) error
	GetAlertRule(ctx context.Context, id string) (*model.AlertRule, error)
	ListAlertRules(ctx context.Context) ([]*model.AlertRule, error)
	UpdateAlertRule(ctx context.Context, rule *model.AlertRule) error
	DeleteAlertRule(ctx context.Context, id string) error
	ListAlertRulesByTask(ctx context.Context, taskID string) ([]*model.AlertRule, error)
	CreateAlertEvent(ctx context.Context, event *model.AlertEvent) error
	ListAlertEvents(ctx context.Context, ruleID string, limit int) ([]*model.AlertEvent, error)
	UpdateAlertRuleState(ctx context.Context, id string, state model.AlertState, triggeredAt *time.Time) error

	// Probe registration (external probes)
	UpsertProbe(ctx context.Context, probe *model.ProbeRegistration) error
	GetProbe(ctx context.Context, name string) (*model.ProbeRegistration, error)
	ListProbes(ctx context.Context) ([]*model.ProbeRegistration, error)
	DeleteProbe(ctx context.Context, name string) error
	UpdateProbeLastPush(ctx context.Context, name string, t time.Time) error

	// Lifecycle
	Close() error
}

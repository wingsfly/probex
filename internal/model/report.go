package model

import "time"

type ReportFormat string

const (
	ReportFormatHTML ReportFormat = "html"
	ReportFormatPDF  ReportFormat = "pdf"
	ReportFormatJSON ReportFormat = "json"
)

type ReportStatus string

const (
	ReportStatusPending    ReportStatus = "pending"
	ReportStatusGenerating ReportStatus = "generating"
	ReportStatusCompleted  ReportStatus = "completed"
	ReportStatusFailed     ReportStatus = "failed"
)

type Report struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	TaskIDs        []string     `json:"task_ids"`
	AgentIDs       []string     `json:"agent_ids"`
	TimeRangeStart time.Time    `json:"time_range_start"`
	TimeRangeEnd   time.Time    `json:"time_range_end"`
	Format         ReportFormat `json:"format"`
	Status         ReportStatus `json:"status"`
	FilePath       string       `json:"file_path,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	GeneratedAt    *time.Time   `json:"generated_at,omitempty"`
}

type ReportCreate struct {
	Name           string       `json:"name"`
	TaskIDs        []string     `json:"task_ids"`
	AgentIDs       []string     `json:"agent_ids,omitempty"`
	TimeRangeStart string       `json:"time_range_start"`
	TimeRangeEnd   string       `json:"time_range_end"`
	Format         ReportFormat `json:"format"`
}

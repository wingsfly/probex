package model

import "time"

type AlertMetric string

const (
	AlertMetricLatency    AlertMetric = "latency_ms"
	AlertMetricJitter     AlertMetric = "jitter_ms"
	AlertMetricPacketLoss AlertMetric = "packet_loss_pct"
	AlertMetricSuccess    AlertMetric = "success_rate"
)

type AlertOperator string

const (
	AlertOpGT  AlertOperator = "gt"
	AlertOpLT  AlertOperator = "lt"
	AlertOpGTE AlertOperator = "gte"
	AlertOpLTE AlertOperator = "lte"
)

type AlertSeverity string

const (
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

type AlertState string

const (
	AlertStateOK     AlertState = "ok"
	AlertStateFiring AlertState = "firing"
)

type AlertRule struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	TaskID          string        `json:"task_id"`
	Metric          AlertMetric   `json:"metric"`
	Operator        AlertOperator `json:"operator"`
	Threshold       float64       `json:"threshold"`
	ConsecutiveCount int          `json:"consecutive_count"`
	Enabled         bool          `json:"enabled"`
	Severity        AlertSeverity `json:"severity"`
	WebhookURL      string        `json:"webhook_url,omitempty"`
	SlackWebhookURL string        `json:"slack_webhook_url,omitempty"`
	State           AlertState    `json:"state"`
	LastTriggeredAt *time.Time    `json:"last_triggered_at,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

type AlertRuleCreate struct {
	Name             string        `json:"name"`
	TaskID           string        `json:"task_id"`
	Metric           AlertMetric   `json:"metric"`
	Operator         AlertOperator `json:"operator"`
	Threshold        float64       `json:"threshold"`
	ConsecutiveCount int           `json:"consecutive_count"`
	Severity         AlertSeverity `json:"severity"`
	WebhookURL       string        `json:"webhook_url,omitempty"`
	SlackWebhookURL  string        `json:"slack_webhook_url,omitempty"`
	Enabled          *bool         `json:"enabled,omitempty"`
}

type AlertRuleUpdate struct {
	Name             *string        `json:"name,omitempty"`
	TaskID           *string        `json:"task_id,omitempty"`
	Metric           *AlertMetric   `json:"metric,omitempty"`
	Operator         *AlertOperator `json:"operator,omitempty"`
	Threshold        *float64       `json:"threshold,omitempty"`
	ConsecutiveCount *int           `json:"consecutive_count,omitempty"`
	Severity         *AlertSeverity `json:"severity,omitempty"`
	WebhookURL       *string        `json:"webhook_url,omitempty"`
	SlackWebhookURL  *string        `json:"slack_webhook_url,omitempty"`
	Enabled          *bool          `json:"enabled,omitempty"`
}

type AlertEvent struct {
	ID        string        `json:"id"`
	RuleID    string        `json:"rule_id"`
	RuleName  string        `json:"rule_name"`
	TaskID    string        `json:"task_id"`
	Metric    AlertMetric   `json:"metric"`
	Value     float64       `json:"value"`
	Threshold float64       `json:"threshold"`
	Severity  AlertSeverity `json:"severity"`
	FiredAt   time.Time     `json:"fired_at"`
}

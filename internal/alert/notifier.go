package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/hjma/probex/internal/model"
)

type Notifier struct {
	logger *slog.Logger
}

func NewNotifier(logger *slog.Logger) *Notifier {
	return &Notifier{logger: logger}
}

// NotifySlack sends a formatted alert message to a Slack webhook URL.
func (n *Notifier) NotifySlack(webhookURL string, event *model.AlertEvent) {
	color := "#f59e0b" // warning yellow
	if event.Severity == model.AlertSeverityCritical {
		color = "#dc2626" // critical red
	}

	emoji := ":warning:"
	if event.Severity == model.AlertSeverityCritical {
		emoji = ":rotating_light:"
	}

	payload := map[string]any{
		"attachments": []map[string]any{
			{
				"color":  color,
				"blocks": []map[string]any{
					{
						"type": "section",
						"text": map[string]any{
							"type": "mrkdwn",
							"text": fmt.Sprintf("%s *ProbeX Alert: %s*", emoji, event.RuleName),
						},
					},
					{
						"type": "section",
						"fields": []map[string]any{
							{"type": "mrkdwn", "text": fmt.Sprintf("*Severity:*\n%s", event.Severity)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Metric:*\n%s", event.Metric)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Value:*\n%.2f", event.Value)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Threshold:*\n%.2f", event.Threshold)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Task:*\n%s", event.TaskID)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Time:*\n%s", event.FiredAt.Format(time.RFC3339))},
						},
					},
				},
			},
		},
	}

	body, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		n.logger.Error("slack notification failed", "url", webhookURL, "error", err)
		return
	}
	resp.Body.Close()
	n.logger.Info("slack notification sent", "rule", event.RuleName, "status", resp.StatusCode)
}

package alert

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/hjma/probex/internal/model"
	"github.com/hjma/probex/internal/store"
)

type Evaluator struct {
	store    store.Store
	notifier *Notifier
	logger   *slog.Logger

	mu            sync.Mutex
	breachCounts  map[string]int // ruleID -> consecutive breach count
}

func NewEvaluator(s store.Store, logger *slog.Logger) *Evaluator {
	return &Evaluator{
		store:        s,
		notifier:     NewNotifier(logger),
		logger:       logger,
		breachCounts: make(map[string]int),
	}
}

func (e *Evaluator) Evaluate(result *model.ProbeResult) {
	ctx := context.Background()
	rules, err := e.store.ListAlertRulesByTask(ctx, result.TaskID)
	if err != nil {
		e.logger.Error("alert: list rules", "task_id", result.TaskID, "error", err)
		return
	}

	for _, rule := range rules {
		e.evaluateRule(ctx, rule, result)
	}
}

func (e *Evaluator) evaluateRule(ctx context.Context, rule *model.AlertRule, result *model.ProbeResult) {
	value, ok := extractMetric(rule.Metric, result)
	if !ok {
		return
	}

	breached := checkThreshold(value, rule.Operator, rule.Threshold)

	e.mu.Lock()
	if breached {
		e.breachCounts[rule.ID]++
	} else {
		e.breachCounts[rule.ID] = 0
		e.mu.Unlock()
		// If was firing, resolve
		if rule.State == model.AlertStateFiring {
			e.store.UpdateAlertRuleState(ctx, rule.ID, model.AlertStateOK, nil)
		}
		return
	}
	count := e.breachCounts[rule.ID]
	e.mu.Unlock()

	threshold := rule.ConsecutiveCount
	if threshold <= 0 {
		threshold = 1
	}

	if count >= threshold && rule.State != model.AlertStateFiring {
		now := time.Now()
		event := &model.AlertEvent{
			ID:        generateID(),
			RuleID:    rule.ID,
			RuleName:  rule.Name,
			TaskID:    result.TaskID,
			Metric:    rule.Metric,
			Value:     value,
			Threshold: rule.Threshold,
			Severity:  rule.Severity,
			FiredAt:   now,
		}

		if err := e.store.CreateAlertEvent(ctx, event); err != nil {
			e.logger.Error("alert: create event", "rule_id", rule.ID, "error", err)
			return
		}
		e.store.UpdateAlertRuleState(ctx, rule.ID, model.AlertStateFiring, &now)
		e.logger.Warn("alert fired", "rule", rule.Name, "metric", rule.Metric, "value", value, "threshold", rule.Threshold)

		if rule.WebhookURL != "" {
			go e.callWebhook(rule.WebhookURL, event)
		}
		if rule.SlackWebhookURL != "" {
			go e.notifier.NotifySlack(rule.SlackWebhookURL, event)
		}
	}
}

func extractMetric(metric model.AlertMetric, result *model.ProbeResult) (float64, bool) {
	switch metric {
	case model.AlertMetricLatency:
		if result.LatencyMs != nil {
			return *result.LatencyMs, true
		}
		return 0, false
	case model.AlertMetricJitter:
		if result.JitterMs != nil {
			return *result.JitterMs, true
		}
		return 0, false
	case model.AlertMetricPacketLoss:
		if result.PacketLossPct != nil {
			return *result.PacketLossPct, true
		}
		return 0, false
	case model.AlertMetricSuccess:
		if result.Success {
			return 100, true
		}
		return 0, true
	default:
		return 0, false
	}
}

func checkThreshold(value float64, op model.AlertOperator, threshold float64) bool {
	switch op {
	case model.AlertOpGT:
		return value > threshold
	case model.AlertOpGTE:
		return value >= threshold
	case model.AlertOpLT:
		return value < threshold
	case model.AlertOpLTE:
		return value <= threshold
	default:
		return false
	}
}

type webhookPayload struct {
	Alert    string  `json:"alert"`
	RuleID   string  `json:"rule_id"`
	RuleName string  `json:"rule_name"`
	TaskID   string  `json:"task_id"`
	Metric   string  `json:"metric"`
	Value    float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Severity string  `json:"severity"`
	FiredAt  string  `json:"fired_at"`
}

func (e *Evaluator) callWebhook(url string, event *model.AlertEvent) {
	payload := webhookPayload{
		Alert:     "ProbeX Alert",
		RuleID:    event.RuleID,
		RuleName:  event.RuleName,
		TaskID:    event.TaskID,
		Metric:    string(event.Metric),
		Value:     event.Value,
		Threshold: event.Threshold,
		Severity:  string(event.Severity),
		FiredAt:   event.FiredAt.Format(time.RFC3339),
	}
	body, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		e.logger.Error("alert: webhook failed", "url", url, "error", err)
		return
	}
	resp.Body.Close()
	e.logger.Info("alert: webhook sent", "url", url, "status", resp.StatusCode)
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

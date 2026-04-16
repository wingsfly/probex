package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hjma/probex/internal/model"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func New(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}
	// Incremental migrations for existing databases
	migrations := []string{
		"ALTER TABLE probe_results ADD COLUMN node_id TEXT DEFAULT ''",
	}
	for _, m := range migrations {
		db.Exec(m) // ignore "duplicate column" errors
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

// --- Task ---

func (s *SQLiteStore) CreateTask(ctx context.Context, t *model.Task) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, name, target, probe_type, interval_ms, timeout_ms, config, agent_selector, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.Target, t.ProbeType,
		t.Interval.Milliseconds(), t.Timeout.Milliseconds(),
		string(t.Config), string(t.AgentSelector),
		boolToInt(t.Enabled), t.CreatedAt.UnixMilli(), t.UpdatedAt.UnixMilli(),
	)
	return err
}

func (s *SQLiteStore) GetTask(ctx context.Context, id string) (*model.Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, target, probe_type, interval_ms, timeout_ms, config, agent_selector, enabled, created_at, updated_at FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

func (s *SQLiteStore) ListTasks(ctx context.Context, f model.TaskFilter) ([]*model.Task, int, error) {
	where, args := buildTaskWhere(f)
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks"+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	query := "SELECT id, name, target, probe_type, interval_ms, timeout_ms, config, agent_selector, enabled, created_at, updated_at FROM tasks" + where + " ORDER BY created_at DESC"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var tasks []*model.Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, t)
	}
	return tasks, total, rows.Err()
}

func (s *SQLiteStore) UpdateTask(ctx context.Context, t *model.Task) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET name=?, target=?, probe_type=?, interval_ms=?, timeout_ms=?, config=?, agent_selector=?, enabled=?, updated_at=? WHERE id=?`,
		t.Name, t.Target, t.ProbeType,
		t.Interval.Milliseconds(), t.Timeout.Milliseconds(),
		string(t.Config), string(t.AgentSelector),
		boolToInt(t.Enabled), t.UpdatedAt.UnixMilli(), t.ID,
	)
	return err
}

func (s *SQLiteStore) DeleteTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", id)
	return err
}

// --- Agent ---

func (s *SQLiteStore) UpsertAgent(ctx context.Context, a *model.Agent) error {
	pluginsJSON, _ := json.Marshal(a.Plugins)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agents (id, name, labels, address, plugins, status, last_heartbeat, registered_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, labels=excluded.labels, address=excluded.address, plugins=excluded.plugins, status=excluded.status, last_heartbeat=excluded.last_heartbeat`,
		a.ID, a.Name, string(a.Labels), a.Address, string(pluginsJSON),
		string(a.Status), a.LastHeartbeat.UnixMilli(), a.RegisteredAt.UnixMilli(),
	)
	return err
}

func (s *SQLiteStore) GetAgent(ctx context.Context, id string) (*model.Agent, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, labels, address, plugins, status, last_heartbeat, registered_at FROM agents WHERE id = ?`, id)
	return scanAgent(row)
}

func (s *SQLiteStore) ListAgents(ctx context.Context) ([]*model.Agent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, labels, address, plugins, status, last_heartbeat, registered_at FROM agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []*model.Agent
	for rows.Next() {
		a, err := scanAgentRow(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *SQLiteStore) DeleteAgent(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM agents WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) UpdateAgentStatus(ctx context.Context, id string, status model.AgentStatus) error {
	_, err := s.db.ExecContext(ctx, "UPDATE agents SET status = ?, last_heartbeat = ? WHERE id = ?",
		string(status), time.Now().UnixMilli(), id)
	return err
}

// --- Result ---

func (s *SQLiteStore) InsertResult(ctx context.Context, r *model.ProbeResult) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO probe_results (id, task_id, agent_id, node_id, timestamp, success, latency_ms, jitter_ms, packet_loss_pct, dns_resolve_ms, tls_handshake_ms, status_code, download_bps, upload_bps, error, extra)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.TaskID, r.AgentID, r.NodeID, r.Timestamp.UnixMilli(),
		boolToInt(r.Success), r.LatencyMs,
		r.JitterMs, r.PacketLossPct, r.DNSResolveMs, r.TLSHandshakeMs,
		r.StatusCode, r.DownloadBps, r.UploadBps,
		nullStr(r.Error), nullJSON(r.Extra),
	)
	return err
}

func (s *SQLiteStore) InsertResults(ctx context.Context, results []*model.ProbeResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO probe_results (id, task_id, agent_id, node_id, timestamp, success, latency_ms, jitter_ms, packet_loss_pct, dns_resolve_ms, tls_handshake_ms, status_code, download_bps, upload_bps, error, extra)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range results {
		if _, err := stmt.ExecContext(ctx, r.ID, r.TaskID, r.AgentID, r.NodeID, r.Timestamp.UnixMilli(),
			boolToInt(r.Success), r.LatencyMs,
			r.JitterMs, r.PacketLossPct, r.DNSResolveMs, r.TLSHandshakeMs,
			r.StatusCode, r.DownloadBps, r.UploadBps,
			nullStr(r.Error), nullJSON(r.Extra),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) QueryResults(ctx context.Context, f model.ResultFilter) ([]*model.ProbeResult, int, error) {
	where, args := buildResultWhere(f)
	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM probe_results"+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	query := "SELECT id, task_id, agent_id, node_id, timestamp, success, latency_ms, jitter_ms, packet_loss_pct, dns_resolve_ms, tls_handshake_ms, status_code, download_bps, upload_bps, error, extra FROM probe_results" + where + " ORDER BY timestamp DESC"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var results []*model.ProbeResult
	for rows.Next() {
		r, err := scanResultRow(rows)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, total, rows.Err()
}

func (s *SQLiteStore) GetResultSummary(ctx context.Context, f model.ResultFilter) (*model.ResultSummary, error) {
	where, args := buildResultWhere(f)
	row := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(task_id, ''),
			COALESCE(agent_id, ''),
			COUNT(*),
			COALESCE(AVG(CASE WHEN success = 1 THEN 1.0 ELSE 0.0 END) * 100, 0),
			COALESCE(AVG(latency_ms), 0),
			COALESCE(MIN(latency_ms), 0),
			COALESCE(MAX(latency_ms), 0),
			COALESCE(AVG(jitter_ms), 0),
			COALESCE(AVG(packet_loss_pct), 0)
		FROM probe_results`+where, args...)

	var rs model.ResultSummary
	err := row.Scan(&rs.TaskID, &rs.AgentID, &rs.Count, &rs.SuccessRate,
		&rs.AvgLatencyMs, &rs.MinLatencyMs, &rs.MaxLatencyMs,
		&rs.AvgJitterMs, &rs.AvgLossPct)
	if err != nil {
		return nil, err
	}
	return &rs, nil
}

func (s *SQLiteStore) GetLatestResults(ctx context.Context) ([]*model.ProbeResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.task_id, r.agent_id, r.node_id, r.timestamp, r.success, r.latency_ms, r.jitter_ms, r.packet_loss_pct, r.dns_resolve_ms, r.tls_handshake_ms, r.status_code, r.download_bps, r.upload_bps, r.error, r.extra
		FROM probe_results r
		INNER JOIN (SELECT task_id, agent_id, MAX(timestamp) as max_ts FROM probe_results GROUP BY task_id, agent_id) latest
		ON r.task_id = latest.task_id AND r.agent_id = latest.agent_id AND r.timestamp = latest.max_ts
		ORDER BY r.timestamp DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*model.ProbeResult
	for rows.Next() {
		r, err := scanResultRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *SQLiteStore) DeleteResultsBefore(ctx context.Context, taskID string, before int64) (int64, error) {
	var res sql.Result
	var err error
	if taskID != "" {
		res, err = s.db.ExecContext(ctx, "DELETE FROM probe_results WHERE task_id = ? AND timestamp < ?", taskID, before)
	} else {
		res, err = s.db.ExecContext(ctx, "DELETE FROM probe_results WHERE timestamp < ?", before)
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- Report ---

func (s *SQLiteStore) CreateReport(ctx context.Context, r *model.Report) error {
	taskIDs, _ := json.Marshal(r.TaskIDs)
	agentIDs, _ := json.Marshal(r.AgentIDs)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO reports (id, name, task_ids, agent_ids, time_range_start, time_range_end, format, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, string(taskIDs), string(agentIDs),
		r.TimeRangeStart.UnixMilli(), r.TimeRangeEnd.UnixMilli(),
		string(r.Format), string(r.Status), r.CreatedAt.UnixMilli(),
	)
	return err
}

func (s *SQLiteStore) GetReport(ctx context.Context, id string) (*model.Report, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, task_ids, agent_ids, time_range_start, time_range_end, format, status, file_path, created_at, generated_at FROM reports WHERE id = ?`, id)
	return scanReport(row)
}

func (s *SQLiteStore) ListReports(ctx context.Context) ([]*model.Report, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, task_ids, agent_ids, time_range_start, time_range_end, format, status, file_path, created_at, generated_at FROM reports ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reports []*model.Report
	for rows.Next() {
		r, err := scanReportRow(rows)
		if err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func (s *SQLiteStore) UpdateReportStatus(ctx context.Context, id string, status model.ReportStatus, filePath string) error {
	var genAt *int64
	if status == model.ReportStatusCompleted {
		t := time.Now().UnixMilli()
		genAt = &t
	}
	_, err := s.db.ExecContext(ctx,
		"UPDATE reports SET status = ?, file_path = ?, generated_at = ? WHERE id = ?",
		string(status), filePath, genAt, id)
	return err
}

func (s *SQLiteStore) DeleteReport(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM reports WHERE id = ?", id)
	return err
}

// --- Alert Rules ---

func (s *SQLiteStore) CreateAlertRule(ctx context.Context, r *model.AlertRule) error {
	var lastTriggered *int64
	if r.LastTriggeredAt != nil {
		t := r.LastTriggeredAt.UnixMilli()
		lastTriggered = &t
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO alert_rules (id, name, task_id, metric, operator, threshold, consecutive_count, enabled, severity, webhook_url, slack_webhook_url, state, last_triggered_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.TaskID, string(r.Metric), string(r.Operator), r.Threshold,
		r.ConsecutiveCount, boolToInt(r.Enabled), string(r.Severity), r.WebhookURL, r.SlackWebhookURL,
		string(r.State), lastTriggered, r.CreatedAt.UnixMilli(), r.UpdatedAt.UnixMilli(),
	)
	return err
}

func (s *SQLiteStore) GetAlertRule(ctx context.Context, id string) (*model.AlertRule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, task_id, metric, operator, threshold, consecutive_count, enabled, severity, webhook_url, slack_webhook_url, state, last_triggered_at, created_at, updated_at FROM alert_rules WHERE id = ?`, id)
	return scanAlertRule(row)
}

func (s *SQLiteStore) ListAlertRules(ctx context.Context) ([]*model.AlertRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, task_id, metric, operator, threshold, consecutive_count, enabled, severity, webhook_url, slack_webhook_url, state, last_triggered_at, created_at, updated_at FROM alert_rules ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []*model.AlertRule
	for rows.Next() {
		r, err := scanAlertRuleRow(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *SQLiteStore) ListAlertRulesByTask(ctx context.Context, taskID string) ([]*model.AlertRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, task_id, metric, operator, threshold, consecutive_count, enabled, severity, webhook_url, slack_webhook_url, state, last_triggered_at, created_at, updated_at FROM alert_rules WHERE task_id = ? AND enabled = 1`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []*model.AlertRule
	for rows.Next() {
		r, err := scanAlertRuleRow(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *SQLiteStore) UpdateAlertRule(ctx context.Context, r *model.AlertRule) error {
	var lastTriggered *int64
	if r.LastTriggeredAt != nil {
		t := r.LastTriggeredAt.UnixMilli()
		lastTriggered = &t
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE alert_rules SET name=?, task_id=?, metric=?, operator=?, threshold=?, consecutive_count=?, enabled=?, severity=?, webhook_url=?, slack_webhook_url=?, state=?, last_triggered_at=?, updated_at=? WHERE id=?`,
		r.Name, r.TaskID, string(r.Metric), string(r.Operator), r.Threshold,
		r.ConsecutiveCount, boolToInt(r.Enabled), string(r.Severity), r.WebhookURL, r.SlackWebhookURL,
		string(r.State), lastTriggered, r.UpdatedAt.UnixMilli(), r.ID,
	)
	return err
}

func (s *SQLiteStore) DeleteAlertRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM alert_rules WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) UpdateAlertRuleState(ctx context.Context, id string, state model.AlertState, triggeredAt *time.Time) error {
	var ts *int64
	if triggeredAt != nil {
		t := triggeredAt.UnixMilli()
		ts = &t
	}
	_, err := s.db.ExecContext(ctx,
		"UPDATE alert_rules SET state = ?, last_triggered_at = ?, updated_at = ? WHERE id = ?",
		string(state), ts, time.Now().UnixMilli(), id)
	return err
}

// --- Alert Events ---

func (s *SQLiteStore) CreateAlertEvent(ctx context.Context, e *model.AlertEvent) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO alert_events (id, rule_id, rule_name, task_id, metric, value, threshold, severity, fired_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.RuleID, e.RuleName, e.TaskID, string(e.Metric), e.Value, e.Threshold,
		string(e.Severity), e.FiredAt.UnixMilli(),
	)
	return err
}

func (s *SQLiteStore) ListAlertEvents(ctx context.Context, ruleID string, limit int) ([]*model.AlertEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	var query string
	var args []any
	if ruleID != "" {
		query = `SELECT id, rule_id, rule_name, task_id, metric, value, threshold, severity, fired_at FROM alert_events WHERE rule_id = ? ORDER BY fired_at DESC LIMIT ?`
		args = []any{ruleID, limit}
	} else {
		query = `SELECT id, rule_id, rule_name, task_id, metric, value, threshold, severity, fired_at FROM alert_events ORDER BY fired_at DESC LIMIT ?`
		args = []any{limit}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*model.AlertEvent
	for rows.Next() {
		e, err := scanAlertEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func scanAlertRule(row scanner) (*model.AlertRule, error) {
	var r model.AlertRule
	var metric, operator, severity, state string
	var enabled int
	var lastTriggered *int64
	var createdAt, updatedAt int64
	err := row.Scan(&r.ID, &r.Name, &r.TaskID, &metric, &operator, &r.Threshold,
		&r.ConsecutiveCount, &enabled, &severity, &r.WebhookURL, &r.SlackWebhookURL, &state,
		&lastTriggered, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	r.Metric = model.AlertMetric(metric)
	r.Operator = model.AlertOperator(operator)
	r.Severity = model.AlertSeverity(severity)
	r.State = model.AlertState(state)
	r.Enabled = enabled == 1
	r.CreatedAt = time.UnixMilli(createdAt)
	r.UpdatedAt = time.UnixMilli(updatedAt)
	if lastTriggered != nil {
		t := time.UnixMilli(*lastTriggered)
		r.LastTriggeredAt = &t
	}
	return &r, nil
}

func scanAlertRuleRow(rows *sql.Rows) (*model.AlertRule, error) { return scanAlertRule(rows) }

// --- Probes (external registration) ---

func (s *SQLiteStore) UpsertProbe(ctx context.Context, p *model.ProbeRegistration) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO probes (name, kind, description, parameter_schema, output_schema, registered_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET description=excluded.description, parameter_schema=excluded.parameter_schema, output_schema=excluded.output_schema`,
		p.Name, p.Kind, p.Description, string(p.ParameterSchema), string(p.OutputSchema), p.RegisteredAt.UnixMilli(),
	)
	return err
}

func (s *SQLiteStore) GetProbe(ctx context.Context, name string) (*model.ProbeRegistration, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT name, kind, description, parameter_schema, output_schema, registered_at, last_push_at FROM probes WHERE name = ?`, name)
	var p model.ProbeRegistration
	var paramSchema, outputSchema string
	var registeredAt int64
	var lastPushAt *int64
	err := row.Scan(&p.Name, &p.Kind, &p.Description, &paramSchema, &outputSchema, &registeredAt, &lastPushAt)
	if err != nil {
		return nil, err
	}
	p.ParameterSchema = json.RawMessage(paramSchema)
	p.OutputSchema = json.RawMessage(outputSchema)
	p.RegisteredAt = time.UnixMilli(registeredAt)
	if lastPushAt != nil {
		t := time.UnixMilli(*lastPushAt)
		p.LastPushAt = &t
	}
	return &p, nil
}

func (s *SQLiteStore) ListProbes(ctx context.Context) ([]*model.ProbeRegistration, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, kind, description, parameter_schema, output_schema, registered_at, last_push_at FROM probes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var probes []*model.ProbeRegistration
	for rows.Next() {
		var p model.ProbeRegistration
		var paramSchema, outputSchema string
		var registeredAt int64
		var lastPushAt *int64
		if err := rows.Scan(&p.Name, &p.Kind, &p.Description, &paramSchema, &outputSchema, &registeredAt, &lastPushAt); err != nil {
			return nil, err
		}
		p.ParameterSchema = json.RawMessage(paramSchema)
		p.OutputSchema = json.RawMessage(outputSchema)
		p.RegisteredAt = time.UnixMilli(registeredAt)
		if lastPushAt != nil {
			t := time.UnixMilli(*lastPushAt)
			p.LastPushAt = &t
		}
		probes = append(probes, &p)
	}
	return probes, rows.Err()
}

func (s *SQLiteStore) DeleteProbe(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM probes WHERE name = ?", name)
	return err
}

func (s *SQLiteStore) UpdateProbeLastPush(ctx context.Context, name string, t time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE probes SET last_push_at = ? WHERE name = ?", t.UnixMilli(), name)
	return err
}

func scanAlertEventRow(rows *sql.Rows) (*model.AlertEvent, error) {
	var e model.AlertEvent
	var metric, severity string
	var firedAt int64
	err := rows.Scan(&e.ID, &e.RuleID, &e.RuleName, &e.TaskID, &metric, &e.Value, &e.Threshold, &severity, &firedAt)
	if err != nil {
		return nil, err
	}
	e.Metric = model.AlertMetric(metric)
	e.Severity = model.AlertSeverity(severity)
	e.FiredAt = time.UnixMilli(firedAt)
	return &e, nil
}

// --- Helpers ---

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullJSON(raw json.RawMessage) *string {
	if len(raw) == 0 {
		return nil
	}
	s := string(raw)
	return &s
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(row scanner) (*model.Task, error) {
	var t model.Task
	var intervalMs, timeoutMs int64
	var config, agentSel string
	var enabled int
	var createdAt, updatedAt int64
	err := row.Scan(&t.ID, &t.Name, &t.Target, &t.ProbeType,
		&intervalMs, &timeoutMs, &config, &agentSel,
		&enabled, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	t.Interval = time.Duration(intervalMs) * time.Millisecond
	t.Timeout = time.Duration(timeoutMs) * time.Millisecond
	t.Config = json.RawMessage(config)
	t.AgentSelector = json.RawMessage(agentSel)
	t.Enabled = enabled == 1
	t.CreatedAt = time.UnixMilli(createdAt)
	t.UpdatedAt = time.UnixMilli(updatedAt)
	return &t, nil
}

func scanTaskRow(rows *sql.Rows) (*model.Task, error) { return scanTask(rows) }

func scanAgent(row scanner) (*model.Agent, error) {
	var a model.Agent
	var labels, plugins, status string
	var heartbeat, registered int64
	err := row.Scan(&a.ID, &a.Name, &labels, &a.Address, &plugins, &status, &heartbeat, &registered)
	if err != nil {
		return nil, err
	}
	a.Labels = json.RawMessage(labels)
	json.Unmarshal([]byte(plugins), &a.Plugins)
	a.Status = model.AgentStatus(status)
	a.LastHeartbeat = time.UnixMilli(heartbeat)
	a.RegisteredAt = time.UnixMilli(registered)
	return &a, nil
}

func scanAgentRow(rows *sql.Rows) (*model.Agent, error) { return scanAgent(rows) }

func scanResult(row scanner) (*model.ProbeResult, error) {
	var r model.ProbeResult
	var ts int64
	var success int
	var errStr, extra *string
	err := row.Scan(&r.ID, &r.TaskID, &r.AgentID, &r.NodeID, &ts, &success,
		&r.LatencyMs, &r.JitterMs, &r.PacketLossPct,
		&r.DNSResolveMs, &r.TLSHandshakeMs, &r.StatusCode,
		&r.DownloadBps, &r.UploadBps, &errStr, &extra)
	if err != nil {
		return nil, err
	}
	r.Timestamp = time.UnixMilli(ts)
	r.Success = success == 1
	if errStr != nil {
		r.Error = *errStr
	}
	if extra != nil {
		r.Extra = json.RawMessage(*extra)
	}
	return &r, nil
}

func scanResultRow(rows *sql.Rows) (*model.ProbeResult, error) { return scanResult(rows) }

func scanReport(row scanner) (*model.Report, error) {
	var r model.Report
	var taskIDsJSON, agentIDsJSON string
	var start, end, createdAt int64
	var genAt *int64
	var filePath *string
	var format, status string
	err := row.Scan(&r.ID, &r.Name, &taskIDsJSON, &agentIDsJSON,
		&start, &end, &format, &status, &filePath, &createdAt, &genAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(taskIDsJSON), &r.TaskIDs)
	json.Unmarshal([]byte(agentIDsJSON), &r.AgentIDs)
	r.TimeRangeStart = time.UnixMilli(start)
	r.TimeRangeEnd = time.UnixMilli(end)
	r.Format = model.ReportFormat(format)
	r.Status = model.ReportStatus(status)
	if filePath != nil {
		r.FilePath = *filePath
	}
	r.CreatedAt = time.UnixMilli(createdAt)
	if genAt != nil {
		t := time.UnixMilli(*genAt)
		r.GeneratedAt = &t
	}
	return &r, nil
}

func scanReportRow(rows *sql.Rows) (*model.Report, error) { return scanReport(rows) }

func buildTaskWhere(f model.TaskFilter) (string, []any) {
	var conds []string
	var args []any
	if f.ProbeType != "" {
		conds = append(conds, "probe_type = ?")
		args = append(args, f.ProbeType)
	}
	if f.Enabled != nil {
		conds = append(conds, "enabled = ?")
		args = append(args, boolToInt(*f.Enabled))
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func buildResultWhere(f model.ResultFilter) (string, []any) {
	var conds []string
	var args []any
	if f.TaskID != "" {
		conds = append(conds, "task_id = ?")
		args = append(args, f.TaskID)
	}
	if f.AgentID != "" {
		conds = append(conds, "agent_id = ?")
		args = append(args, f.AgentID)
	}
	if !f.From.IsZero() {
		conds = append(conds, "timestamp >= ?")
		args = append(args, f.From.UnixMilli())
	}
	if !f.To.IsZero() {
		conds = append(conds, "timestamp <= ?")
		args = append(args, f.To.UnixMilli())
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

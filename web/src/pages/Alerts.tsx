import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { AlertRule, AlertEvent, Task, AlertRuleCreate } from '../types/api';

type Tab = 'rules' | 'events';

export default function Alerts() {
  const [tab, setTab] = useState<Tab>('rules');

  return (
    <div>
      <h1 style={{ fontSize: '1.5rem', fontWeight: 600, marginBottom: '1rem' }}>Alerts</h1>

      <div style={{ display: 'flex', gap: '0.25rem', marginBottom: '1rem' }}>
        {(['rules', 'events'] as Tab[]).map(t => (
          <button key={t} onClick={() => setTab(t)} style={{
            padding: '0.5rem 1rem', border: '1px solid #d1d5db', borderRadius: 6,
            background: tab === t ? '#3b82f6' : '#fff', color: tab === t ? '#fff' : '#374151',
            cursor: 'pointer', fontSize: '0.875rem', textTransform: 'capitalize',
          }}>
            {t}
          </button>
        ))}
      </div>

      {tab === 'rules' ? <RulesTab /> : <EventsTab />}
    </div>
  );
}

function RulesTab() {
  const queryClient = useQueryClient();
  const [showModal, setShowModal] = useState(false);
  const [editingRule, setEditingRule] = useState<AlertRule | null>(null);

  const { data: rulesData, isLoading } = useQuery({
    queryKey: ['alertRules'],
    queryFn: () => api.getAlertRules(),
    refetchInterval: 5000,
  });

  const { data: tasksData } = useQuery({
    queryKey: ['tasks'],
    queryFn: () => api.getTasks(),
  });

  const rules: AlertRule[] = rulesData?.data ?? [];
  const tasks: Task[] = tasksData?.data ?? [];
  const taskMap = new Map(tasks.map(t => [t.id, t]));

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteAlertRule(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['alertRules'] }),
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api.updateAlertRule(id, { enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['alertRules'] }),
  });

  return (
    <>
      <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: '0.75rem' }}>
        <button onClick={() => { setEditingRule(null); setShowModal(true); }} style={btnPrimary}>New Rule</button>
      </div>

      {isLoading ? <p>Loading...</p> : (
        <div style={{ background: '#fff', borderRadius: 8, border: '1px solid #e5e7eb' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.875rem' }}>
            <thead>
              <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                <th style={thStyle}>Name</th>
                <th style={thStyle}>Task</th>
                <th style={thStyle}>Condition</th>
                <th style={thStyle}>Severity</th>
                <th style={thStyle}>State</th>
                <th style={thStyle}>Enabled</th>
                <th style={thStyle}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {rules.map(r => (
                <tr key={r.id} style={{ borderBottom: '1px solid #f3f4f6', background: r.state === 'firing' ? '#fef2f2' : undefined }}>
                  <td style={tdStyle}>{r.name}</td>
                  <td style={tdStyle}>{taskMap.get(r.task_id)?.name ?? r.task_id.slice(0, 8)}</td>
                  <td style={tdStyle}>
                    <code style={{ fontSize: '0.8rem', background: '#f3f4f6', padding: '1px 4px', borderRadius: 3 }}>
                      {r.metric} {opLabel(r.operator)} {r.threshold}
                    </code>
                    {r.consecutive_count > 1 && <span style={{ color: '#6b7280', fontSize: '0.75rem' }}> x{r.consecutive_count}</span>}
                  </td>
                  <td style={tdStyle}>
                    <span style={{ ...badgeStyle, ...(r.severity === 'critical' ? { background: '#fee2e2', color: '#991b1b' } : { background: '#fef3c7', color: '#92400e' }) }}>
                      {r.severity}
                    </span>
                  </td>
                  <td style={tdStyle}>
                    <span style={{ ...badgeStyle, ...(r.state === 'firing' ? { background: '#fee2e2', color: '#dc2626' } : { background: '#dcfce7', color: '#166534' }) }}>
                      {r.state}
                    </span>
                  </td>
                  <td style={tdStyle}>
                    <input type="checkbox" checked={r.enabled}
                      onChange={() => toggleMutation.mutate({ id: r.id, enabled: !r.enabled })} />
                  </td>
                  <td style={tdStyle}>
                    <div style={{ display: 'flex', gap: '0.5rem' }}>
                      <button onClick={() => { setEditingRule(r); setShowModal(true); }} style={btnSmall}>Edit</button>
                      <button onClick={() => { if (window.confirm('Delete this rule?')) deleteMutation.mutate(r.id); }}
                        style={{ ...btnSmall, color: '#ef4444', borderColor: '#fca5a5' }}>Delete</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {rules.length === 0 && <p style={{ padding: '2rem', textAlign: 'center', color: '#6b7280' }}>No alert rules</p>}
        </div>
      )}

      {showModal && (
        <RuleModal
          tasks={tasks}
          existing={editingRule}
          onClose={() => setShowModal(false)}
        />
      )}
    </>
  );
}

function RuleModal({ tasks, existing, onClose }: { tasks: Task[]; existing: AlertRule | null; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(existing?.name ?? '');
  const [taskId, setTaskId] = useState(existing?.task_id ?? (tasks[0]?.id ?? ''));
  const [metric, setMetric] = useState(existing?.metric ?? 'latency_ms');
  const [operator, setOperator] = useState(existing?.operator ?? 'gt');
  const [threshold, setThreshold] = useState(String(existing?.threshold ?? 100));
  const [consecutive, setConsecutive] = useState(String(existing?.consecutive_count ?? 1));
  const [severity, setSeverity] = useState<'warning' | 'critical'>(existing?.severity ?? 'warning');
  const [webhookUrl, setWebhookUrl] = useState(existing?.webhook_url ?? '');
  const [slackWebhookUrl, setSlackWebhookUrl] = useState(existing?.slack_webhook_url ?? '');

  const createMutation = useMutation({
    mutationFn: (body: AlertRuleCreate) => api.createAlertRule(body),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['alertRules'] }); onClose(); },
  });

  const updateMutation = useMutation({
    mutationFn: (body: any) => api.updateAlertRule(existing!.id, body),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['alertRules'] }); onClose(); },
  });

  const handleSubmit = () => {
    if (!name || !taskId) return;
    const body = {
      name, task_id: taskId, metric, operator,
      threshold: parseFloat(threshold),
      consecutive_count: parseInt(consecutive) || 1,
      severity, webhook_url: webhookUrl || undefined,
      slack_webhook_url: slackWebhookUrl || undefined,
    };
    if (existing) {
      updateMutation.mutate(body);
    } else {
      createMutation.mutate(body);
    }
  };

  return (
    <div style={overlay}>
      <div style={modal}>
        <h2 style={{ fontSize: '1.25rem', fontWeight: 600, marginBottom: '1rem' }}>
          {existing ? 'Edit Alert Rule' : 'New Alert Rule'}
        </h2>

        <label style={labelStyle}>Name</label>
        <input value={name} onChange={e => setName(e.target.value)} style={inputStyle} placeholder="e.g. High Latency Alert" />

        <label style={labelStyle}>Task</label>
        <select value={taskId} onChange={e => setTaskId(e.target.value)} style={inputStyle}>
          {tasks.map(t => <option key={t.id} value={t.id}>{t.name} ({t.target})</option>)}
        </select>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: '0.5rem' }}>
          <div>
            <label style={labelStyle}>Metric</label>
            <select value={metric} onChange={e => setMetric(e.target.value)} style={inputStyle}>
              <option value="latency_ms">Latency (ms)</option>
              <option value="jitter_ms">Jitter (ms)</option>
              <option value="packet_loss_pct">Packet Loss (%)</option>
              <option value="success_rate">Success Rate (%)</option>
            </select>
          </div>
          <div>
            <label style={labelStyle}>Operator</label>
            <select value={operator} onChange={e => setOperator(e.target.value)} style={inputStyle}>
              <option value="gt">&gt; (greater than)</option>
              <option value="gte">&ge; (greater or equal)</option>
              <option value="lt">&lt; (less than)</option>
              <option value="lte">&le; (less or equal)</option>
            </select>
          </div>
          <div>
            <label style={labelStyle}>Threshold</label>
            <input type="number" value={threshold} onChange={e => setThreshold(e.target.value)} style={inputStyle} />
          </div>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.5rem' }}>
          <div>
            <label style={labelStyle}>Consecutive Count</label>
            <input type="number" min="1" value={consecutive} onChange={e => setConsecutive(e.target.value)} style={inputStyle} />
          </div>
          <div>
            <label style={labelStyle}>Severity</label>
            <select value={severity} onChange={e => setSeverity(e.target.value as 'warning' | 'critical')} style={inputStyle}>
              <option value="warning">Warning</option>
              <option value="critical">Critical</option>
            </select>
          </div>
        </div>

        <label style={labelStyle}>Webhook URL (optional)</label>
        <input value={webhookUrl} onChange={e => setWebhookUrl(e.target.value)} style={inputStyle} placeholder="https://hooks.example.com/alert" />

        <label style={labelStyle}>Slack Webhook URL (optional)</label>
        <input value={slackWebhookUrl} onChange={e => setSlackWebhookUrl(e.target.value)} style={inputStyle} placeholder="https://hooks.slack.com/services/..." />

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem', marginTop: '1.5rem' }}>
          <button onClick={onClose} style={btnCancel}>Cancel</button>
          <button onClick={handleSubmit} disabled={!name || !taskId || createMutation.isPending || updateMutation.isPending} style={btnPrimary}>
            {(createMutation.isPending || updateMutation.isPending) ? 'Saving...' : (existing ? 'Update' : 'Create')}
          </button>
        </div>
      </div>
    </div>
  );
}

function EventsTab() {
  const { data: eventsData, isLoading } = useQuery({
    queryKey: ['alertEvents'],
    queryFn: () => api.getAlertEvents('limit=100'),
    refetchInterval: 5000,
  });

  const events: AlertEvent[] = eventsData?.data ?? [];

  return (
    <div style={{ background: '#fff', borderRadius: 8, border: '1px solid #e5e7eb' }}>
      {isLoading ? <p style={{ padding: '1rem' }}>Loading...</p> : (
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.875rem' }}>
          <thead>
            <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
              <th style={thStyle}>Time</th>
              <th style={thStyle}>Rule</th>
              <th style={thStyle}>Metric</th>
              <th style={thStyle}>Value</th>
              <th style={thStyle}>Threshold</th>
              <th style={thStyle}>Severity</th>
            </tr>
          </thead>
          <tbody>
            {events.map(e => (
              <tr key={e.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                <td style={tdStyle}>{new Date(e.fired_at).toLocaleString()}</td>
                <td style={tdStyle}>{e.rule_name}</td>
                <td style={tdStyle}><code style={{ fontSize: '0.8rem' }}>{e.metric}</code></td>
                <td style={{ ...tdStyle, color: '#dc2626', fontWeight: 500 }}>{e.value.toFixed(2)}</td>
                <td style={tdStyle}>{e.threshold}</td>
                <td style={tdStyle}>
                  <span style={{ ...badgeStyle, ...(e.severity === 'critical' ? { background: '#fee2e2', color: '#991b1b' } : { background: '#fef3c7', color: '#92400e' }) }}>
                    {e.severity}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {!isLoading && events.length === 0 && (
        <p style={{ padding: '2rem', textAlign: 'center', color: '#6b7280' }}>No alert events</p>
      )}
    </div>
  );
}

function opLabel(op: string): string {
  switch (op) {
    case 'gt': return '>';
    case 'gte': return '>=';
    case 'lt': return '<';
    case 'lte': return '<=';
    default: return op;
  }
}

const thStyle: React.CSSProperties = { padding: '0.75rem 0.5rem', fontWeight: 500 };
const tdStyle: React.CSSProperties = { padding: '0.5rem' };
const badgeStyle: React.CSSProperties = { display: 'inline-block', padding: '2px 8px', borderRadius: 4, fontSize: '0.75rem', fontWeight: 500 };
const overlay: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100 };
const modal: React.CSSProperties = { background: '#fff', borderRadius: 12, padding: '1.5rem', width: 560, maxHeight: '80vh', overflowY: 'auto' };
const labelStyle: React.CSSProperties = { display: 'block', fontSize: '0.8rem', fontWeight: 500, color: '#374151', marginTop: '1rem', marginBottom: '0.25rem' };
const inputStyle: React.CSSProperties = { width: '100%', padding: '0.5rem', border: '1px solid #d1d5db', borderRadius: 6, fontSize: '0.875rem' };
const btnPrimary: React.CSSProperties = { padding: '0.5rem 1rem', background: '#3b82f6', color: '#fff', border: 'none', borderRadius: 6, cursor: 'pointer', fontSize: '0.875rem' };
const btnCancel: React.CSSProperties = { padding: '0.5rem 1rem', background: '#fff', color: '#374151', border: '1px solid #d1d5db', borderRadius: 6, cursor: 'pointer', fontSize: '0.875rem' };
const btnSmall: React.CSSProperties = { padding: '0.25rem 0.5rem', background: '#fff', border: '1px solid #d1d5db', borderRadius: 4, cursor: 'pointer', fontSize: '0.75rem' };

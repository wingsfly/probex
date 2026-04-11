import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import type { ProbeResult, Task, AlertRule } from '../types/api';
import {
  LineChart, Line, ResponsiveContainer, YAxis,
} from 'recharts';

export default function Dashboard() {
  const { data: latestData, isLoading } = useQuery({
    queryKey: ['latestResults'],
    queryFn: api.getLatestResults,
    refetchInterval: 5000,
  });

  const { data: tasksData } = useQuery({
    queryKey: ['tasks'],
    queryFn: () => api.getTasks(),
  });

  const { data: alertData } = useQuery({
    queryKey: ['alertRules'],
    queryFn: () => api.getAlertRules(),
    refetchInterval: 5000,
  });

  // Fetch recent results for sparklines (last 20 per task)
  const { data: recentData } = useQuery({
    queryKey: ['recentResults'],
    queryFn: () => {
      const from = new Date();
      from.setHours(from.getHours() - 1);
      return api.getResults(`limit=500&from=${from.toISOString()}`);
    },
    refetchInterval: 10000,
  });

  const results: ProbeResult[] = latestData?.data ?? [];
  const tasks: Task[] = tasksData?.data ?? [];
  const alertRules: AlertRule[] = alertData?.data ?? [];
  const recentResults: ProbeResult[] = recentData?.data ?? [];

  const taskMap = useMemo(() => {
    const m = new Map<string, Task>();
    tasks.forEach(t => m.set(t.id, t));
    return m;
  }, [tasks]);

  const firingCount = alertRules.filter(r => r.state === 'firing').length;

  // Sparkline data: last 20 results per task, oldest first
  const sparklines = useMemo(() => {
    const byTask = new Map<string, { latency: number }[]>();
    // recentResults is desc, reverse for chart
    const asc = [...recentResults].reverse();
    for (const r of asc) {
      if (!byTask.has(r.task_id)) byTask.set(r.task_id, []);
      const arr = byTask.get(r.task_id)!;
      if (arr.length < 20) arr.push({ latency: r.latency_ms });
    }
    return byTask;
  }, [recentResults]);

  return (
    <div>
      <h1 style={{ fontSize: '1.5rem', fontWeight: 600, marginBottom: '1rem' }}>
        Dashboard
      </h1>

      {isLoading ? (
        <p>Loading...</p>
      ) : results.length === 0 ? (
        <div style={{ padding: '2rem', textAlign: 'center', color: '#666' }}>
          <p>No probe results yet. Create a task to start monitoring.</p>
        </div>
      ) : (
        <>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '1rem', marginBottom: '1.5rem' }}>
            <StatCard label="Total Tasks" value={tasks.length} />
            <StatCard label="Active Probes" value={results.length} />
            <StatCard label="Healthy" value={results.filter(r => r.success).length} color="#22c55e" />
            <StatCard label="Failed" value={results.filter(r => !r.success).length} color="#ef4444" />
            <StatCard
              label="Avg Latency"
              value={`${(results.reduce((s, r) => s + r.latency_ms, 0) / results.length).toFixed(1)}ms`}
            />
            <StatCard
              label="Firing Alerts"
              value={firingCount}
              color={firingCount > 0 ? '#dc2626' : '#22c55e'}
            />
          </div>

          {/* Firing alerts banner */}
          {firingCount > 0 && (
            <div style={{
              background: '#fef2f2', border: '1px solid #fecaca', borderRadius: 8,
              padding: '0.75rem 1rem', marginBottom: '1rem', display: 'flex', gap: '0.5rem', alignItems: 'center',
            }}>
              <span style={{ fontSize: '1.1rem' }}>!</span>
              <div>
                <strong style={{ color: '#dc2626' }}>{firingCount} alert{firingCount > 1 ? 's' : ''} firing</strong>
                <span style={{ color: '#991b1b', fontSize: '0.85rem', marginLeft: '0.5rem' }}>
                  {alertRules.filter(r => r.state === 'firing').map(r => r.name).join(', ')}
                </span>
              </div>
            </div>
          )}

          <div style={{ background: '#fff', borderRadius: 8, padding: '1rem', border: '1px solid #e5e7eb' }}>
            <h2 style={{ fontSize: '1.1rem', fontWeight: 500, marginBottom: '0.5rem' }}>Latest Results</h2>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.875rem' }}>
              <thead>
                <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                  <th style={thStyle}>Task</th>
                  <th style={thStyle}>Target</th>
                  <th style={thStyle}>Agent</th>
                  <th style={thStyle}>Status</th>
                  <th style={thStyle}>Latency</th>
                  <th style={thStyle}>Trend</th>
                  <th style={thStyle}>Jitter</th>
                  <th style={thStyle}>Loss%</th>
                  <th style={thStyle}>Time</th>
                </tr>
              </thead>
              <tbody>
                {results.map((r) => {
                  const task = taskMap.get(r.task_id);
                  const spark = sparklines.get(r.task_id);
                  return (
                    <tr key={r.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                      <td style={tdStyle}>
                        <span style={{ fontWeight: 500 }}>{task?.name ?? r.task_id.slice(0, 8)}</span>
                        <span style={{
                          marginLeft: 6, fontSize: '0.7rem', padding: '1px 5px',
                          background: '#f3f4f6', borderRadius: 3, color: '#6b7280',
                        }}>
                          {task?.probe_type ?? ''}
                        </span>
                      </td>
                      <td style={{ ...tdStyle, color: '#6b7280', fontSize: '0.8rem' }}>{task?.target ?? '-'}</td>
                      <td style={tdStyle}>{r.agent_id}</td>
                      <td style={tdStyle}>
                        <span style={{
                          display: 'inline-block', width: 8, height: 8, borderRadius: '50%',
                          backgroundColor: r.success ? '#22c55e' : '#ef4444', marginRight: 6,
                        }} />
                        {r.success ? 'OK' : 'FAIL'}
                      </td>
                      <td style={tdStyle}>{r.latency_ms.toFixed(1)}ms</td>
                      <td style={{ ...tdStyle, width: 100 }}>
                        {spark && spark.length > 1 ? (
                          <ResponsiveContainer width={80} height={24}>
                            <LineChart data={spark}>
                              <YAxis hide domain={['dataMin', 'dataMax']} />
                              <Line type="monotone" dataKey="latency" stroke="#3b82f6" dot={false} strokeWidth={1.5} />
                            </LineChart>
                          </ResponsiveContainer>
                        ) : '-'}
                      </td>
                      <td style={tdStyle}>{r.jitter_ms != null ? `${r.jitter_ms.toFixed(1)}ms` : '-'}</td>
                      <td style={tdStyle}>{r.packet_loss_pct != null ? `${r.packet_loss_pct.toFixed(1)}%` : '-'}</td>
                      <td style={tdStyle}>{new Date(r.timestamp).toLocaleTimeString()}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}

function StatCard({ label, value, color }: { label: string; value: string | number; color?: string }) {
  return (
    <div style={{
      background: '#fff', borderRadius: 8, padding: '1rem',
      border: '1px solid #e5e7eb',
    }}>
      <div style={{ fontSize: '0.75rem', color: '#6b7280', textTransform: 'uppercase' }}>{label}</div>
      <div style={{ fontSize: '1.5rem', fontWeight: 700, color: color ?? '#111827', marginTop: 4 }}>
        {value}
      </div>
    </div>
  );
}

const thStyle: React.CSSProperties = { padding: '0.5rem' };
const tdStyle: React.CSSProperties = { padding: '0.5rem' };

import { useState, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { ProbeResult, Task } from '../types/api';
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
} from 'recharts';

const TASK_COLORS = ['#3b82f6', '#ef4444', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16'];
const EXTRA_COLORS = ['#3b82f6', '#ef4444', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16', '#78716c', '#9333ea'];

// Standard fields that can appear in results
const STANDARD_FIELDS: { key: keyof ProbeResult; label: string; unit: string; fmt: (v: any) => string }[] = [
  { key: 'latency_ms', label: 'Latency', unit: 'ms', fmt: (v: number) => `${v.toFixed(1)}ms` },
  { key: 'jitter_ms', label: 'Jitter', unit: 'ms', fmt: (v: number) => `${v.toFixed(2)}ms` },
  { key: 'packet_loss_pct', label: 'Loss%', unit: '%', fmt: (v: number) => `${v.toFixed(2)}%` },
  { key: 'dns_resolve_ms', label: 'DNS', unit: 'ms', fmt: (v: number) => `${v.toFixed(1)}ms` },
  { key: 'tls_handshake_ms', label: 'TLS', unit: 'ms', fmt: (v: number) => `${v.toFixed(1)}ms` },
  { key: 'status_code', label: 'Code', unit: '', fmt: (v: number) => String(v) },
  { key: 'download_bps', label: 'Download', unit: 'Mbps', fmt: (v: number) => `${(v / 1e6).toFixed(2)} Mbps` },
  { key: 'upload_bps', label: 'Upload', unit: 'Mbps', fmt: (v: number) => `${(v / 1e6).toFixed(2)} Mbps` },
];

export default function Results() {
  const [taskId, setTaskId] = useState('');
  const [timeRange, setTimeRange] = useState('1h');
  const [hiddenLines, setHiddenLines] = useState<Set<string>>(new Set());

  const toggleLine = (dataKey: string) => {
    setHiddenLines(prev => {
      const next = new Set(prev);
      next.has(dataKey) ? next.delete(dataKey) : next.add(dataKey);
      return next;
    });
  };

  const fromTime = () => {
    const now = new Date();
    const hours: Record<string, number> = { '1h': 1, '6h': 6, '24h': 24, '7d': 168 };
    now.setHours(now.getHours() - (hours[timeRange] ?? 1));
    return now.toISOString();
  };

  const params = new URLSearchParams({ limit: '500', from: fromTime() });
  if (taskId) params.set('task_id', taskId);

  const { data, isLoading } = useQuery({
    queryKey: ['results', taskId, timeRange],
    queryFn: () => api.getResults(params.toString()),
    refetchInterval: 10000,
  });

  const { data: tasksData } = useQuery({
    queryKey: ['tasks'],
    queryFn: () => api.getTasks(),
  });

  // Fetch latest results (one per task) to discover ALL task_ids including external probes
  const { data: latestData } = useQuery({
    queryKey: ['latestResults'],
    queryFn: api.getLatestResults,
  });

  const resultsDesc: ProbeResult[] = data?.data ?? [];
  const resultsAsc: ProbeResult[] = [...resultsDesc].reverse();
  const tasks: Task[] = tasksData?.data ?? [];
  const latestResults: ProbeResult[] = latestData?.data ?? [];

  const taskMap = useMemo(() => {
    const m = new Map<string, Task>();
    tasks.forEach(t => m.set(t.id, t));
    return m;
  }, [tasks]);

  // --- Auto-detect which fields are present in the data ---
  // A field is "present" if at least one result has a non-null, non-zero value for it.
  // This avoids showing columns like latency_ms=0 when the probe doesn't produce that metric.
  const presentStdFields = useMemo(() =>
    STANDARD_FIELDS.filter(f => resultsDesc.some(r => {
      const v = (r as any)[f.key];
      return v != null && v !== 0;
    })),
    [resultsDesc]
  );

  // Detect extra fields present in results (all types for table, numeric for charts)
  const presentExtraFields = useMemo(() => {
    const allKeys = new Set<string>();
    resultsDesc.forEach(r => {
      if (r.extra) Object.keys(r.extra).forEach(k => {
        const v = (r.extra as any)[k];
        if (v != null && v !== '' && v !== 0) allKeys.add(k);
      });
    });
    return [...allKeys].sort();
  }, [resultsDesc]);

  // Numeric-only extra fields (for chart lines)
  const numericExtraFields = useMemo(() =>
    presentExtraFields.filter(k =>
      resultsDesc.some(r => r.extra && typeof (r.extra as any)[k] === 'number' && (r.extra as any)[k] !== 0)
    ),
    [presentExtraFields, resultsDesc]
  );

  // Chartable fields: all numeric standard fields + all numeric extra fields
  const chartableStdFields = presentStdFields.filter(f =>
    f.key !== 'status_code' && resultsDesc.some(r => typeof (r as any)[f.key] === 'number')
  );

  const involvedTaskIds = useMemo(() => {
    const ids = new Set<string>();
    resultsDesc.forEach(r => ids.add(r.task_id));
    return [...ids];
  }, [resultsDesc]);

  const isMultiTask = !taskId && involvedTaskIds.length > 1;

  // Task name labels for multi-task chart
  const taskNameColorMap = useMemo(() => {
    const m = new Map<string, string>();
    involvedTaskIds.forEach((tid, i) => {
      const name = taskMap.get(tid)?.name ?? tid.replace('ext_', '');
      m.set(name, TASK_COLORS[i % TASK_COLORS.length]);
    });
    return m;
  }, [involvedTaskIds, taskMap]);

  // Multi-task chart data
  const multiTaskChartData = useMemo(() => {
    if (!isMultiTask) return [];
    const byTask = new Map<string, ProbeResult[]>();
    resultsAsc.forEach(r => {
      if (!byTask.has(r.task_id)) byTask.set(r.task_id, []);
      byTask.get(r.task_id)!.push(r);
    });
    const allTimes = resultsAsc.map(r => r.timestamp);
    const uniqueTimes = [...new Set(allTimes)].sort();
    const lookup = new Map<string, Map<string, number>>();
    byTask.forEach((results, tid) => {
      const m = new Map<string, number>();
      results.forEach(r => m.set(r.timestamp, r.latency_ms));
      lookup.set(tid, m);
    });
    return uniqueTimes.map(ts => {
      const row: Record<string, any> = { time: new Date(ts).toLocaleTimeString() };
      involvedTaskIds.forEach(tid => {
        const taskName = taskMap.get(tid)?.name ?? tid.replace('ext_', '');
        row[taskName] = lookup.get(tid)?.get(ts) ?? undefined;
      });
      return row;
    });
  }, [isMultiTask, resultsAsc, involvedTaskIds, taskMap]);

  // Single task chart data — auto-include all present fields
  const singleTaskChartData = useMemo(() => {
    if (isMultiTask) return [];
    return resultsAsc.map(r => {
      const extra = (r.extra ?? {}) as Record<string, any>;
      const row: Record<string, any> = {
        time: new Date(r.timestamp).toLocaleTimeString(),
      };
      // Standard fields
      chartableStdFields.forEach(f => {
        const v = (r as any)[f.key];
        if (v != null) {
          if (f.key === 'download_bps' || f.key === 'upload_bps') {
            row[f.label] = v / 1e6; // to Mbps
          } else {
            row[f.label] = v;
          }
        }
      });
      // Extra fields (numeric only for chart)
      numericExtraFields.forEach(k => {
        if (extra[k] != null && typeof extra[k] === 'number') {
          row[`extra:${k}`] = extra[k];
        }
      });
      return row;
    });
  }, [isMultiTask, resultsAsc, chartableStdFields, numericExtraFields]);

  // Build chart line definitions
  const chartLines = useMemo(() => {
    const lines: { key: string; name: string; color: string }[] = [];
    let ci = 0;
    chartableStdFields.forEach(f => {
      lines.push({ key: f.label, name: `${f.label} (${f.unit})`, color: EXTRA_COLORS[ci++ % EXTRA_COLORS.length] });
    });
    numericExtraFields.forEach(k => {
      lines.push({ key: `extra:${k}`, name: k, color: EXTRA_COLORS[ci++ % EXTRA_COLORS.length] });
    });
    return lines;
  }, [chartableStdFields, presentExtraFields]);

  const hasAnyData = resultsDesc.length > 0;

  const queryClient = useQueryClient();
  const clearMutation = useMutation({
    mutationFn: () => api.clearResults(taskId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['results'] }),
  });

  const handleClear = () => {
    if (!taskId) return;
    const taskName = taskMap.get(taskId)?.name ?? taskId;
    if (window.confirm(`Clear all history for "${taskName}"?`)) {
      clearMutation.mutate();
    }
  };

  // Detect primary Y-axis unit
  const primaryUnit = useMemo(() => {
    if (chartableStdFields.some(f => f.key === 'download_bps' || f.key === 'upload_bps')) return 'Mbps';
    if (chartableStdFields.some(f => f.unit === 'ms')) return 'ms';
    return '';
  }, [chartableStdFields]);

  return (
    <div>
      <h1 style={{ fontSize: '1.5rem', fontWeight: 600, marginBottom: '1rem' }}>Results</h1>

      <div style={{ display: 'flex', gap: '1rem', marginBottom: '1rem', alignItems: 'center' }}>
        <select value={taskId} onChange={e => { setTaskId(e.target.value); setHiddenLines(new Set()); }} style={selectStyle}>
          <option value="">All Tasks</option>
          {tasks.map((t) => (
            <option key={t.id} value={t.id}>{t.name} ({t.target}) [{t.probe_type}]</option>
          ))}
          {/* External probe virtual task IDs — discovered from latest results (one per task) */}
          {(() => {
            const taskIds = new Set(tasks.map(t => t.id));
            const externalIds = [...new Set(latestResults.map(r => r.task_id))].filter(id => !taskIds.has(id));
            return externalIds.map(id => (
              <option key={id} value={id}>{id.replace('ext_', '')} [external]</option>
            ));
          })()}
        </select>
        <select value={timeRange} onChange={e => setTimeRange(e.target.value)} style={selectStyle}>
          <option value="1h">Last 1 hour</option>
          <option value="6h">Last 6 hours</option>
          <option value="24h">Last 24 hours</option>
          <option value="7d">Last 7 days</option>
        </select>
        {taskId && (
          <button onClick={handleClear} disabled={clearMutation.isPending}
            style={{ padding: '0.5rem 0.75rem', border: '1px solid #fca5a5', borderRadius: 6, background: '#fff',
              color: '#ef4444', fontSize: '0.8rem', cursor: 'pointer' }}>
            {clearMutation.isPending ? 'Clearing...' : 'Clear History'}
          </button>
        )}
      </div>

      {isLoading ? <p>Loading...</p> : (
        <>
          {/* Chart */}
          {hasAnyData && (
            <div style={{ background: '#fff', borderRadius: 8, padding: '1rem', border: '1px solid #e5e7eb', marginBottom: '1rem' }}>
              <h2 style={{ fontSize: '1rem', fontWeight: 500, marginBottom: '0.5rem' }}>
                {isMultiTask ? 'Latency by Task' : 'Metrics Over Time'}
              </h2>
              <ResponsiveContainer width="100%" height={300}>
                {isMultiTask ? (
                  <LineChart data={multiTaskChartData}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="time" tick={{ fontSize: 11 }} />
                    <YAxis tick={{ fontSize: 11 }} unit="ms" />
                    <Tooltip />
                    <Legend onClick={(e) => toggleLine(e.dataKey as string)} wrapperStyle={{ cursor: 'pointer' }}
                      formatter={(value, entry) => (
                        <span style={{
                          color: hiddenLines.has(entry.dataKey as string) ? '#d1d5db' : (entry.color ?? '#333'),
                          textDecoration: hiddenLines.has(entry.dataKey as string) ? 'line-through' : 'none',
                        }}>{value}</span>
                      )} />
                    {[...taskNameColorMap.entries()].map(([name, color]) => (
                      <Line key={name} type="monotone" dataKey={name} stroke={color} name={name} dot={false} hide={hiddenLines.has(name)} connectNulls={false} />
                    ))}
                  </LineChart>
                ) : (
                  <LineChart data={singleTaskChartData}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="time" tick={{ fontSize: 11 }} />
                    <YAxis tick={{ fontSize: 11 }} unit={primaryUnit} />
                    <Tooltip />
                    <Legend onClick={(e) => toggleLine(e.dataKey as string)} wrapperStyle={{ cursor: 'pointer' }}
                      formatter={(value, entry) => (
                        <span style={{
                          color: hiddenLines.has(entry.dataKey as string) ? '#d1d5db' : (entry.color ?? '#333'),
                          textDecoration: hiddenLines.has(entry.dataKey as string) ? 'line-through' : 'none',
                        }}>{value}</span>
                      )} />
                    {chartLines.map(line => (
                      <Line key={line.key} type="monotone" dataKey={line.key} stroke={line.color}
                        name={line.name} dot={false} hide={hiddenLines.has(line.key)} />
                    ))}
                  </LineChart>
                )}
              </ResponsiveContainer>
            </div>
          )}

          {/* Table */}
          <div style={{ background: '#fff', borderRadius: 8, border: '1px solid #e5e7eb', maxHeight: 420, overflowY: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.875rem' }}>
              <thead style={{ position: 'sticky', top: 0, background: '#fff', zIndex: 1 }}>
                <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                  <th style={thStyle}>Time</th>
                  {isMultiTask && <th style={thStyle}>Task</th>}
                  <th style={thStyle}>Status</th>
                  {presentStdFields.map(f => <th key={f.key} style={thStyle}>{f.label}</th>)}
                  {presentExtraFields.map(k => <th key={k} style={{ ...thStyle, fontSize: '0.75rem' }}>{k}</th>)}
                  <th style={thStyle}>Error</th>
                </tr>
              </thead>
              <tbody>
                {resultsDesc.slice(0, 200).map((r) => {
                  const task = taskMap.get(r.task_id);
                  const extra = (r.extra ?? {}) as Record<string, any>;
                  return (
                    <tr key={r.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                      <td style={tdStyle}>{new Date(r.timestamp).toLocaleTimeString()}</td>
                      {isMultiTask && (
                        <td style={tdStyle}>
                          <span style={{
                            display: 'inline-block', padding: '1px 6px', borderRadius: 3, fontSize: '0.75rem',
                            background: taskNameColorMap.get(task?.name ?? r.task_id.replace('ext_', '')) ?? '#e5e7eb', color: '#fff',
                          }}>
                            {task?.name ?? r.task_id.replace('ext_', '')}
                          </span>
                        </td>
                      )}
                      <td style={tdStyle}>
                        <span style={{ color: r.success ? '#22c55e' : '#ef4444' }}>
                          {r.success ? 'OK' : 'FAIL'}
                        </span>
                      </td>
                      {presentStdFields.map(f => {
                        const v = (r as any)[f.key];
                        return <td key={f.key} style={tdStyle}>{v != null ? f.fmt(v) : '-'}</td>;
                      })}
                      {presentExtraFields.map(k => {
                        const v = extra[k];
                        let display = '-';
                        if (v != null) {
                          if (typeof v === 'boolean') display = v ? 'Yes' : 'No';
                          else if (typeof v === 'number') display = v % 1 === 0 ? String(v) : v.toFixed(2);
                          else display = String(v);
                        }
                        return <td key={k} style={tdStyle}>{display}</td>;
                      })}
                      <td style={tdStyle} title={r.error}>{r.error ? r.error.slice(0, 40) : '-'}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            {resultsDesc.length === 0 && <p style={{ padding: '2rem', textAlign: 'center', color: '#6b7280' }}>No results for this filter</p>}
          </div>
        </>
      )}
    </div>
  );
}

const selectStyle: React.CSSProperties = {
  padding: '0.5rem', border: '1px solid #d1d5db', borderRadius: 6,
  fontSize: '0.875rem', background: '#fff',
};
const thStyle: React.CSSProperties = { padding: '0.75rem 0.5rem', fontWeight: 500 };
const tdStyle: React.CSSProperties = { padding: '0.5rem' };

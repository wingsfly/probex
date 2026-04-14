import { useState, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { ProbeResult, Task } from '../types/api';
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
} from 'recharts';
import * as XLSX from 'sheetjs-style';

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
  const [customFrom, setCustomFrom] = useState('');
  const [customTo, setCustomTo] = useState('');
  const [hiddenLines, setHiddenLines] = useState<Set<string>>(new Set());

  const toggleLine = (dataKey: string) => {
    setHiddenLines(prev => {
      const next = new Set(prev);
      next.has(dataKey) ? next.delete(dataKey) : next.add(dataKey);
      return next;
    });
  };

  const isCustom = timeRange === 'custom';

  const fromTime = () => {
    if (isCustom && customFrom) return new Date(customFrom).toISOString();
    const now = new Date();
    const hours: Record<string, number> = { '1h': 1, '6h': 6, '24h': 24, '7d': 168 };
    now.setHours(now.getHours() - (hours[timeRange] ?? 1));
    return now.toISOString();
  };

  const toTime = () => {
    if (isCustom && customTo) return new Date(customTo).toISOString();
    return '';
  };

  // Scale limit by time range; larger ranges pull more data
  const limitByRange: Record<string, string> = { '1h': '2000', '6h': '5000', '24h': '15000', '7d': '50000', 'custom': '10000' };
  // Larger ranges: slower refresh (no point refreshing 7d data every 10s)
  const refreshByRange: Record<string, number | false> = { '1h': 10000, '6h': 30000, '24h': 60000, '7d': false, 'custom': false };
  const params = new URLSearchParams({ limit: limitByRange[timeRange] ?? '2000', from: fromTime() });
  const to = toTime();
  if (to) params.set('to', to);
  if (taskId) params.set('task_id', taskId);

  const { data, isLoading } = useQuery({
    queryKey: ['results', taskId, timeRange, customFrom, customTo],
    queryFn: () => api.getResults(params.toString()),
    refetchInterval: refreshByRange[timeRange] ?? 10000,
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

  // Downsample: keep at most MAX_CHART_POINTS for chart rendering
  const MAX_CHART_POINTS = 1000;

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
    const sampledTimes = downsample(uniqueTimes, MAX_CHART_POINTS);
    return sampledTimes.map((ts) => {
      const row: Record<string, any> = { time: new Date(ts).getTime() };
      involvedTaskIds.forEach(tid => {
        const taskName = taskMap.get(tid)?.name ?? tid.replace('ext_', '');
        row[taskName] = lookup.get(tid)?.get(ts) ?? undefined;
      });
      return row;
    });
  }, [isMultiTask, resultsAsc, involvedTaskIds, taskMap]);

  // Fields where we want MAX (not avg) to preserve spikes/anomalies
  const maxFields = new Set(['Loss%', 'Jitter', 'packet_loss_pct', 'jitter_ms',
    'effective_loss_pct', 'out_of_order_pct', 'retransmits']);

  // Single task chart data — bucket-aggregate to preserve anomalies
  const singleTaskChartData = useMemo(() => {
    if (isMultiTask) return [];
    if (resultsAsc.length <= MAX_CHART_POINTS) {
      // No need to aggregate, use raw data
      return resultsAsc.map(r => {
        const extra = (r.extra ?? {}) as Record<string, any>;
        const row: Record<string, any> = { time: new Date(r.timestamp).getTime() };
        chartableStdFields.forEach(f => {
          const v = (r as any)[f.key];
          if (v != null) row[f.label] = f.key === 'download_bps' || f.key === 'upload_bps' ? v / 1e6 : v;
        });
        numericExtraFields.forEach(k => {
          if (extra[k] != null && typeof extra[k] === 'number') row[`extra:${k}`] = extra[k];
        });
        return row;
      });
    }

    // Bucket-aggregate: split into MAX_CHART_POINTS buckets, compute avg/max per field
    const bucketSize = Math.ceil(resultsAsc.length / MAX_CHART_POINTS);
    const chartData: Record<string, any>[] = [];

    for (let i = 0; i < resultsAsc.length; i += bucketSize) {
      const bucket = resultsAsc.slice(i, Math.min(i + bucketSize, resultsAsc.length));
      const midpoint = bucket[Math.floor(bucket.length / 2)];
      const row: Record<string, any> = { time: new Date(midpoint.timestamp).getTime() };

      // For each field, collect values from the bucket
      chartableStdFields.forEach(f => {
        const vals: number[] = [];
        bucket.forEach(r => {
          let v = (r as any)[f.key];
          if (v != null) {
            if (f.key === 'download_bps' || f.key === 'upload_bps') v = v / 1e6;
            vals.push(v);
          }
        });
        if (vals.length > 0) {
          // Use MAX for anomaly fields (loss, jitter), AVG for everything else
          row[f.label] = maxFields.has(f.label) || maxFields.has(f.key)
            ? Math.max(...vals)
            : vals.reduce((s, v) => s + v, 0) / vals.length;
        }
      });

      numericExtraFields.forEach(k => {
        const vals: number[] = [];
        bucket.forEach(r => {
          const extra = (r.extra ?? {}) as Record<string, any>;
          if (extra[k] != null && typeof extra[k] === 'number') vals.push(extra[k]);
        });
        if (vals.length > 0) {
          row[`extra:${k}`] = maxFields.has(k)
            ? Math.max(...vals)
            : vals.reduce((s, v) => s + v, 0) / vals.length;
        }
      });

      chartData.push(row);
    }
    return chartData;
  }, [isMultiTask, resultsAsc, chartableStdFields, numericExtraFields, MAX_CHART_POINTS, maxFields]);

  // Build chart line definitions, each tagged with a yAxisId by unit group
  const chartLines = useMemo(() => {
    const lines: { key: string; name: string; color: string; yAxisId: string }[] = [];
    let ci = 0;
    chartableStdFields.forEach(f => {
      const yAxisId = (f.key === 'download_bps' || f.key === 'upload_bps') ? 'bps' : 'default';
      lines.push({ key: f.label, name: `${f.label} (${f.unit})`, color: EXTRA_COLORS[ci++ % EXTRA_COLORS.length], yAxisId });
    });
    numericExtraFields.forEach(k => {
      // available_outgoing_bitrate is bps-scale, everything else is small-scale
      const yAxisId = k === 'available_outgoing_bitrate' ? 'bps' : 'default';
      lines.push({ key: `extra:${k}`, name: k, color: EXTRA_COLORS[ci++ % EXTRA_COLORS.length], yAxisId });
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

  // Determine which Y-axis groups have visible (non-hidden) lines
  const hasBpsAxis = chartLines.some(l => l.yAxisId === 'bps' && !hiddenLines.has(l.key));
  const hasDefaultAxis = chartLines.some(l => l.yAxisId === 'default' && !hiddenLines.has(l.key));

  // Detect default-axis unit from visible lines
  const defaultAxisUnit = useMemo(() => {
    const visibleStd = chartableStdFields.filter(f => {
      const key = f.label;
      return !hiddenLines.has(key) && f.key !== 'download_bps' && f.key !== 'upload_bps';
    });
    if (visibleStd.length === 0) return '';
    const units = new Set(visibleStd.map(f => f.unit));
    return units.size === 1 ? [...units][0] : '';
  }, [chartableStdFields, hiddenLines]);

  // ---- Export: chart PNG + data Excel ----
  const handleExport = async () => {
    const taskLabel = taskId ? (taskMap.get(taskId)?.name ?? taskId).replace(/[^a-zA-Z0-9_-]/g, '_') : 'all';
    const now = new Date().toISOString().slice(0, 16).replace(/[T:]/g, '-');
    const baseName = `probex-${taskLabel}-${timeRange}-${now}`;

    // 1. Export chart as PNG
    const svgEl = document.querySelector('.recharts-responsive-container svg') as SVGSVGElement | null;
    if (svgEl) {
      try {
        const svgData = new XMLSerializer().serializeToString(svgEl);
        const rect = svgEl.getBoundingClientRect();
        const scale = 2;
        const canvas = document.createElement('canvas');
        canvas.width = rect.width * scale;
        canvas.height = rect.height * scale;
        const ctx = canvas.getContext('2d')!;
        ctx.scale(scale, scale);
        ctx.fillStyle = '#fff';
        ctx.fillRect(0, 0, rect.width, rect.height);
        const img = new Image();
        const blob = new Blob([svgData], { type: 'image/svg+xml;charset=utf-8' });
        const url = URL.createObjectURL(blob);
        await new Promise<void>((resolve) => {
          img.onload = () => { ctx.drawImage(img, 0, 0, rect.width, rect.height); URL.revokeObjectURL(url); resolve(); };
          img.onerror = () => { URL.revokeObjectURL(url); resolve(); };
          img.src = url;
        });
        canvas.toBlob((pngBlob) => {
          if (!pngBlob) return;
          const a = document.createElement('a');
          a.href = URL.createObjectURL(pngBlob);
          a.download = baseName + '-chart.png';
          a.click();
          URL.revokeObjectURL(a.href);
        }, 'image/png');
      } catch (e) {
        console.warn('Chart PNG export failed:', e);
      }
    }

    // 2. Export data table as Excel
    const wb = XLSX.utils.book_new();
    const headers: string[] = ['Time'];
    if (isMultiTask) headers.push('Task');
    headers.push('Status');
    presentStdFields.forEach(f => headers.push(f.label));
    presentExtraFields.forEach(k => headers.push(k));
    headers.push('Error');

    const dataRows = resultsDesc.slice(0, 5000).map(r => {
      const extra = (r.extra ?? {}) as Record<string, any>;
      const row: any[] = [new Date(r.timestamp).toLocaleString()];
      if (isMultiTask) row.push(taskMap.get(r.task_id)?.name ?? r.task_id);
      row.push(r.success ? 'OK' : 'FAIL');
      presentStdFields.forEach(f => {
        const v = (r as any)[f.key];
        if (v == null) { row.push(''); return; }
        if (f.key === 'download_bps' || f.key === 'upload_bps') row.push(Number((v / 1e6).toFixed(2)));
        else if (typeof v === 'number') row.push(Number(v.toFixed(2)));
        else row.push(v);
      });
      presentExtraFields.forEach(k => {
        const v = extra[k];
        if (v == null) row.push('');
        else if (typeof v === 'number') row.push(Number(v % 1 === 0 ? v : Number(v.toFixed(2))));
        else row.push(String(v));
      });
      row.push(r.error || '');
      return row;
    });

    const ws = XLSX.utils.aoa_to_sheet([headers, ...dataRows]);

    // Column width: based on data content (ignore long headers since they wrap)
    const dataWidths = headers.map((_h, i) => {
      let max = 8; // minimum width
      dataRows.slice(0, 200).forEach(row => {
        const v = String(row[i] ?? '');
        if (v.length > max) max = v.length;
      });
      return max;
    });
    // Use uniform width: pick a reasonable common width that fits most data
    const medianWidth = [...dataWidths].sort((a, b) => a - b)[Math.floor(dataWidths.length / 2)] || 12;
    const colWidth = Math.max(12, Math.min(medianWidth + 2, 22));
    ws['!cols'] = headers.map(() => ({ wch: colWidth }));

    // Header row: wrap text + bold style
    const headerRowIdx = 0;
    headers.forEach((_h, c) => {
      const cellRef = XLSX.utils.encode_cell({ r: headerRowIdx, c });
      if (ws[cellRef]) {
        ws[cellRef].s = {
          alignment: { wrapText: true, vertical: 'center', horizontal: 'center' },
          font: { bold: true, sz: 10 },
          fill: { patternType: 'solid', fgColor: { rgb: 'F3F4F6' } },
          border: {
            bottom: { style: 'thin', color: { rgb: 'D1D5DB' } },
          },
        };
      }
    });
    // Set header row height to accommodate wrapped text
    if (!ws['!rows']) ws['!rows'] = [];
    ws['!rows'][headerRowIdx] = { hpt: 40 }; // ~2.5 lines

    XLSX.utils.book_append_sheet(wb, ws, 'Results');
    XLSX.writeFile(wb, baseName + '.xlsx');
  };

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
          <option value="custom">Custom range</option>
        </select>
        {isCustom && (
          <>
            <input type="datetime-local" value={customFrom}
              onChange={e => setCustomFrom(e.target.value)}
              style={{ ...selectStyle, fontSize: '0.8rem' }}
              title="From" />
            <span style={{ color: '#6b7280', fontSize: '0.875rem' }}>to</span>
            <input type="datetime-local" value={customTo}
              onChange={e => setCustomTo(e.target.value)}
              style={{ ...selectStyle, fontSize: '0.8rem' }}
              title="To" />
          </>
        )}
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
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.5rem' }}>
                <h2 style={{ fontSize: '1rem', fontWeight: 500, margin: 0 }}>
                  {isMultiTask ? 'Latency by Task' : 'Metrics Over Time'}
                </h2>
                <div style={{ display: 'flex', gap: '0.5rem' }}>
                  <button onClick={() => setHiddenLines(new Set())}
                    style={legendBtnStyle} title="Show all metrics">All</button>
                  <button onClick={() => setHiddenLines(new Set(chartLines.map(l => l.key)))}
                    style={legendBtnStyle} title="Hide all metrics">None</button>
                  <button onClick={() => {
                    const allKeys = new Set(chartLines.map(l => l.key));
                    setHiddenLines(prev => {
                      const next = new Set<string>();
                      allKeys.forEach(k => { if (!prev.has(k)) next.add(k); });
                      return next;
                    });
                  }} style={legendBtnStyle} title="Invert selection">Invert</button>
                  <button onClick={handleExport}
                    style={{ ...legendBtnStyle, background: '#059669', color: '#fff', border: 'none' }}
                    title="Export chart & table to Excel">Export</button>
                </div>
              </div>
              <ResponsiveContainer width="100%" height={300}>
                {isMultiTask ? (
                  <LineChart data={multiTaskChartData}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="time" type="number" scale="time" domain={['dataMin', 'dataMax']}
                      ticks={generateTimeTicks(multiTaskChartData)}
                      tickFormatter={formatXTick} tick={{ fontSize: 10 }} angle={-25} textAnchor="end" height={50} />
                    <YAxis tick={{ fontSize: 11 }} unit="ms" />
                    <Tooltip labelFormatter={(v) => typeof v === 'number' ? formatTooltipTime(v) : v} />
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
                    <XAxis dataKey="time" type="number" scale="time" domain={['dataMin', 'dataMax']}
                      ticks={generateTimeTicks(singleTaskChartData)}
                      tickFormatter={formatXTick} tick={{ fontSize: 10 }} angle={-25} textAnchor="end" height={50} />
                    <YAxis yAxisId="default" tick={{ fontSize: 11 }} unit={defaultAxisUnit}
                      hide={!hasDefaultAxis} domain={['auto', 'auto']} />
                    <YAxis yAxisId="bps" orientation="right" tick={{ fontSize: 11 }} unit="Mbps"
                      hide={!hasBpsAxis} domain={['auto', 'auto']} />
                    <Tooltip labelFormatter={(v) => typeof v === 'number' ? formatTooltipTime(v) : v} />
                    <Legend onClick={(e) => toggleLine(e.dataKey as string)} wrapperStyle={{ cursor: 'pointer' }}
                      formatter={(value, entry) => (
                        <span style={{
                          color: hiddenLines.has(entry.dataKey as string) ? '#d1d5db' : (entry.color ?? '#333'),
                          textDecoration: hiddenLines.has(entry.dataKey as string) ? 'line-through' : 'none',
                        }}>{value}</span>
                      )} />
                    {chartLines.map(line => (
                      <Line key={line.key} type="monotone" dataKey={line.key} stroke={line.color}
                        yAxisId={line.yAxisId} name={line.name} dot={false} hide={hiddenLines.has(line.key)} connectNulls />
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
                {resultsDesc.slice(0, 500).map((r) => {
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

// Track the last displayed day to show date only at day boundaries on X axis.
let _lastTickDay = '';

function formatXTick(epochMs: number): string {
  const d = new Date(epochMs);
  const day = `${d.getMonth() + 1}/${d.getDate()}`;
  const time = d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  if (day !== _lastTickDay) {
    _lastTickDay = day;
    return `${day} ${time}`;
  }
  return time;
}

/** Generate evenly-spaced tick values so the X-axis never gets overcrowded. */
function generateTimeTicks(data: Record<string, any>[], maxTicks = 8): number[] {
  if (data.length === 0) return [];
  const times = data.map(d => d.time as number).filter(t => typeof t === 'number' && isFinite(t));
  if (times.length === 0) return [];
  const min = times[0];
  const max = times[times.length - 1];
  if (min === max) return [min];
  const count = Math.min(maxTicks, times.length);
  const step = (max - min) / (count - 1);
  const ticks: number[] = [];
  for (let i = 0; i < count; i++) {
    ticks.push(Math.round(min + step * i));
  }
  return ticks;
}

function formatTooltipTime(epochMs: number): string {
  return new Date(epochMs).toLocaleString();
}

// Downsample an array to at most maxPoints entries, evenly spaced.
function downsample<T>(arr: T[], maxPoints: number): T[] {
  if (arr.length <= maxPoints) return arr;
  const step = arr.length / maxPoints;
  const result: T[] = [];
  for (let i = 0; i < maxPoints; i++) {
    result.push(arr[Math.floor(i * step)]);
  }
  // Always include last element
  if (result[result.length - 1] !== arr[arr.length - 1]) {
    result.push(arr[arr.length - 1]);
  }
  return result;
}

const selectStyle: React.CSSProperties = {
  padding: '0.5rem', border: '1px solid #d1d5db', borderRadius: 6,
  fontSize: '0.875rem', background: '#fff',
};
const thStyle: React.CSSProperties = { padding: '0.75rem 0.5rem', fontWeight: 500 };
const tdStyle: React.CSSProperties = { padding: '0.5rem' };
const legendBtnStyle: React.CSSProperties = {
  padding: '2px 10px', border: '1px solid #d1d5db', borderRadius: 4,
  background: '#f9fafb', fontSize: '0.75rem', cursor: 'pointer', color: '#374151',
};

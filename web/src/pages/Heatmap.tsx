import { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import type { ProbeResult, Agent } from '../types/api';

// Pure CSS heatmap — no external charting library needed
export default function Heatmap() {
  const [metric, setMetric] = useState('latency_ms');
  const [timeRange, setTimeRange] = useState('24h');

  const hours: Record<string, number> = { '6h': 6, '12h': 12, '24h': 24, '7d': 168 };
  const fromTime = () => {
    const now = new Date();
    now.setHours(now.getHours() - (hours[timeRange] ?? 24));
    return now.toISOString();
  };

  const { data: agentsData } = useQuery({ queryKey: ['agents'], queryFn: api.getAgents });
  const { data: resultsData, isLoading } = useQuery({
    queryKey: ['heatmap', timeRange],
    queryFn: () => api.getResults(`limit=10000&from=${fromTime()}`),
    refetchInterval: 30000,
  });

  const agents: Agent[] = agentsData?.data ?? [];
  const results: ProbeResult[] = resultsData?.data ?? [];

  // Build heatmap data: rows = agents, columns = hour buckets
  const heatmapData = useMemo(() => {
    const hourCount = hours[timeRange] ?? 24;
    const bucketSize = hourCount <= 24 ? 1 : 4; // 1h buckets or 4h buckets
    const numBuckets = Math.ceil(hourCount / bucketSize);

    const now = new Date();
    const buckets: Date[] = [];
    for (let i = numBuckets - 1; i >= 0; i--) {
      const t = new Date(now);
      t.setHours(t.getHours() - i * bucketSize, 0, 0, 0);
      buckets.push(t);
    }

    const agentIds = [...new Set(results.map(r => r.agent_id))];
    const grid: { agentId: string; agentName: string; values: (number | null)[] }[] = [];

    for (const agentId of agentIds) {
      const agent = agents.find(a => a.id === agentId);
      const agentResults = results.filter(r => r.agent_id === agentId);
      const values: (number | null)[] = [];

      for (let i = 0; i < numBuckets; i++) {
        const bucketStart = buckets[i].getTime();
        const bucketEnd = bucketStart + bucketSize * 3600000;
        const inBucket = agentResults.filter(r => {
          const t = new Date(r.timestamp).getTime();
          return t >= bucketStart && t < bucketEnd;
        });

        if (inBucket.length === 0) {
          values.push(null);
        } else {
          const vals = inBucket.map(r => getMetricValue(r, metric)).filter(v => v !== null) as number[];
          values.push(vals.length > 0 ? vals.reduce((s, v) => s + v, 0) / vals.length : null);
        }
      }

      grid.push({
        agentId,
        agentName: agent?.name ?? agentId.slice(0, 8),
        values,
      });
    }

    return { grid, buckets, bucketSize };
  }, [results, agents, metric, timeRange]);

  // Color scale
  const allValues = heatmapData.grid.flatMap(r => r.values).filter(v => v !== null) as number[];
  const minVal = allValues.length > 0 ? Math.min(...allValues) : 0;
  const maxVal = allValues.length > 0 ? Math.max(...allValues) : 1;

  return (
    <div>
      <h1 style={{ fontSize: '1.5rem', fontWeight: 600, marginBottom: '1rem' }}>Heatmap</h1>

      <div style={{ display: 'flex', gap: '1rem', marginBottom: '1rem' }}>
        <select value={metric} onChange={e => setMetric(e.target.value)} style={selectStyle}>
          <option value="latency_ms">Latency (ms)</option>
          <option value="jitter_ms">Jitter (ms)</option>
          <option value="packet_loss_pct">Packet Loss (%)</option>
          <option value="success_rate">Success Rate (%)</option>
        </select>
        <select value={timeRange} onChange={e => setTimeRange(e.target.value)} style={selectStyle}>
          <option value="6h">Last 6 hours</option>
          <option value="12h">Last 12 hours</option>
          <option value="24h">Last 24 hours</option>
          <option value="7d">Last 7 days</option>
        </select>
      </div>

      {isLoading ? <p>Loading...</p> : (
        <div style={{ background: '#fff', borderRadius: 8, padding: '1rem', border: '1px solid #e5e7eb', overflowX: 'auto' }}>
          {heatmapData.grid.length === 0 ? (
            <p style={{ textAlign: 'center', color: '#6b7280', padding: '2rem' }}>No data for this period</p>
          ) : (
            <table style={{ borderCollapse: 'collapse', fontSize: '0.75rem' }}>
              <thead>
                <tr>
                  <th style={{ padding: '4px 8px', textAlign: 'left', fontWeight: 500 }}>Node</th>
                  {heatmapData.buckets.map((b, i) => (
                    <th key={i} style={{ padding: '4px 2px', fontWeight: 400, color: '#6b7280', minWidth: 28, textAlign: 'center' }}>
                      {b.getHours().toString().padStart(2, '0')}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {heatmapData.grid.map(row => (
                  <tr key={row.agentId}>
                    <td style={{ padding: '4px 8px', fontWeight: 500, whiteSpace: 'nowrap' }}>{row.agentName}</td>
                    {row.values.map((v, i) => (
                      <td key={i} style={{ padding: '2px' }}>
                        <div
                          title={v !== null ? `${v.toFixed(1)}` : 'no data'}
                          style={{
                            width: 24, height: 20, borderRadius: 3,
                            background: v !== null ? heatColor(v, minVal, maxVal, metric) : '#f3f4f6',
                          }}
                        />
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {/* Legend */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', marginTop: '0.75rem', fontSize: '0.7rem', color: '#6b7280' }}>
            <span>Low</span>
            <div style={{ display: 'flex', height: 12 }}>
              {[0, 0.2, 0.4, 0.6, 0.8, 1].map(p => (
                <div key={p} style={{
                  width: 20, height: 12,
                  background: heatColor(minVal + p * (maxVal - minVal), minVal, maxVal, metric),
                }} />
              ))}
            </div>
            <span>High</span>
          </div>
        </div>
      )}
    </div>
  );
}

function getMetricValue(r: ProbeResult, metric: string): number | null {
  if (metric === 'success_rate') return r.success ? 100 : 0;
  const v = (r as any)[metric];
  return v != null ? v : null;
}

function heatColor(value: number, min: number, max: number, metric: string): string {
  const range = max - min || 1;
  let ratio = (value - min) / range;
  // For success_rate, invert (high is good)
  if (metric === 'success_rate') ratio = 1 - ratio;
  // Green (good) → Yellow → Red (bad)
  if (ratio < 0.5) {
    const g = Math.round(180 + (220 - 180) * (ratio * 2));
    const r = Math.round(34 + (245 - 34) * (ratio * 2));
    return `rgb(${r}, ${g}, 34)`;
  }
  const r = Math.round(245 + (239 - 245) * ((ratio - 0.5) * 2));
  const g = Math.round(220 - 220 * ((ratio - 0.5) * 2));
  return `rgb(${r}, ${g}, 68)`;
}

const selectStyle: React.CSSProperties = {
  padding: '0.5rem', border: '1px solid #d1d5db', borderRadius: 6, fontSize: '0.875rem', background: '#fff',
};

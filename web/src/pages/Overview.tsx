import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import type { ProbeResult, Task, AlertRule, Agent } from '../types/api';
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
} from 'recharts';

const NODE_COLORS = ['#3b82f6', '#ef4444', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16'];

export default function Overview() {
  const { data: agentsData } = useQuery({ queryKey: ['agents'], queryFn: api.getAgents, refetchInterval: 5000 });
  const { data: tasksData } = useQuery({ queryKey: ['tasks'], queryFn: () => api.getTasks() });
  const { data: alertData } = useQuery({ queryKey: ['alertRules'], queryFn: () => api.getAlertRules(), refetchInterval: 5000 });
  const { data: latestData } = useQuery({ queryKey: ['latestResults'], queryFn: api.getLatestResults, refetchInterval: 5000 });
  const { data: recentData } = useQuery({
    queryKey: ['recentResults'],
    queryFn: () => {
      const from = new Date();
      from.setHours(from.getHours() - 1);
      return api.getResults(`limit=1000&from=${from.toISOString()}`);
    },
    refetchInterval: 10000,
  });

  const agents: Agent[] = agentsData?.data ?? [];
  const tasks: Task[] = tasksData?.data ?? [];
  const alerts: AlertRule[] = alertData?.data ?? [];
  const latestResults: ProbeResult[] = latestData?.data ?? [];
  const recentResults: ProbeResult[] = recentData?.data ?? [];

  const firingAlerts = alerts.filter(a => a.state === 'firing');
  const healthyAgents = agents.filter(a => a.status === 'healthy');
  const unhealthyAgents = agents.filter(a => a.status !== 'healthy');

  // Per-agent summary from latest results
  const agentSummary = useMemo(() => {
    const map = new Map<string, { results: ProbeResult[]; agent: Agent | undefined }>();
    agents.forEach(a => map.set(a.id, { results: [], agent: a }));
    latestResults.forEach(r => {
      const entry = map.get(r.agent_id);
      if (entry) entry.results.push(r);
    });
    return [...map.entries()].filter(([_, v]) => v.agent).map(([id, v]) => ({
      id,
      name: v.agent!.name,
      status: v.agent!.status,
      labels: v.agent!.labels,
      resultCount: v.results.length,
      avgLatency: v.results.length > 0
        ? v.results.reduce((s, r) => s + (r.latency_ms ?? 0), 0) / v.results.length : 0,
      successRate: v.results.length > 0
        ? (v.results.filter(r => r.success).length / v.results.length) * 100 : 100,
    }));
  }, [agents, latestResults]);

  // Multi-agent latency trend
  const trendData = useMemo(() => {
    const byAgent = new Map<string, ProbeResult[]>();
    [...recentResults].reverse().forEach(r => {
      if (!byAgent.has(r.agent_id)) byAgent.set(r.agent_id, []);
      byAgent.get(r.agent_id)!.push(r);
    });
    const allTimes = [...new Set(recentResults.map(r => r.timestamp))].sort();
    const sampled = allTimes.length > 60 ? allTimes.filter((_, i) => i % Math.ceil(allTimes.length / 60) === 0) : allTimes;
    return sampled.map(ts => {
      const row: Record<string, any> = { time: new Date(ts).toLocaleTimeString() };
      agents.forEach(a => {
        const results = byAgent.get(a.id) ?? [];
        const closest = results.find(r => r.timestamp === ts);
        if (closest && closest.latency_ms) row[a.name] = closest.latency_ms;
      });
      return row;
    });
  }, [recentResults, agents]);

  // Sparkline per agent
  const sparklines = useMemo(() => {
    const byAgent = new Map<string, { lat: number }[]>();
    [...recentResults].reverse().forEach(r => {
      if (!byAgent.has(r.agent_id)) byAgent.set(r.agent_id, []);
      const arr = byAgent.get(r.agent_id)!;
      if (arr.length < 20 && r.latency_ms) arr.push({ lat: r.latency_ms });
    });
    return byAgent;
  }, [recentResults]);

  return (
    <div>
      <h1 style={{ fontSize: '1.5rem', fontWeight: 600, marginBottom: '1rem' }}>Overview</h1>

      {/* Summary cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))', gap: '1rem', marginBottom: '1.5rem' }}>
        <Card label="Nodes" value={agents.length} />
        <Card label="Healthy" value={healthyAgents.length} color="#22c55e" />
        <Card label="Unhealthy" value={unhealthyAgents.length} color={unhealthyAgents.length > 0 ? '#ef4444' : '#22c55e'} />
        <Card label="Tasks" value={tasks.length} />
        <Card label="Firing Alerts" value={firingAlerts.length} color={firingAlerts.length > 0 ? '#dc2626' : '#22c55e'} />
      </div>

      {/* Firing alerts banner */}
      {firingAlerts.length > 0 && (
        <div style={{ background: '#fef2f2', border: '1px solid #fecaca', borderRadius: 8, padding: '0.75rem 1rem', marginBottom: '1rem' }}>
          <strong style={{ color: '#dc2626' }}>{firingAlerts.length} alert{firingAlerts.length > 1 ? 's' : ''} firing: </strong>
          <span style={{ color: '#991b1b', fontSize: '0.85rem' }}>{firingAlerts.map(a => a.name).join(', ')}</span>
        </div>
      )}

      {/* Node status grid */}
      <h2 style={{ fontSize: '1.1rem', fontWeight: 500, marginBottom: '0.75rem' }}>Nodes</h2>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))', gap: '1rem', marginBottom: '1.5rem' }}>
        {agentSummary.map(node => (
          <div key={node.id} style={{
            background: '#fff', border: `1px solid ${node.status === 'healthy' ? '#e5e7eb' : '#fecaca'}`,
            borderRadius: 8, padding: '1rem', borderLeft: `4px solid ${statusColor(node.status)}`,
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.5rem' }}>
              <span style={{ fontWeight: 600 }}>{node.name}</span>
              <span style={{ ...statusBadge, background: statusColor(node.status) + '20', color: statusColor(node.status) }}>
                {node.status}
              </span>
            </div>
            <div style={{ display: 'flex', gap: '1rem', fontSize: '0.8rem', color: '#6b7280' }}>
              <span>Avg: {node.avgLatency.toFixed(1)}ms</span>
              <span>OK: {node.successRate.toFixed(0)}%</span>
              <span>Tasks: {node.resultCount}</span>
            </div>
            {/* Sparkline */}
            {(sparklines.get(node.id)?.length ?? 0) > 1 && (
              <div style={{ marginTop: '0.5rem' }}>
                <ResponsiveContainer width="100%" height={30}>
                  <LineChart data={sparklines.get(node.id)}>
                    <YAxis hide domain={['dataMin', 'dataMax']} />
                    <Line type="monotone" dataKey="lat" stroke={statusColor(node.status)} dot={false} strokeWidth={1.5} />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Multi-node latency trend */}
      {trendData.length > 0 && agents.length > 0 && (
        <div style={{ background: '#fff', borderRadius: 8, padding: '1rem', border: '1px solid #e5e7eb', marginBottom: '1rem' }}>
          <h2 style={{ fontSize: '1.1rem', fontWeight: 500, marginBottom: '0.5rem' }}>Latency Trend (all nodes)</h2>
          <ResponsiveContainer width="100%" height={250}>
            <LineChart data={trendData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="time" tick={{ fontSize: 11 }} />
              <YAxis tick={{ fontSize: 11 }} unit="ms" />
              <Tooltip />
              <Legend />
              {agents.filter(a => a.id !== 'local' || agents.length === 1).map((a, i) => (
                <Line key={a.id} type="monotone" dataKey={a.name} stroke={NODE_COLORS[i % NODE_COLORS.length]}
                  dot={false} connectNulls={false} />
              ))}
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}

function Card({ label, value, color }: { label: string; value: string | number; color?: string }) {
  return (
    <div style={{ background: '#fff', borderRadius: 8, padding: '1rem', border: '1px solid #e5e7eb' }}>
      <div style={{ fontSize: '0.75rem', color: '#6b7280', textTransform: 'uppercase' }}>{label}</div>
      <div style={{ fontSize: '1.5rem', fontWeight: 700, color: color ?? '#111827', marginTop: 4 }}>{value}</div>
    </div>
  );
}

function statusColor(status: string): string {
  switch (status) {
    case 'healthy': return '#22c55e';
    case 'unhealthy': return '#f59e0b';
    case 'disconnected': return '#ef4444';
    default: return '#6b7280';
  }
}

const statusBadge: React.CSSProperties = {
  padding: '2px 8px', borderRadius: 4, fontSize: '0.7rem', fontWeight: 500,
};

import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import type { Agent } from '../types/api';

export default function Agents() {
  const { data, isLoading } = useQuery({
    queryKey: ['agents'],
    queryFn: api.getAgents,
    refetchInterval: 10000,
  });

  const agents: Agent[] = data?.data ?? [];

  const statusColor = (s: string) => {
    if (s === 'healthy') return '#22c55e';
    if (s === 'unhealthy') return '#f59e0b';
    return '#9ca3af';
  };

  return (
    <div>
      <h1 style={{ fontSize: '1.5rem', fontWeight: 600, marginBottom: '1rem' }}>Agents</h1>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: '1rem' }}>
        {agents.map((a) => (
          <div key={a.id} style={{ background: '#fff', borderRadius: 8, padding: '1rem', border: '1px solid #e5e7eb' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3 style={{ fontWeight: 600, fontSize: '1rem' }}>{a.name}</h3>
              <span style={{
                display: 'inline-block', padding: '2px 8px', borderRadius: 12,
                background: statusColor(a.status), color: '#fff', fontSize: '0.75rem',
              }}>
                {a.status}
              </span>
            </div>
            <div style={{ fontSize: '0.8rem', color: '#6b7280', marginTop: '0.5rem' }}>
              <p>ID: {a.id}</p>
              <p>Address: {a.address}</p>
              <p>Plugins: {a.plugins?.join(', ') || 'none'}</p>
              <p>Last heartbeat: {new Date(a.last_heartbeat).toLocaleString()}</p>
            </div>
          </div>
        ))}
      </div>
      {!isLoading && agents.length === 0 && (
        <p style={{ textAlign: 'center', color: '#6b7280', marginTop: '2rem' }}>No agents registered</p>
      )}
    </div>
  );
}

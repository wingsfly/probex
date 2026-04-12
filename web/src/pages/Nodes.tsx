import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import type { Agent } from '../types/api';

export default function Nodes() {
  const { data: agentsData, isLoading } = useQuery({
    queryKey: ['agents'],
    queryFn: api.getAgents,
    refetchInterval: 5000,
  });

  const agents: Agent[] = agentsData?.data ?? [];

  return (
    <div>
      <h1 style={{ fontSize: '1.5rem', fontWeight: 600, marginBottom: '1rem' }}>Nodes</h1>

      {isLoading ? <p>Loading...</p> : (
        <div style={{ background: '#fff', borderRadius: 8, border: '1px solid #e5e7eb' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.875rem' }}>
            <thead>
              <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                <th style={th}>Name</th>
                <th style={th}>Status</th>
                <th style={th}>Address</th>
                <th style={th}>Labels</th>
                <th style={th}>Plugins</th>
                <th style={th}>Last Heartbeat</th>
                <th style={th}>Registered</th>
              </tr>
            </thead>
            <tbody>
              {agents.map(a => (
                <tr key={a.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                  <td style={td}>
                    <span style={{ fontWeight: 500 }}>{a.name}</span>
                    <span style={{ fontSize: '0.7rem', color: '#9ca3af', marginLeft: 6 }}>{a.id.slice(0, 8)}</span>
                  </td>
                  <td style={td}>
                    <span style={{
                      display: 'inline-block', padding: '2px 8px', borderRadius: 4, fontSize: '0.7rem', fontWeight: 500,
                      background: statusBg(a.status), color: statusFg(a.status),
                    }}>
                      {a.status}
                    </span>
                  </td>
                  <td style={td}>{a.address || '-'}</td>
                  <td style={td}>
                    {Object.entries(a.labels || {}).map(([k, v]) => (
                      <span key={k} style={{ display: 'inline-block', padding: '1px 5px', borderRadius: 3, fontSize: '0.7rem', background: '#f3f4f6', marginRight: 4 }}>
                        {k}={v}
                      </span>
                    ))}
                  </td>
                  <td style={td}>
                    <span style={{ fontSize: '0.75rem', color: '#6b7280' }}>{a.plugins?.length ?? 0} probes</span>
                  </td>
                  <td style={td}>{new Date(a.last_heartbeat).toLocaleString()}</td>
                  <td style={td}>{new Date(a.registered_at).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {agents.length === 0 && <p style={{ padding: '2rem', textAlign: 'center', color: '#6b7280' }}>No nodes registered</p>}
        </div>
      )}
    </div>
  );
}

function statusBg(s: string): string {
  switch (s) {
    case 'healthy': return '#dcfce7';
    case 'unhealthy': return '#fef3c7';
    case 'disconnected': return '#fee2e2';
    default: return '#f3f4f6';
  }
}

function statusFg(s: string): string {
  switch (s) {
    case 'healthy': return '#166534';
    case 'unhealthy': return '#92400e';
    case 'disconnected': return '#991b1b';
    default: return '#6b7280';
  }
}

const th: React.CSSProperties = { padding: '0.75rem 0.5rem', fontWeight: 500 };
const td: React.CSSProperties = { padding: '0.5rem' };

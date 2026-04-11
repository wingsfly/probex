import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import type { ProbeMetadata } from '../types/api';

export default function Probes() {
  const [filter, setFilter] = useState<string>('all');
  const [search, setSearch] = useState('');
  const [expanded, setExpanded] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['probes'],
    queryFn: () => api.getProbes(),
  });

  const probes: ProbeMetadata[] = data?.data ?? [];

  const filtered = probes.filter(p => {
    if (filter !== 'all' && p.kind !== filter) return false;
    if (search && !p.name.toLowerCase().includes(search.toLowerCase()) &&
        !p.description?.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  const counts = { all: probes.length, builtin: 0, script: 0, external: 0 };
  probes.forEach(p => { counts[p.kind]++; });

  return (
    <div>
      <h1 style={{ fontSize: '1.5rem', fontWeight: 600, marginBottom: '1rem' }}>Probes</h1>

      <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem', alignItems: 'center', flexWrap: 'wrap' }}>
        {(['all', 'builtin', 'script', 'external'] as const).map(kind => (
          <button key={kind} onClick={() => setFilter(kind)} style={{
            padding: '0.4rem 0.75rem', border: '1px solid #d1d5db', borderRadius: 6,
            background: filter === kind ? kindBg(kind) : '#fff',
            color: filter === kind ? '#fff' : '#374151',
            cursor: 'pointer', fontSize: '0.8rem', textTransform: 'capitalize',
          }}>
            {kind} ({counts[kind]})
          </button>
        ))}
        <input value={search} onChange={e => setSearch(e.target.value)}
          placeholder="Search probes..."
          style={{ marginLeft: 'auto', padding: '0.4rem 0.75rem', border: '1px solid #d1d5db', borderRadius: 6, fontSize: '0.8rem', width: 200 }}
        />
      </div>

      {isLoading ? <p>Loading...</p> : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(340px, 1fr))', gap: '1rem' }}>
          {filtered.map(p => (
            <ProbeCard key={p.name} probe={p} isExpanded={expanded === p.name}
              onToggle={() => setExpanded(expanded === p.name ? null : p.name)} />
          ))}
        </div>
      )}
      {!isLoading && filtered.length === 0 && (
        <p style={{ padding: '2rem', textAlign: 'center', color: '#6b7280' }}>No probes match the filter</p>
      )}
    </div>
  );
}

function ProbeCard({ probe, isExpanded, onToggle }: { probe: ProbeMetadata; isExpanded: boolean; onToggle: () => void }) {
  const paramCount = probe.parameter_schema?.properties ? Object.keys(probe.parameter_schema.properties).length : 0;
  const standardFields = probe.output_schema?.standard_fields ?? [];
  const extraFields = probe.output_schema?.extra_fields ?? [];

  return (
    <div style={{
      background: '#fff', borderRadius: 8, border: '1px solid #e5e7eb',
      overflow: 'hidden', cursor: 'pointer', transition: 'box-shadow 0.15s',
    }}
      onClick={onToggle}
    >
      <div style={{ padding: '1rem' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '0.5rem' }}>
          <div>
            <span style={{ fontSize: '1rem', fontWeight: 600 }}>{probe.name}</span>
            <span style={{ ...badge, background: kindBg(probe.kind), marginLeft: 8 }}>{probe.kind}</span>
          </div>
          <span style={{ fontSize: '0.7rem', color: '#9ca3af' }}>{isExpanded ? '\u25B2' : '\u25BC'}</span>
        </div>
        <p style={{ fontSize: '0.8rem', color: '#6b7280', margin: 0, lineHeight: 1.4 }}>{probe.description || 'No description'}</p>

        <div style={{ display: 'flex', gap: '1rem', marginTop: '0.5rem', fontSize: '0.7rem', color: '#9ca3af' }}>
          <span>{paramCount} parameter{paramCount !== 1 ? 's' : ''}</span>
          <span>{standardFields.length + extraFields.length} output metric{standardFields.length + extraFields.length !== 1 ? 's' : ''}</span>
        </div>
      </div>

      {isExpanded && (
        <div style={{ borderTop: '1px solid #e5e7eb', padding: '1rem', background: '#f9fafb', fontSize: '0.8rem' }} onClick={e => e.stopPropagation()}>
          {/* Parameters */}
          {paramCount > 0 && (
            <div style={{ marginBottom: '0.75rem' }}>
              <h4 style={{ fontSize: '0.75rem', fontWeight: 600, color: '#374151', marginBottom: 4 }}>Parameters</h4>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                {Object.entries(probe.parameter_schema.properties ?? {}).map(([key, prop]) => (
                  <span key={key} style={{ background: '#e5e7eb', padding: '2px 6px', borderRadius: 3, fontSize: '0.7rem' }}>
                    {(prop as any).title || key}
                    <span style={{ color: '#9ca3af', marginLeft: 3 }}>({(prop as any).type})</span>
                  </span>
                ))}
              </div>
            </div>
          )}

          {/* Output metrics */}
          {(standardFields.length > 0 || extraFields.length > 0) && (
            <div>
              <h4 style={{ fontSize: '0.75rem', fontWeight: 600, color: '#374151', marginBottom: 4 }}>Output Metrics</h4>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                {standardFields.map(f => (
                  <span key={f} style={{ background: '#dbeafe', color: '#1d4ed8', padding: '2px 6px', borderRadius: 3, fontSize: '0.7rem' }}>{f}</span>
                ))}
                {extraFields.map(f => (
                  <span key={f.name} style={{ background: '#f3e8ff', color: '#7c3aed', padding: '2px 6px', borderRadius: 3, fontSize: '0.7rem' }}
                    title={f.description}>
                    {f.name}{f.unit ? ` (${f.unit})` : ''}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function kindBg(kind: string): string {
  switch (kind) {
    case 'builtin': return '#3b82f6';
    case 'script': return '#10b981';
    case 'external': return '#8b5cf6';
    default: return '#6b7280';
  }
}

const badge: React.CSSProperties = { display: 'inline-block', padding: '2px 6px', borderRadius: 3, color: '#fff', fontSize: '0.65rem', fontWeight: 500 };

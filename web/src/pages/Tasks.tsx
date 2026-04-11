import { useState, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { Task, ProbeMetadata } from '../types/api';
import SchemaForm from '../components/SchemaForm';

type FormMode = { type: 'create' } | { type: 'edit'; task: Task } | { type: 'clone'; task: Task };

export default function Tasks() {
  const queryClient = useQueryClient();
  const [formMode, setFormMode] = useState<FormMode | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['tasks'],
    queryFn: () => api.getTasks(),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteTask(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['tasks'] }),
  });

  const pauseMutation = useMutation({
    mutationFn: (id: string) => api.pauseTask(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['tasks'] }),
  });

  const resumeMutation = useMutation({
    mutationFn: (id: string) => api.resumeTask(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['tasks'] }),
  });

  const tasks: Task[] = data?.data ?? [];

  const formatInterval = (ns: number) => {
    const ms = ns / 1e6;
    if (ms >= 60000) return `${(ms / 60000).toFixed(0)}m`;
    if (ms >= 1000) return `${(ms / 1000).toFixed(0)}s`;
    return `${ms.toFixed(0)}ms`;
  };

  const closeForm = () => { setFormMode(null); queryClient.invalidateQueries({ queryKey: ['tasks'] }); };

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
        <h1 style={{ fontSize: '1.5rem', fontWeight: 600 }}>Tasks</h1>
        <button onClick={() => setFormMode(formMode ? null : { type: 'create' })} style={btnStyle}>
          {formMode ? 'Cancel' : '+ New Task'}
        </button>
      </div>

      {formMode && <TaskForm mode={formMode} onSuccess={closeForm} onCancel={() => setFormMode(null)} />}

      {isLoading ? <p>Loading...</p> : (
        <div style={{ background: '#fff', borderRadius: 8, border: '1px solid #e5e7eb' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.875rem' }}>
            <thead>
              <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                <th style={thStyle}>Name</th>
                <th style={thStyle}>Target</th>
                <th style={thStyle}>Type</th>
                <th style={thStyle}>Interval</th>
                <th style={thStyle}>Status</th>
                <th style={thStyle}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {tasks.map((t) => (
                <tr key={t.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                  <td style={tdStyle}>{t.name}</td>
                  <td style={tdStyle}>{t.target}</td>
                  <td style={tdStyle}>
                    <span style={{ ...tagStyle, background: probeColor(t.probe_type) }}>{t.probe_type}</span>
                  </td>
                  <td style={tdStyle}>{formatInterval(t.interval)}</td>
                  <td style={tdStyle}>
                    <span style={{ color: t.enabled ? '#22c55e' : '#9ca3af' }}>
                      {t.enabled ? 'Active' : 'Paused'}
                    </span>
                  </td>
                  <td style={tdStyle}>
                    {t.enabled ? (
                      <button onClick={() => pauseMutation.mutate(t.id)} style={{ ...smallBtn, color: '#f59e0b', borderColor: '#f59e0b' }}>Pause</button>
                    ) : (
                      <button onClick={() => resumeMutation.mutate(t.id)} style={{ ...smallBtn, color: '#22c55e', borderColor: '#22c55e' }}>Run</button>
                    )}
                    <button
                      onClick={() => !t.enabled && setFormMode({ type: 'edit', task: t })}
                      style={{ ...smallBtn, ...(t.enabled ? { color: '#d1d5db', borderColor: '#f3f4f6', cursor: 'not-allowed' } : {}) }}
                      title={t.enabled ? 'Pause task before editing' : 'Edit task'}
                      disabled={t.enabled}
                    >Edit</button>
                    <button onClick={() => setFormMode({ type: 'clone', task: t })} style={smallBtn} title="Clone task">Clone</button>
                    <button onClick={() => deleteMutation.mutate(t.id)} style={{ ...smallBtn, color: '#ef4444' }}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {tasks.length === 0 && <p style={{ padding: '2rem', textAlign: 'center', color: '#6b7280' }}>No tasks yet</p>}
        </div>
      )}
    </div>
  );
}

function durationStr(ns: number): string {
  const ms = ns / 1e6;
  if (ms >= 60000) return `${(ms / 60000).toFixed(0)}m`;
  if (ms >= 1000) return `${(ms / 1000).toFixed(0)}s`;
  return `${ms.toFixed(0)}ms`;
}

function TaskForm({ mode, onSuccess, onCancel }: { mode: FormMode; onSuccess: () => void; onCancel: () => void }) {
  const isEdit = mode.type === 'edit';
  const isClone = mode.type === 'clone';
  const seed = isEdit || isClone ? mode.task : null;
  const seedCfg = (seed?.config ?? {}) as Record<string, unknown>;

  const [name, setName] = useState(isClone && seed ? `${seed.name} (copy)` : seed?.name ?? '');
  const [target, setTarget] = useState(seed?.target ?? '');
  const [probeType, setProbeType] = useState(seed?.probe_type ?? 'icmp');
  const [interval, setInterval] = useState(seed ? durationStr(seed.interval) : '30s');
  const [timeout, setTimeout] = useState(seed ? durationStr(seed.timeout) : '10s');
  const [enableNow, setEnableNow] = useState(true);
  const [config, setConfig] = useState<Record<string, any>>(seedCfg as Record<string, any>);
  const [error, setError] = useState('');

  // Fetch probes from API for dynamic form generation
  const { data: probesData } = useQuery({
    queryKey: ['probes'],
    queryFn: () => api.getProbes(),
  });

  const probes: ProbeMetadata[] = probesData?.data ?? [];

  const selectedProbe = useMemo(() =>
    probes.find(p => p.name === probeType),
    [probes, probeType]
  );

  // When probe type changes, reset config to defaults from schema
  const handleProbeTypeChange = (newType: string) => {
    setProbeType(newType);
    const probe = probes.find(p => p.name === newType);
    if (probe?.parameter_schema?.properties) {
      const defaults: Record<string, any> = {};
      for (const [k, v] of Object.entries(probe.parameter_schema.properties)) {
        if (v.default !== undefined) defaults[k] = v.default;
      }
      setConfig(defaults);
    } else {
      setConfig({});
    }
  };

  const isExternalProbe = selectedProbe?.kind === 'external';

  const createMutation = useMutation({
    mutationFn: (body: any) => api.createTask(body),
    onSuccess: () => onSuccess(),
    onError: (e: Error) => setError(e.message),
  });

  const updateMutation = useMutation({
    mutationFn: (body: any) => api.updateTask(seed!.id, body),
    onSuccess: () => onSuccess(),
    onError: (e: Error) => setError(e.message),
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    const body: any = { name, target, probe_type: probeType, interval, timeout, config, enabled: enableNow };
    if (isEdit) {
      updateMutation.mutate(body);
    } else {
      createMutation.mutate(body);
    }
  };

  const isPending = createMutation.isPending || updateMutation.isPending;
  const title = isEdit ? 'Edit Task' : isClone ? 'Clone Task' : 'New Task';
  const submitLabel = isEdit ? 'Save Changes' : isClone ? 'Create Clone' : 'Create Task';

  // Executable probes only (not external)
  const executableProbes = probes.filter(p => p.kind !== 'external');

  return (
    <form onSubmit={handleSubmit} style={{ background: '#fff', borderRadius: 8, padding: '1.5rem', border: `2px solid ${isEdit ? '#f59e0b' : isClone ? '#8b5cf6' : '#e5e7eb'}`, marginBottom: '1rem' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
        <h3 style={{ fontSize: '0.95rem', fontWeight: 600, color: '#374151' }}>
          {title}
          {isEdit && <span style={{ fontWeight: 400, color: '#9ca3af', marginLeft: 8, fontSize: '0.8rem' }}>ID: {seed!.id.slice(0, 12)}</span>}
        </h3>
        <button type="button" onClick={onCancel} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#9ca3af', fontSize: '1.2rem' }}>x</button>
      </div>
      {error && <p style={{ color: '#ef4444', marginBottom: '0.5rem' }}>{error}</p>}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
        <div>
          <label style={labelStyle}>Name</label>
          <input value={name} onChange={e => setName(e.target.value)} style={inputStyle} placeholder="My ping test" required />
        </div>
        <div>
          <label style={labelStyle}>Target</label>
          <input value={target} onChange={e => setTarget(e.target.value)} style={inputStyle}
            placeholder="8.8.8.8 or https://example.com" required={!isExternalProbe} disabled={isExternalProbe} />
        </div>
        <div>
          <label style={labelStyle}>Probe Type</label>
          <select value={probeType} onChange={e => handleProbeTypeChange(e.target.value)} style={inputStyle}>
            {executableProbes.length > 0 ? (
              executableProbes.map(p => (
                <option key={p.name} value={p.name}>
                  {p.name}{p.kind === 'script' ? ' (script)' : ''} — {p.description?.slice(0, 50) ?? ''}
                </option>
              ))
            ) : (
              <>
                <option value="icmp">ICMP Ping</option>
                <option value="tcp">TCP Connect</option>
                <option value="http">HTTP(S)</option>
                <option value="dns">DNS</option>
                <option value="iperf3">iPerf3 Bandwidth</option>
              </>
            )}
          </select>
          {selectedProbe && (
            <p style={{ fontSize: '0.7rem', color: '#6b7280', marginTop: 2 }}>
              <span style={{ ...kindBadge, background: kindColor(selectedProbe.kind) }}>{selectedProbe.kind}</span>
              {selectedProbe.description}
            </p>
          )}
        </div>
        <div>
          <label style={labelStyle}>Probe Interval</label>
          <input value={interval} onChange={e => setInterval(e.target.value)} style={inputStyle} placeholder="30s" />
        </div>
      </div>

      {/* Dynamic parameter form from probe schema */}
      {selectedProbe?.parameter_schema?.properties && Object.keys(selectedProbe.parameter_schema.properties).length > 0 && (
        <div style={{ borderTop: '1px solid #e5e7eb', marginTop: '1rem', paddingTop: '1rem' }}>
          <h3 style={{ fontSize: '0.85rem', fontWeight: 600, marginBottom: '0.75rem', color: '#374151' }}>
            {selectedProbe.name} Parameters
          </h3>
          <SchemaForm
            schema={selectedProbe.parameter_schema}
            values={config}
            onChange={setConfig}
          />
        </div>
      )}

      <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginTop: '1rem' }}>
        <button type="submit" style={{ ...btnStyle, background: isEdit ? '#f59e0b' : '#3b82f6' }} disabled={isPending}>
          {isPending ? 'Saving...' : submitLabel}
        </button>
        <label style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: '0.8rem', color: '#374151', cursor: 'pointer' }}>
          <input type="checkbox" checked={enableNow} onChange={e => setEnableNow(e.target.checked)} />
          Enable immediately
        </label>
      </div>
    </form>
  );
}

const probeColor = (t: string) => {
  const colors: Record<string, string> = { icmp: '#3b82f6', tcp: '#8b5cf6', http: '#f59e0b', dns: '#10b981', iperf3: '#ef4444' };
  return colors[t] ?? '#6b7280';
};

const kindColor = (kind: string) => {
  switch (kind) {
    case 'builtin': return '#3b82f6';
    case 'script': return '#10b981';
    case 'external': return '#8b5cf6';
    default: return '#6b7280';
  }
};

const thStyle: React.CSSProperties = { padding: '0.75rem 0.5rem', fontWeight: 500 };
const tdStyle: React.CSSProperties = { padding: '0.75rem 0.5rem' };
const tagStyle: React.CSSProperties = { display: 'inline-block', padding: '2px 8px', borderRadius: 4, color: '#fff', fontSize: '0.75rem', fontWeight: 500 };
const kindBadge: React.CSSProperties = { display: 'inline-block', padding: '1px 5px', borderRadius: 3, color: '#fff', fontSize: '0.65rem', fontWeight: 500, marginRight: 4 };
const btnStyle: React.CSSProperties = { background: '#3b82f6', color: '#fff', border: 'none', borderRadius: 6, padding: '0.5rem 1rem', cursor: 'pointer', fontSize: '0.875rem' };
const smallBtn: React.CSSProperties = { background: 'none', border: '1px solid #e5e7eb', borderRadius: 4, padding: '2px 8px', cursor: 'pointer', fontSize: '0.75rem', marginRight: 4 };
const labelStyle: React.CSSProperties = { display: 'block', fontSize: '0.75rem', fontWeight: 500, marginBottom: 4, color: '#374151' };
const inputStyle: React.CSSProperties = { width: '100%', padding: '0.5rem', border: '1px solid #d1d5db', borderRadius: 6, fontSize: '0.875rem', boxSizing: 'border-box' as const };

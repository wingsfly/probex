import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { Report, Task, ReportCreate } from '../types/api';

export default function Reports() {
  const queryClient = useQueryClient();
  const [showModal, setShowModal] = useState(false);

  const { data: reportsData, isLoading } = useQuery({
    queryKey: ['reports'],
    queryFn: () => api.getReports(),
    refetchInterval: 3000,
  });

  const { data: tasksData } = useQuery({
    queryKey: ['tasks'],
    queryFn: () => api.getTasks(),
  });

  const reports: Report[] = reportsData?.data ?? [];
  const tasks: Task[] = tasksData?.data ?? [];

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteReport(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['reports'] }),
  });

  const handleDownload = async (report: Report) => {
    const blob = await api.downloadReport(report.id);
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${report.name}.${report.format}`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
        <h1 style={{ fontSize: '1.5rem', fontWeight: 600 }}>Reports</h1>
        <button onClick={() => setShowModal(true)} style={btnPrimary}>New Report</button>
      </div>

      {isLoading ? <p>Loading...</p> : (
        <div style={{ background: '#fff', borderRadius: 8, border: '1px solid #e5e7eb' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.875rem' }}>
            <thead>
              <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                <th style={thStyle}>Name</th>
                <th style={thStyle}>Format</th>
                <th style={thStyle}>Status</th>
                <th style={thStyle}>Time Range</th>
                <th style={thStyle}>Tasks</th>
                <th style={thStyle}>Created</th>
                <th style={thStyle}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {reports.map(r => (
                <tr key={r.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                  <td style={tdStyle}>{r.name}</td>
                  <td style={tdStyle}>
                    <span style={{ ...badge, background: r.format === 'html' ? '#dbeafe' : '#f3e8ff', color: r.format === 'html' ? '#1d4ed8' : '#7c3aed' }}>
                      {r.format.toUpperCase()}
                    </span>
                  </td>
                  <td style={tdStyle}>
                    <span style={{ ...badge, ...statusStyle(r.status) }}>{r.status}</span>
                  </td>
                  <td style={tdStyle}>
                    <span style={{ fontSize: '0.8rem', color: '#6b7280' }}>
                      {new Date(r.time_range_start).toLocaleDateString()} - {new Date(r.time_range_end).toLocaleDateString()}
                    </span>
                  </td>
                  <td style={tdStyle}>{r.task_ids.length} task(s)</td>
                  <td style={tdStyle}>{new Date(r.created_at).toLocaleString()}</td>
                  <td style={tdStyle}>
                    <div style={{ display: 'flex', gap: '0.5rem' }}>
                      {r.status === 'completed' && (
                        <button onClick={() => handleDownload(r)} style={btnSmall}>Download</button>
                      )}
                      <button
                        onClick={() => { if (window.confirm('Delete this report?')) deleteMutation.mutate(r.id); }}
                        style={{ ...btnSmall, color: '#ef4444', borderColor: '#fca5a5' }}
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {reports.length === 0 && (
            <p style={{ padding: '2rem', textAlign: 'center', color: '#6b7280' }}>No reports yet</p>
          )}
        </div>
      )}

      {showModal && (
        <CreateReportModal tasks={tasks} onClose={() => setShowModal(false)} />
      )}
    </div>
  );
}

function CreateReportModal({ tasks, onClose }: { tasks: Task[]; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState('');
  const [selectedTasks, setSelectedTasks] = useState<string[]>([]);
  const [format, setFormat] = useState<'html' | 'json'>('html');
  const [timePreset, setTimePreset] = useState('24h');
  const [customStart, setCustomStart] = useState('');
  const [customEnd, setCustomEnd] = useState('');

  const createMutation = useMutation({
    mutationFn: (body: ReportCreate) => api.createReport(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['reports'] });
      onClose();
    },
  });

  const toggleTask = (id: string) => {
    setSelectedTasks(prev => prev.includes(id) ? prev.filter(t => t !== id) : [...prev, id]);
  };

  const getTimeRange = () => {
    if (timePreset === 'custom') {
      return { start: customStart, end: customEnd };
    }
    const end = new Date();
    const start = new Date();
    const hours: Record<string, number> = { '1h': 1, '6h': 6, '24h': 24, '7d': 168, '30d': 720 };
    start.setHours(start.getHours() - (hours[timePreset] ?? 24));
    return { start: start.toISOString(), end: end.toISOString() };
  };

  const handleSubmit = () => {
    if (!name || selectedTasks.length === 0) return;
    const range = getTimeRange();
    createMutation.mutate({
      name,
      task_ids: selectedTasks,
      time_range_start: range.start,
      time_range_end: range.end,
      format,
    });
  };

  return (
    <div style={overlay}>
      <div style={modal}>
        <h2 style={{ fontSize: '1.25rem', fontWeight: 600, marginBottom: '1rem' }}>New Report</h2>

        <label style={labelStyle}>Name</label>
        <input value={name} onChange={e => setName(e.target.value)} style={inputStyle} placeholder="e.g. Weekly Network Report" />

        <label style={labelStyle}>Tasks</label>
        <div style={{ border: '1px solid #d1d5db', borderRadius: 6, maxHeight: 160, overflowY: 'auto', padding: '0.5rem' }}>
          {tasks.map(t => (
            <label key={t.id} style={{ display: 'flex', gap: '0.5rem', padding: '0.25rem 0', fontSize: '0.875rem', cursor: 'pointer' }}>
              <input type="checkbox" checked={selectedTasks.includes(t.id)} onChange={() => toggleTask(t.id)} />
              {t.name} ({t.target}) [{t.probe_type}]
            </label>
          ))}
          {tasks.length === 0 && <p style={{ color: '#6b7280', fontSize: '0.8rem' }}>No tasks available</p>}
        </div>

        <label style={labelStyle}>Time Range</label>
        <select value={timePreset} onChange={e => setTimePreset(e.target.value)} style={inputStyle}>
          <option value="1h">Last 1 hour</option>
          <option value="6h">Last 6 hours</option>
          <option value="24h">Last 24 hours</option>
          <option value="7d">Last 7 days</option>
          <option value="30d">Last 30 days</option>
          <option value="custom">Custom</option>
        </select>
        {timePreset === 'custom' && (
          <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
            <input type="datetime-local" value={customStart} onChange={e => setCustomStart(e.target.value)} style={{ ...inputStyle, flex: 1 }} />
            <input type="datetime-local" value={customEnd} onChange={e => setCustomEnd(e.target.value)} style={{ ...inputStyle, flex: 1 }} />
          </div>
        )}

        <label style={labelStyle}>Format</label>
        <select value={format} onChange={e => setFormat(e.target.value as 'html' | 'json')} style={inputStyle}>
          <option value="html">HTML</option>
          <option value="json">JSON</option>
        </select>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem', marginTop: '1.5rem' }}>
          <button onClick={onClose} style={btnCancel}>Cancel</button>
          <button onClick={handleSubmit} disabled={!name || selectedTasks.length === 0 || createMutation.isPending} style={btnPrimary}>
            {createMutation.isPending ? 'Creating...' : 'Generate Report'}
          </button>
        </div>
      </div>
    </div>
  );
}

function statusStyle(status: string): React.CSSProperties {
  switch (status) {
    case 'completed': return { background: '#dcfce7', color: '#166534' };
    case 'generating': return { background: '#dbeafe', color: '#1d4ed8' };
    case 'pending': return { background: '#f3f4f6', color: '#6b7280' };
    case 'failed': return { background: '#fee2e2', color: '#991b1b' };
    default: return {};
  }
}

const thStyle: React.CSSProperties = { padding: '0.75rem 0.5rem', fontWeight: 500 };
const tdStyle: React.CSSProperties = { padding: '0.5rem' };
const badge: React.CSSProperties = { display: 'inline-block', padding: '2px 8px', borderRadius: 4, fontSize: '0.75rem', fontWeight: 500 };
const overlay: React.CSSProperties = { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100 };
const modal: React.CSSProperties = { background: '#fff', borderRadius: 12, padding: '1.5rem', width: 520, maxHeight: '80vh', overflowY: 'auto' };
const labelStyle: React.CSSProperties = { display: 'block', fontSize: '0.8rem', fontWeight: 500, color: '#374151', marginTop: '1rem', marginBottom: '0.25rem' };
const inputStyle: React.CSSProperties = { width: '100%', padding: '0.5rem', border: '1px solid #d1d5db', borderRadius: 6, fontSize: '0.875rem' };
const btnPrimary: React.CSSProperties = { padding: '0.5rem 1rem', background: '#3b82f6', color: '#fff', border: 'none', borderRadius: 6, cursor: 'pointer', fontSize: '0.875rem' };
const btnCancel: React.CSSProperties = { padding: '0.5rem 1rem', background: '#fff', color: '#374151', border: '1px solid #d1d5db', borderRadius: 6, cursor: 'pointer', fontSize: '0.875rem' };
const btnSmall: React.CSSProperties = { padding: '0.25rem 0.5rem', background: '#fff', border: '1px solid #d1d5db', borderRadius: 4, cursor: 'pointer', fontSize: '0.75rem' };

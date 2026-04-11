const BASE_URL = '/api/v1';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    ...options,
  });
  const json = await res.json();
  if (json.error) throw new Error(json.error);
  return json;
}

export const api = {
  // Tasks
  getTasks: (params?: string) =>
    request<any>(`/tasks${params ? '?' + params : ''}`),
  getTask: (id: string) => request<any>(`/tasks/${id}`),
  createTask: (body: any) =>
    request<any>('/tasks', { method: 'POST', body: JSON.stringify(body) }),
  updateTask: (id: string, body: any) =>
    request<any>(`/tasks/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteTask: (id: string) =>
    request<any>(`/tasks/${id}`, { method: 'DELETE' }),
  pauseTask: (id: string) =>
    request<any>(`/tasks/${id}/pause`, { method: 'POST' }),
  resumeTask: (id: string) =>
    request<any>(`/tasks/${id}/resume`, { method: 'POST' }),
  runTask: (id: string) =>
    request<any>(`/tasks/${id}/run`, { method: 'POST' }),

  // Results
  getResults: (params?: string) =>
    request<any>(`/results${params ? '?' + params : ''}`),
  getResultSummary: (params?: string) =>
    request<any>(`/results/summary${params ? '?' + params : ''}`),
  getLatestResults: () => request<any>('/results/latest'),
  clearResults: (taskId: string) =>
    request<any>(`/results?task_id=${taskId}`, { method: 'DELETE' }),

  // Agents
  getAgents: () => request<any>('/agents'),

  // Plugins (legacy)
  getPlugins: () => request<any>('/plugins'),

  // Probes (unified registry)
  getProbes: () => request<any>('/probes'),
  getProbe: (name: string) => request<any>(`/probes/${name}`),

  // Reports
  getReports: () => request<any>('/reports'),
  getReport: (id: string) => request<any>(`/reports/${id}`),
  createReport: (body: any) =>
    request<any>('/reports', { method: 'POST', body: JSON.stringify(body) }),
  deleteReport: (id: string) =>
    request<any>(`/reports/${id}`, { method: 'DELETE' }),
  downloadReport: (id: string) =>
    fetch(`${BASE_URL}/reports/${id}/download`).then(res => res.blob()),

  // Alert Rules
  getAlertRules: () => request<any>('/alerts/rules'),
  createAlertRule: (body: any) =>
    request<any>('/alerts/rules', { method: 'POST', body: JSON.stringify(body) }),
  updateAlertRule: (id: string, body: any) =>
    request<any>(`/alerts/rules/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteAlertRule: (id: string) =>
    request<any>(`/alerts/rules/${id}`, { method: 'DELETE' }),

  // Alert Events
  getAlertEvents: (params?: string) =>
    request<any>(`/alerts/events${params ? '?' + params : ''}`),
};

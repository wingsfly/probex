export interface Task {
  id: string;
  name: string;
  target: string;
  probe_type: string;
  interval: number;
  timeout: number;
  config: Record<string, unknown>;
  agent_selector: Record<string, unknown>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface TaskCreate {
  name: string;
  target: string;
  probe_type: string;
  interval: string;
  timeout: string;
  config?: Record<string, unknown>;
  enabled?: boolean;
}

export interface Agent {
  id: string;
  name: string;
  labels: Record<string, string>;
  address: string;
  plugins: string[];
  status: 'healthy' | 'unhealthy' | 'disconnected';
  last_heartbeat: string;
  registered_at: string;
}

export interface ProbeResult {
  id: string;
  task_id: string;
  agent_id: string;
  timestamp: string;
  success: boolean;
  latency_ms: number;
  jitter_ms?: number;
  packet_loss_pct?: number;
  dns_resolve_ms?: number;
  tls_handshake_ms?: number;
  status_code?: number;
  download_bps?: number;
  upload_bps?: number;
  error?: string;
  extra?: Record<string, unknown>;
}

export interface ResultSummary {
  task_id: string;
  agent_id: string;
  count: number;
  success_rate: number;
  avg_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
  min_latency_ms: number;
  max_latency_ms: number;
  avg_jitter_ms: number;
  avg_loss_pct: number;
}

export interface Report {
  id: string;
  name: string;
  task_ids: string[];
  agent_ids: string[];
  time_range_start: string;
  time_range_end: string;
  format: 'html' | 'json';
  status: 'pending' | 'generating' | 'completed' | 'failed';
  file_path?: string;
  created_at: string;
  generated_at?: string;
}

export interface ReportCreate {
  name: string;
  task_ids: string[];
  agent_ids?: string[];
  time_range_start: string;
  time_range_end: string;
  format: 'html' | 'json';
}

export interface AlertRule {
  id: string;
  name: string;
  task_id: string;
  metric: string;
  operator: string;
  threshold: number;
  consecutive_count: number;
  enabled: boolean;
  severity: 'warning' | 'critical';
  webhook_url?: string;
  slack_webhook_url?: string;
  state: 'ok' | 'firing';
  last_triggered_at?: string;
  created_at: string;
  updated_at: string;
}

export interface AlertRuleCreate {
  name: string;
  task_id: string;
  metric: string;
  operator: string;
  threshold: number;
  consecutive_count?: number;
  severity?: 'warning' | 'critical';
  webhook_url?: string;
  slack_webhook_url?: string;
  enabled?: boolean;
}

export interface AlertEvent {
  id: string;
  rule_id: string;
  rule_name: string;
  task_id: string;
  metric: string;
  value: number;
  threshold: number;
  severity: 'warning' | 'critical';
  fired_at: string;
}

export interface ProbeMetadata {
  name: string;
  kind: 'builtin' | 'script' | 'external';
  description: string;
  version?: string;
  parameter_schema: JSONSchema;
  output_schema?: {
    standard_fields: string[];
    extra_fields?: Array<{
      name: string;
      type: string;
      unit?: string;
      description?: string;
      chartable?: boolean;
    }>;
  };
}

export interface JSONSchema {
  type: string;
  properties?: Record<string, JSONSchemaProperty>;
  required?: string[];
  'x-ui-order'?: string[];
  [key: string]: unknown;
}

export interface JSONSchemaProperty {
  type: string;
  title?: string;
  description?: string;
  default?: unknown;
  enum?: unknown[];
  minimum?: number;
  maximum?: number;
  'x-ui-placeholder'?: string;
  'x-ui-widget'?: string;
  'x-ui-group'?: string;
  'x-ui-hidden'?: boolean;
  additionalProperties?: JSONSchemaProperty;
  [key: string]: unknown;
}

export interface ApiResponse<T> {
  data: T;
  meta?: { total: number; limit: number; offset: number };
  error?: string;
}

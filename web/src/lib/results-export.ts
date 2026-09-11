import type { ProbeResult } from '../types/api';
import { guidexSortFields } from './guidex-timeline.ts';

// Export raw records independently of chart filtering and downsampling.
export async function loadResultExport(
  fetchResults: (params: string) => Promise<{ data?: ProbeResult[] | null }>,
  filters: string,
): Promise<ProbeResult[]> {
  const params = new URLSearchParams(filters);
  params.delete('points');
  params.set('limit', '5000');
  params.set('offset', '0');
  params.set('slim', '1');
  if (!params.get('to')) params.set('to', new Date().toISOString());
  const response = await fetchResults(params.toString());
  return (response.data ?? []).slice(0, 5000);
}

export function resultExportFields(rows: ProbeResult[], displayed: string[], runtimeV4: boolean): string[] {
  const fields = new Set(displayed);
  for (const row of rows) for (const [key, value] of Object.entries(row.extra ?? {})) {
    if (value == null || typeof value !== 'object') fields.add(key);
  }
  return runtimeV4 ? guidexSortFields([...fields]) : [...fields].sort();
}

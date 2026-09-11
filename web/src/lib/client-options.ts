import type { ProbeMetadata, ProbeResult, Task } from '../types/api';

type TaskIdentity = Pick<Task, 'id' | 'probe_type'>;
type ResultIdentity = Pick<ProbeResult, 'task_id' | 'agent_id'>;
type ProbeIdentity = Pick<ProbeMetadata, 'name' | 'kind' | 'output_schema'>;

const interactionProbes = new Set(['guidex-interaction', 'guidex-runtime-v4']);

export function clientOptions(
  agentIds: readonly string[],
  taskId: string,
  tasks: readonly TaskIdentity[],
  latestResults: readonly ResultIdentity[],
  probes: readonly ProbeIdentity[],
) {
  const taskProbes = new Map(tasks.map(task => [task.id, task.probe_type]));
  const probeForTask = (id: string) => taskProbes.get(id) || (id.startsWith('ext_') ? id.slice(4) : '');
  const selectedProbe = probeForTask(taskId);
  const isInteraction = interactionProbes.has(selectedProbe);

  // GuideX business probes use fixed names. The extension's configured Probe
  // Name belongs to its separate WebRTC probe, associated by the same agent ID.
  const browserProbes = new Set(probes.filter(probe => {
    const fields = new Set(probe.output_schema?.extra_fields?.map(field => field.name));
    return probe.kind === 'external' && !interactionProbes.has(probe.name) &&
      ['connection_count', 'audio_jitter', 'page_url'].every(field => fields.has(field));
  }).map(probe => probe.name));
  const namesByAgent = new Map<string, Set<string>>();
  if (isInteraction) {
    for (const row of latestResults) {
      const name = probeForTask(row.task_id);
      if (!browserProbes.has(name)) continue;
      const names = namesByAgent.get(row.agent_id) ?? new Set<string>();
      names.add(name);
      namesByAgent.set(row.agent_id, names);
    }
  }

  return [...new Set(agentIds)].map(agentId => {
    const names = isInteraction ? [...(namesByAgent.get(agentId) ?? [])].sort()
      : selectedProbe ? [selectedProbe] : [];
    const label = `${agentId} + ${names.length ? names.join(' / ') : 'ProbeName 未知'}`;
    const detail = isInteraction
      ? names.length ? '名称来自相同 Agent ID 的 WebRTC 上报；多个名称全部列出，不代表当前配置。'
        : '尚无相同 Agent ID 的 WebRTC 上报可关联插件 Probe Name。'
      : '名称为当前任务的探针名称。';
    return { value: agentId, label, title: `${label}\n${detail} 筛选值仍为 Agent ID。` };
  });
}

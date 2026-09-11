import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clientOptions } from '../src/lib/client-options.ts';

const browserProbe = name => ({ name, kind: 'external', output_schema: {
  standard_fields: ['latency_ms'],
  extra_fields: ['connection_count', 'audio_jitter', 'page_url'].map(name => ({ name, type: 'number' })),
} });
const row = (name, agent = 'browser-a') => ({ task_id: `ext_${name}`, agent_id: agent });

test('GuideX v4 and Legacy clients show configured WebRTC Probe Name, not the shared adapter', () => {
  for (const taskId of ['ext_guidex-runtime-v4', 'ext_guidex-interaction']) {
    const [client] = clientOptions(['browser-a'], taskId, [],
      [row('GuideX Macbook'), row('guidex-runtime-v4'), row('guidex-interaction')], [browserProbe('GuideX Macbook')]);
    assert.equal(client.label, 'browser-a + GuideX Macbook');
    assert.equal(client.value, 'browser-a');
    assert.match(client.title, /WebRTC/);
  }
});

test('client names never leak across agent IDs', () => {
  const options = clientOptions(['browser-a', 'browser-b', 'browser-c'], 'ext_guidex-runtime-v4', [],
    [row('Macbook'), row('Office', 'browser-b')], [browserProbe('Macbook'), browserProbe('Office')]);
  assert.deepEqual(options.map(o => o.label), ['browser-a + Macbook', 'browser-b + Office', 'browser-c + ProbeName 未知']);
});

test('multiple observed names are sorted and deduplicated without duplicating clients', () => {
  const options = clientOptions(['browser-a', 'browser-a'], 'ext_guidex-interaction', [],
    [row('West'), row('East'), row('West')], [browserProbe('East'), browserProbe('West')]);
  assert.equal(options.length, 1);
  assert.equal(options[0].label, 'browser-a + East / West');
  assert.equal(options[0].value, 'browser-a');
  assert.match(options[0].title, /不代表当前配置/);
});

test('unrelated, missing and non-external schemas cannot masquerade as client names', () => {
  const options = clientOptions(['browser-a'], 'ext_guidex-runtime-v4', [],
    [row('HTTP'), row('Missing'), row('Partial'), row('Built-in'), row('guidex-interaction')],
    [{ name: 'HTTP', kind: 'external' }, { name: 'Partial', kind: 'external', output_schema: { extra_fields: [{ name: 'page_url' }] } },
      { ...browserProbe('Built-in'), kind: 'builtin' }, browserProbe('guidex-interaction')]);
  assert.equal(options[0].label, 'browser-a + ProbeName 未知');
  assert.match(options[0].title, /尚无/);
});

test('WebRTC and other external tasks use the selected probe instead of all client aliases', () => {
  const [client] = clientOptions(['browser-a'], 'ext_GuideX Macbook', [],
    [row('Office')], [browserProbe('Office')]);
  assert.equal(client.label, 'browser-a + GuideX Macbook');
  assert.equal(client.value, 'browser-a');
});

test('task probe_type supports native and explicit external task IDs', () => {
  const tasks = [{ id: 'ping-task', probe_type: 'icmp' },
    { id: 'interaction-task', probe_type: 'guidex-runtime-v4' },
    { id: 'browser-task', probe_type: 'Macbook' }];
  assert.equal(clientOptions(['node-a'], 'ping-task', tasks, [], [])[0].label, 'node-a + icmp');
  assert.equal(clientOptions(['browser-a'], 'interaction-task', tasks,
    [{ task_id: 'browser-task', agent_id: 'browser-a' }], [browserProbe('Macbook')])[0].label, 'browser-a + Macbook');
});

test('empty names remain unknown; exact IDs and names are not rewritten', () => {
  assert.deepEqual(clientOptions([], '', [], [], []), []);
  assert.equal(clientOptions(['browser-a'], '', [], [], [])[0].label, 'browser-a + ProbeName 未知');
  const [client] = clientOptions(['agent + one'], 'ext_ext_会议室 + A', [], [], []);
  assert.equal(client.label, 'agent + one + ext_会议室 + A');
  assert.equal(client.value, 'agent + one');
});

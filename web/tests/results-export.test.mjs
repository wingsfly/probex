import { test } from 'node:test';
import assert from 'node:assert/strict';
import { loadResultExport, resultExportFields } from '../src/lib/results-export.ts';

test('export fetches raw rows including normal session closures, independently of charts and pagination', async () => {
  const raw = [{ id: 'leave', success: true, extra: { completion_reason: 'session_ended', observed: 17295 } },
    { id: 'failed', success: false, extra: { completion_reason: 'timeout' } }];
  const rows = await loadResultExport(async query => {
    const params = new URLSearchParams(query);
    assert.equal(params.get('task_id'), 'ext_guidex-runtime-v4');
    assert.equal(params.get('agent_id'), 'agent');
    assert.equal(params.get('node_id'), 'page');
    assert.equal(params.get('from'), '2026-09-11T00:00:00Z');
    assert.equal(params.get('to'), '2026-09-11T08:00:00Z');
    assert.equal(params.get('limit'), '5000');
    assert.equal(params.get('offset'), '0');
    assert.equal(params.has('points'), false);
    return { data: raw };
  }, 'task_id=ext_guidex-runtime-v4&agent_id=agent&node_id=page&from=2026-09-11T00:00:00Z&to=2026-09-11T08:00:00Z&offset=100&limit=100&points=800');
  assert.deepEqual(rows, raw);
});

test('export fixes the open-ended range at click time, bounds rows, and propagates read failures', async () => {
  const before = Date.now();
  const rows = await loadResultExport(async query => {
    assert.ok(Date.parse(new URLSearchParams(query).get('to')) >= before);
    return { data: Array.from({ length: 5001 }, (_, id) => ({ id })) };
  }, '');
  assert.equal(rows.length, 5000);
  assert.deepEqual(await loadResultExport(async () => ({ data: null }), ''), []);
  await assert.rejects(loadResultExport(async () => { throw new Error('read failed'); }, ''), /read failed/);
});

test('export discovers raw scalar columns even if chart data and the current table page lack them', () => {
  const rows = [{ extra: { completion_reason: 'session_ended', success: true,
    observed: 73, stt_text: '', reply: null, upload: 0, nested: {}, intervals: [], start: 0 } }];
  const keys = resultExportFields(rows, ['mic_ready'], true);
  for (const key of ['completion_reason', 'success', 'observed', 'stt_text', 'reply', 'upload', 'mic_ready'])
    assert.ok(keys.includes(key));
  for (const key of ['nested', 'intervals', 'start']) assert.equal(keys.includes(key), false);
  assert.ok(resultExportFields([{ extra: { old_metric: 1 } }], [], false).includes('old_metric'));
});

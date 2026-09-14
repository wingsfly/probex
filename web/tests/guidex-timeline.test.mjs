import { test } from 'node:test';
import assert from 'node:assert/strict';
import { GUIDEX_CHART_FIELDS, GUIDEX_DURATION_KEYS, GUIDEX_LABELS, GUIDEX_STAGES, guidexChartAxis, guidexTimeline, guidexChartTimeline, guidexChartFields, guidexSource, guidexStartBy,
  guidexStatus, guidexSortFields, guidexFieldOrder, guidexTimingEligible } from '../src/lib/guidex-timeline.ts';

const names = ['Mic_Ready', '1st_Audio', 'Speech_Started', '1st_STT', 'Last_Audio', 'Speech_Stopped',
  'Last_STT', '1st_Answer', 'Last_Answer', '1st_Play', 'Last_Play', 'Trig_Interrupt', 'Interrupted', 'End'];

test('normal session closure is Done in details but excluded from timing without rewriting history', () => {
  const row = Object.freeze({ task_id: 'ext_guidex-runtime-v4', success: true,
    extra: Object.freeze({ completion_reason: 'session_ended', observed: 17295, end: 17295, upload: 0 }) });
  assert.equal(guidexStatus(row.success, row.extra).label, 'Done');
  assert.equal(guidexTimingEligible(row), false);
  assert.equal(guidexTimingEligible({ ...row, task_id: 'custom',
    extra: { ...row.extra, client_adapter: 'guidex-runtime-v4' } }), false);
  for (const candidate of [
    { ...row, success: false }, { ...row, task_id: 'ext_guidex-interaction' },
    { ...row, error: 'explicit error' }, { ...row, extra: {} },
    ...['completed', 'interrupted', 'error', 'timeout', 'disconnected'].map(reason =>
      ({ ...row, extra: { completion_reason: reason } })),
  ]) assert.equal(guidexTimingEligible(candidate), true);
  assert.equal(guidexStatus(false, row.extra).label, 'Failed');
  assert.equal(row.extra.observed, 17295);
  assert.equal(guidexTimeline(row.extra).find(item => item.key === 'end').ms, 17295);
});

test('14 English milestones preserve zero, missing values and interleaving', () => {
  const stages = guidexTimeline({ start_by: 'press', mic_ready: 110, upload: 100,
    asr: 150, input_end: 140, asr_end: null, end: 500 });
  assert.deepEqual(stages.map(s => s.label), names);
  assert.equal(stages[0].ms, 110); assert.equal(stages[1].ms, 100);
  assert.equal(stages[2].ms, null);
  assert.equal(guidexTimeline({ mic_ready: 0 })[0].ms, 0);
});

test('only finite numeric milestones are accepted without changing source data', () => {
  const extra = Object.freeze({ upload: '100', mic_ready: NaN, speech: -Infinity,
    input_end: -10, asr_end: null, end: 500 });
  const t = Object.fromEntries(guidexTimeline(extra).map(s => [s.key, s.ms]));
  assert.equal(t.end, 500); assert.equal(t.input_end, -10);
  for (const key of ['upload', 'mic_ready', 'speech', 'asr_end']) assert.equal(t[key], null);
  assert.equal(guidexTimeline({ mic_ready: Infinity })[0].ms, null);
  assert.equal(extra.upload, '100');
});

test('chart keeps early milestones and replaces late cumulative nodes with intervals', () => {
  const fields = guidexChartFields([
    'mic_ready', 'upload', 'speech', 'asr',
    'upload_end', 'input_end', 'asr_end', 'reply', 'reply_end',
    'speak', 'speak_end', 'interrupt', 'interrupted', 'end',
    'audio_start_to_speech_started', 'last_audio_to_final_asr',
    'last_audio_to_first_answer', 'final_asr_to_first_answer',
    'answer_stream', 'last_audio_to_avatar_start',
    'avatar_speak_duration', 'interrupt_ack',
  ]);
  assert.deepEqual(fields, [
    'mic_ready', 'upload', 'speech', 'asr',
    'last_audio_to_final_asr',
    'last_audio_to_first_answer', 'final_asr_to_first_answer',
    'answer_stream', 'last_audio_to_avatar_start',
    'avatar_speak_duration', 'interrupt_ack',
  ]);
  const chart = guidexChartTimeline({
    upload: 0, asr: 150, upload_end: 2200, input_end: 2230,
    audio_start_to_speech_started: 40, last_audio_to_final_asr: 20,
    final_asr_to_first_answer: 15, answer_stream: 300,
  });
  assert.deepEqual(chart.map(item => item.key), GUIDEX_CHART_FIELDS.map(([key]) => key));
  assert.deepEqual(chart.map(item => item.label), [
    'Mic_Ready', '1st_Audio', 'Speech_Started', '1st_STT',
    'LastAudio_To_STT', 'LastAudio_To_Answer', 'STT_To_Answer', 'Answer_Dur',
    'LastAudio_To_Play', 'Play_Dur', 'Interrupt_ACK',
  ]);
  assert.equal(chart.some(item => ['upload_end', 'input_end', 'asr_end', 'reply', 'reply_end',
    'speak', 'speak_end', 'interrupt', 'interrupted', 'end'].includes(item.key)), false);
  assert.equal(chart.some(item => item.key === 'audio_start_to_speech_started'), false);
  assert.equal(chart.find(item => item.key === 'last_audio_to_final_asr').ms, 20);
});

test('duration and response metrics share a chart without sharing a numeric axis', () => {
  assert.deepEqual([...GUIDEX_DURATION_KEYS], ['answer_stream', 'avatar_speak_duration']);
  for (const key of ['answer_stream', 'avatar_speak_duration']) {
    assert.equal(guidexChartAxis(key), 'duration');
  }
  for (const key of ['upload', 'asr', 'last_audio_to_final_asr',
    'last_audio_to_first_answer', 'final_asr_to_first_answer',
    'last_audio_to_avatar_start', 'interrupt_ack']) {
    assert.equal(guidexChartAxis(key), 'response');
  }
});

test('retired audio-to-speech stays hidden while speech and renamed durations keep their values', () => {
  const extra = Object.freeze({ audio_start_to_speech_started: 40, speech: 90,
    answer_stream: 0, avatar_speak_duration: 275 });
  assert.deepEqual(guidexSortFields(Object.keys(extra)), ['speech', 'answer_stream', 'avatar_speak_duration']);
  assert.deepEqual(guidexChartFields(Object.keys(extra)), ['speech', 'answer_stream', 'avatar_speak_duration']);
  const chart = guidexChartTimeline(extra);
  assert.equal(chart.length, 11);
  assert.equal(chart.find(item => item.key === 'speech').ms, 90);
  assert.deepEqual(chart.filter(item => ['answer_stream', 'avatar_speak_duration'].includes(item.key)), [
    { key: 'answer_stream', label: 'Answer_Dur', ms: 0 },
    { key: 'avatar_speak_duration', label: 'Play_Dur', ms: 275 },
  ]);
  for (const item of guidexChartTimeline({ answer_stream: null, avatar_speak_duration: null })) {
    assert.equal(item.ms, null);
  }
  assert.equal(Object.hasOwn(GUIDEX_LABELS, 'audio_start_to_speech_started'), false);
  for (const label of ['Audio_To_Speech', '1st_Answer_To_Last_Answer', '1st_Play_To_Last_Play']) {
    assert.equal(Object.values(GUIDEX_LABELS).includes(label), false);
  }
});

test('last-audio intervals preserve reported deltas and never convert old metrics or infer anchors', () => {
  const retired = ['speech_stop_to_final_asr', 'speech_stop_to_first_answer',
    'speech_stop_to_avatar_start', 'test_audio_to_avatar_start'];
  const keys = ['last_audio_to_final_asr', 'last_audio_to_first_answer', 'last_audio_to_avatar_start'];
  const old = Object.fromEntries(retired.map(key => [key, 100]));
  const extra = Object.freeze({ ...old, upload_end: 100, input_end: 200, asr_end: 300, reply: 400, speak: 500 });
  assert.deepEqual(guidexChartFields(retired), []);
  assert.deepEqual(guidexSortFields(retired), []);
  for (const key of retired) assert.equal(Object.hasOwn(GUIDEX_LABELS, key), false);
  const values = row => guidexChartTimeline(row).filter(item => keys.includes(item.key)).map(item => item.ms);
  assert.deepEqual(values(extra), [null, null, null]);
  assert.deepEqual(values({ ...extra, last_audio_to_final_asr: 0,
    last_audio_to_first_answer: -10, last_audio_to_avatar_start: 25 }), [0, -10, 25]);
  assert.deepEqual(values({ last_audio_to_final_asr: '10', last_audio_to_first_answer: Infinity,
    last_audio_to_avatar_start: NaN }), [null, null, null]);
});

test('missing readiness and other milestones are never inferred from retired fields or durations', () => {
  const stages = guidexTimeline({ start_by: 'first_input', start_at: '2026-09-10T02:00:00Z',
    start: 0, mic_request: 100, mic: 200, ready: 300,
    first_answer: 19278, last_audio_to_first_answer: 937, last_audio_to_avatar_start: 3829,
    final_asr_to_first_answer: 892, avatar_speak_duration: 11683, observed: 33853 });
  assert.ok(stages.every(stage => stage.ms === null));
  assert.ok(guidexTimeline({}).every(stage => stage.ms === null));
  assert.deepEqual(guidexSortFields(['start', 'mic_request', 'mic', 'ready']), []);
});

test('interruption is completion but never a new turn start or fabricated playback end', () => {
  const extra = { start_by: 'press', completion_reason: 'interrupted',
    interrupt: 1000, interrupted: 1200, end: 1200 };
  assert.equal(guidexStatus(true, extra).label, 'Interrupted');
  assert.equal(guidexStatus(false, extra).label, 'Failed');
  assert.equal(guidexStatus(true, { completion_reason: 'completed' }).label, 'Done');
  assert.equal(guidexStatus(false, { completion_reason: 'timeout' }).label, 'Failed');
  const t = Object.fromEntries(guidexTimeline(extra).map(s => [s.key, s.ms]));
  assert.equal(t.interrupt, 1000); assert.equal(t.interrupted, 1200); assert.equal(t.speak_end, null);
  assert.equal(extra.start_by, 'press');
});

test('confirmation without a request does not invent request or playback endpoints', () => {
  for (const extra of [{ completion_reason: 'interrupted', interrupted: 1200, end: 1200 },
    { completion_reason: 'interrupted', end: 1200 }]) {
    const t = Object.fromEntries(guidexTimeline(extra).map(s => [s.key, s.ms]));
    assert.equal(t.interrupt, null); assert.equal(t.speak_end, null); assert.equal(Object.hasOwn(t, 'start'), false);
    assert.equal(t.interrupted, extra.interrupted ?? null);
  }
});

test('obsolete field aliases are not converted into current milestones', () => {
  const extra = { timeline_origin: 'user_press', press_at: '2026-09-10T02:00:00Z', press_ms: 0,
    mic_ms: 234, upload_ms: 374, observed_ms: 33853, end_ms: 33853 };
  assert.ok(guidexTimeline(extra).every(stage => stage.ms === null));
  const t = Object.fromEntries(guidexTimeline({ ...extra, upload: 0, mic_ready: null }).map(s => [s.key, s.ms]));
  assert.equal(t.upload, 0); assert.equal(t.mic_ready, null);
  assert.deepEqual(guidexChartFields(Object.keys(extra)), []);
});

test('default charts and grouped table ordering separate metadata, milestones and units', () => {
  assert.deepEqual(guidexChartFields(['end', 'audio_bytes', 'mic_ready', 'reply', 'answer_stream', 'end', 'start']),
    ['mic_ready', 'answer_stream']);
  assert.deepEqual(guidexChartFields(['interrupt_ack', 'interrupted', 'interrupt']), ['interrupt_ack']);
  assert.deepEqual(guidexSortFields(['end', 'start_by', 'interrupt_at', 'start_at', 'audio_bytes', 'mic_ready',
    'stt_text', 'model_complete', 'input_ended']),
    ['start_at', 'start_by', 'mic_ready', 'stt_text', 'interrupt_at', 'end']);
  assert.equal(GUIDEX_LABELS.stt_text, 'STT Text');
  // Late intervals must stay beside their business stage after removing numeric labels.
  const shuffled = ['observed', 'avatar_speak_duration', 'asr', 'audio_start_to_speech_started',
    'stt_revocations', 'reply', 'test_audio_duration', 'asr_end', 'last_audio_to_first_answer',
    'reply_end', 'end', 'stt_text', 'speech', 'last_audio_to_final_asr', 'unknown_z', 'unknown_a'];
  const ordered = ['speech', 'asr', 'test_audio_duration',
    'asr_end', 'stt_text', 'stt_revocations', 'last_audio_to_final_asr', 'reply',
    'last_audio_to_first_answer', 'reply_end', 'avatar_speak_duration', 'end', 'observed',
    'unknown_a', 'unknown_z'];
  assert.deepEqual(guidexSortFields(shuffled), ordered);
  assert.deepEqual(ordered.slice(0, -2).toReversed().sort((a, b) => guidexFieldOrder(a) - guidexFieldOrder(b)),
    ordered.slice(0, -2));
  for (const key of ['audio_bytes', 'audio_frames', 'audio_upload_window', 'audio_start_to_first_asr',
    'click_to_first_audio', 'input_ended', 'asr_final', 'asr_recognized', 'asr_characters',
    'model_completed', 'model_complete', 'audio_start_to_speech_started'])
    assert.equal(Object.hasOwn(GUIDEX_LABELS, key), false, key);
  assert.equal(guidexSource('multimodal_box'), 'Box');
  assert.equal(guidexSource('browser_mic'), 'Mic');
  assert.equal(guidexStartBy('multimodal_vad'), 'Box_VAD');
  assert.equal(guidexStartBy('first_input'), '1st_Input');
  const labels = [...Object.values(GUIDEX_LABELS), guidexSource(null), guidexStartBy(null), guidexSource('not_applicable')];
  for (const label of labels) {
    assert.match(label, /^[\x20-\x7e]+$/);
    assert.doesNotMatch(label, /^\d+\.\s/);
    if (!['Answer_Dur', 'Play_Dur'].includes(label)) assert.doesNotMatch(label, /_Dur\b/);
  }
  assert.equal(GUIDEX_STAGES.length, 14);
});

// Current Runtime v4 fields only; missing milestones are never reconstructed.
export const GUIDEX_STAGES = [
  ['mic_ready', 'Mic_Ready'], ['upload', '1st_Audio'], ['speech', 'Speech_Started'],
  ['asr', '1st_STT'], ['upload_end', 'Last_Audio'], ['input_end', 'Speech_Stopped'],
  ['asr_end', 'Last_STT'], ['reply', '1st_Answer'], ['reply_end', 'Last_Answer'],
  ['speak', '1st_Play'], ['speak_end', 'Last_Play'], ['interrupt', 'Trig_Interrupt'],
  ['interrupted', 'Interrupted'], ['end', 'End'],
] as const;

// Keep early setup/input milestones, but use interval metrics for response
// phases so a longer user utterance does not shift every later line.
export const GUIDEX_INTERVAL_FIELDS = [
  ['last_audio_to_final_asr', 'LastAudio_To_STT'],
  ['last_audio_to_first_answer', 'LastAudio_To_Answer'],
  ['final_asr_to_first_answer', 'STT_To_Answer'],
  ['answer_stream', 'Answer_Dur'],
  ['last_audio_to_avatar_start', 'LastAudio_To_Play'],
  ['avatar_speak_duration', 'Play_Dur'],
  ['interrupt_ack', 'Interrupt_ACK'],
] as const;

export const GUIDEX_CHART_FIELDS = [
  ...GUIDEX_STAGES.slice(0, 4),
  ...GUIDEX_INTERVAL_FIELDS,
] as const;

export const GUIDEX_CHART_KEYS = new Set<string>(GUIDEX_CHART_FIELDS.map(([key]) => key));

export const GUIDEX_LABELS: Record<string, string> = {
  ...Object.fromEntries(GUIDEX_STAGES),
  ...Object.fromEntries(GUIDEX_INTERVAL_FIELDS),
  start_at: 'Start_At', start_by: 'Start_By', press_kind: 'Press_Kind', input_source: 'Source',
  stt_text: 'STT Text', stt_revocations: 'STT_Rev (count)',
  answer_characters: 'Answer (chars)', avatar_ended: 'Play_Done',
  interrupt_at: 'Trig_At', interrupted_at: 'Interrupted_At',
  success: 'OK', completion_reason: 'End_By', error_code: 'Err_Code',
  total_interaction: 'Audio_To_End', observed: 'Input_To_Observed_End', cid_end: 'CID_End',
  first_answer: 'Audio_To_Answer', test_audio_duration: 'Test_Audio_Start_To_End',
  cycle: 'Cycle (count)', client_adapter: 'Adapter', instance_id: 'Instance', sid: 'SID', cid: 'CID',
  input_type: 'Input', interaction_mode: 'Mode', page_url: 'Page',
};

// Business order is independent of display names and observed arrival times.
const GUIDEX_FIELD_ORDER = new Map([
  'start_at', 'start_by', 'press_kind', 'input_source',
  'mic_ready', 'upload', 'speech', 'asr',
  'upload_end', 'test_audio_duration', 'input_end',
  'asr_end', 'stt_text', 'stt_revocations', 'last_audio_to_final_asr',
  'reply', 'last_audio_to_first_answer', 'final_asr_to_first_answer', 'first_answer',
  'reply_end', 'answer_characters', 'answer_stream',
  'speak', 'last_audio_to_avatar_start',
  'speak_end', 'avatar_ended', 'avatar_speak_duration',
  'interrupt', 'interrupt_at', 'interrupted', 'interrupted_at', 'interrupt_ack',
  'end', 'success', 'completion_reason', 'error_code', 'total_interaction', 'observed', 'cid_end',
  'cycle', 'client_adapter', 'instance_id', 'sid', 'cid', 'input_type', 'interaction_mode', 'page_url',
].map((key, index) => [key, index]));

export function guidexFieldOrder(key: string): number {
  return GUIDEX_FIELD_ORDER.get(key) ?? Number.MAX_SAFE_INTEGER;
}

const GUIDEX_REMOVED_FIELDS = new Set([
  'audio_bytes', 'audio_frames', 'audio_upload_window', 'audio_start_to_first_asr',
  'click_to_first_audio', 'input_ended', 'asr_final', 'asr_recognized', 'asr_characters',
  'model_completed', 'model_complete',
  'audio_start_to_speech_started',
  'speech_stop_to_final_asr', 'speech_stop_to_first_answer', 'speech_stop_to_avatar_start',
  'test_audio_to_avatar_start',
]);

export function guidexStartBy(value: unknown): string {
  return ({ press: 'Press', multimodal_vad: 'Box_VAD', interrupt: 'Interrupt',
    auto_test: 'Auto', first_input: '1st_Input' } as Record<string, string>)[String(value)] ?? 'N/A';
}

export function guidexStatus(success: boolean, extra: Record<string, unknown>) {
  if (success && extra.completion_reason === 'interrupted') return {
    label: 'Interrupted', color: '#a16207',
  };
  return { label: success ? 'Done' : 'Failed', color: success ? '#15803d' : '#ef4444' };
}

// Use the reported outcome, never reinterpret historical Failed/session_ended rows.
export function guidexTimingEligible(row: {
  task_id: string; success: boolean; error?: string; extra?: Record<string, unknown>;
}): boolean {
  const runtime = row.task_id === 'ext_guidex-runtime-v4' || row.extra?.client_adapter === 'guidex-runtime-v4';
  return !(runtime && row.success === true && !row.error && row.extra?.completion_reason === 'session_ended');
}

export function guidexSource(value: unknown): string {
  return ({ browser_mic: 'Mic', multimodal_box: 'Box',
    unknown: 'Unknown', not_applicable: 'N/A' } as Record<string, string>)[String(value)] ?? 'Unknown';
}

export function guidexTimeline(extra: Record<string, unknown>) {
  const number = (key: string) => typeof extra[key] === 'number' && Number.isFinite(extra[key]) ? extra[key] as number : null;
  return GUIDEX_STAGES.map(([key, label]) => ({ key, label, ms: number(key) }));
}

export function guidexChartTimeline(extra: Record<string, unknown>) {
  const number = (key: string) => typeof extra[key] === 'number' && Number.isFinite(extra[key]) ? extra[key] as number : null;
  return GUIDEX_CHART_FIELDS.map(([key, label]) => ({ key, label, ms: number(key) }));
}

export function guidexChartFields(keys: string[]): string[] {
  const present = new Set(keys);
  return GUIDEX_CHART_FIELDS.map(([key]) => key).filter(key => present.has(key));
}

export function guidexSortFields(keys: string[]): string[] {
  return [...new Set(keys)].filter(key => !GUIDEX_REMOVED_FIELDS.has(key) &&
    !['start', 'mic_request', 'mic', 'ready'].includes(key)).sort((a, b) =>
    guidexFieldOrder(a) - guidexFieldOrder(b) || a.localeCompare(b));
}

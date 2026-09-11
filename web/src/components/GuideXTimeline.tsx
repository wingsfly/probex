import { useState } from 'react';
import { BarChart, Bar, XAxis, YAxis, Tooltip, CartesianGrid, ResponsiveContainer, LabelList } from 'recharts';
import type { ProbeResult } from '../types/api';
import { guidexChartTimeline, guidexSource, guidexStartBy, guidexStatus, guidexTimingEligible } from '../lib/guidex-timeline';

export default function GuideXTimeline({ results }: { results: ProbeResult[] }) {
  const [selectedId, setSelectedId] = useState('');
  const timingResults = results.filter(guidexTimingEligible);
  const result = timingResults.find(row => row.id === selectedId) ?? timingResults[0];
  if (!result) return null;
  const extra = result.extra ?? {};
  const timings = guidexChartTimeline(extra);
  const wallTime = (value: unknown) => typeof value === 'string' && Number.isFinite(Date.parse(value))
    ? new Date(value).toLocaleString('en-GB', {
      year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3,
    }) : 'N/A';
  return (
    <section aria-label="GuideX Turn Timing" style={{ background: '#fff', border: '1px solid #e5e7eb',
      borderRadius: 8, padding: '1rem', marginTop: '1rem' }}>
      <h2 style={{ fontSize: '1rem', marginTop: 0 }}>Turn Timing</h2>
      <select aria-label="Select turn" value={result.id} onChange={event => setSelectedId(event.target.value)}
        style={{ maxWidth: '100%', padding: '0.5rem', border: '1px solid #d1d5db', borderRadius: 6 }}>
        {timingResults.map(row => <option key={row.id} value={row.id}>
          {new Date(row.timestamp).toLocaleString('en-GB')} · {guidexStatus(row.success, row.extra ?? {}).label} · {String(row.extra?.cid ?? row.id)}
        </option>)}
      </select>
      <p style={{ fontSize: '0.85rem', color: '#475569', lineHeight: 1.7 }}>
        Start_At: {wallTime(extra.start_at)} (T=0)
        <br />Start_By: {guidexStartBy(extra.start_by)} · Source: {guidexSource(extra.input_source)}
        {extra.start_by === 'press' && <> · Press_Kind: {String(extra.press_kind ?? 'N/A')}</>}
        {(extra.completion_reason === 'interrupted' || extra.interrupt != null) && <>
          <br />Trig_At: {wallTime(extra.interrupt_at)} · Interrupted_At: {wallTime(extra.interrupted_at)}
          {' · '}Interrupt_ACK: {typeof extra.interrupt_ack === 'number' ? extra.interrupt_ack : 'N/A'}
        </>}
        <br />End_By: {String(extra.completion_reason ?? 'Unknown')}
      </p>
      <ResponsiveContainer width="100%" height={470}>
        <BarChart data={timings} layout="vertical" margin={{ top: 4, right: 76, bottom: 18, left: 0 }}>
          <CartesianGrid strokeDasharray="3 3" horizontal={false} />
          <XAxis type="number" domain={[0, 'auto']} tick={{ fontSize: 11 }} tickCount={3}
            label={{ value: 'Elapsed (ms)', position: 'insideBottom', offset: -15, fontSize: 11 }} />
          <YAxis type="category" dataKey="label" width={180} interval={0} tick={{ fontSize: 11 }} />
          <Tooltip formatter={value => [value, 'Elapsed (ms)']} />
          <Bar dataKey="ms" name="Elapsed" fill="#0f766e" isAnimationActive={false}
            minPointSize={value => value === 0 ? 2 : 0}>
            <LabelList position="right" dataKey={entry => entry.ms == null ? '' : String(entry.ms)} style={{ fontSize: 11 }} />
          </Bar>
        </BarChart>
      </ResponsiveContainer>
      <p style={{ fontSize: '0.8rem', color: '#64748b', lineHeight: 1.6 }}>
        N/A: {timings.filter(timing => timing.ms == null).map(timing => timing.label).join(', ') || 'None'}.
        {' '}Mic_Ready 表示从起点到 H5 收音界面就绪的累计时间，包含前面的准备，不是旧偏移相加；未观测到就绪时不推算。
        图表保留 Mic_Ready、1st_Audio、Speech_Started、1st_STT，后续改用关键区间耗时，避免用户说话时长把后续累计时间整体推后。
        LastAudio_To_STT、LastAudio_To_Answer、LastAudio_To_Play 均从本轮最后一次成功音频上传起算，分别到最终识别、首次回答、首次播报通知。
        自动测试也使用同一口径；LastAudio 不等于物理停止说话、服务端语音结束通知或测试样本播放结束。
        Answer_Dur 表示首次回答到模型终态；1st_Play 是业务通知，不代表扬声器实际出声，Play_Dur 表示业务播放事件之间的区间。
        Trig_Interrupt 和 Interrupted 归属于被打断的旧轮次；Interrupt_ACK 表示请求到确认的区间。
        名称不带序号，按业务顺序显示，不强制流式事件先后；_To_ 连接计时起止点，Answer_Dur、Play_Dur 使用时长短名；数值默认毫秒，绝对时间单独显示。
        仅使用当前表格页的原始记录，不使用聚合均值。
      </p>
    </section>
  );
}

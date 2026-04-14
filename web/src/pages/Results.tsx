import { useState, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { ProbeResult, Task } from '../types/api';
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
} from 'recharts';
// Short display names for columns (maps raw field name → [short name, description])
const FIELD_DICT: Record<string, [string, string]> = {
  // WebRTC 标准指标
  'Latency': ['Latency', '端到端往返时延(ms)。数据源: remote-inbound-rtp.roundTripTime × 1000，回退: candidate-pair.currentRoundTripTime × 1000'],
  'Jitter': ['Jitter', '音频RTP到达间隔抖动(ms)。数据源: inbound-rtp(audio).jitter × 1000'],
  'Loss%': ['Loss%', '丢包率(%)。计算: (packetsLost增量) / (packetsReceived增量 + packetsLost增量) × 100'],
  'Download': ['Download', '下行码率(Mbps)。计算: inbound-rtp的bytesReceived增量 × 8 / 采样时间间隔'],
  'Upload': ['Upload', '上行码率(Mbps)。计算: outbound-rtp的bytesSent增量 × 8 / 采样时间间隔'],
  'DNS': ['DNS', 'DNS解析耗时(ms)'],
  'TLS': ['TLS', 'TLS握手耗时(ms)'],
  // Guidex 交互拨测指标
  'success': ['OK?', 'ASR是否识别成功。判定: voiceDictation WS收到code:0的响应中ws[].cw[].w有非空词即为true'],
  'asr_text': ['ASR Text', 'ASR识别文本。数据源: voiceDictation WS recv中code:0消息的ws[].cw[].w拼接。优先取最终结果(status:2, ls:true)，为空则取首次识别文本'],
  'audio_duration_ms': ['Audio Len', '测试音频时长(ms)。数据源: audioBuffer.duration × 1000，每次测试固定值'],
  'click_to_vd_ready_ms': ['Click→VD', '点击按钮到VD会话就绪(ms)。计算: tVdReady - tClick。tVdReady = 新voiceDictation WS创建且status:0消息已发出的时刻'],
  'audio_start_to_first_asr_ms': ['1st ASR', '首字延迟(ms)。计算: firstAsrTime - tAudioStart。firstAsrTime = voiceDictation WS首次收到含非空词的code:0结果的时刻'],
  'audio_end_to_final_asr_ms': ['ASR Tail', '说完话到ASR最终结果(ms)。计算: finalAsrTime - tAudioEnd。finalAsrTime = voiceDictation WS收到status:2且ls:true的时刻'],
  'audio_end_to_tts_ms': ['Wait TTS', '说完话到TTS合成开始(ms)。计算: ttsStartTime - tAudioEnd。ttsStartTime = interact WS在finalAsrTime之后首次收到tts_duration事件的时刻'],
  'tts_to_avatar_speak_ms': ['TTS→Lip', 'TTS事件到嘴巴开始动(ms)。计算: firstVmr1Time - ttsStartTime。firstVmr1Time = interact WS首次收到vmr_status=1的时刻'],
  'avatar_speak_duration_ms': ['Avatar Dur', '数字人说话挂钟时间(ms)。计算: avatarSpeakEnd - avatarSpeakStart。起点=首个vmr=0/1，终点=最后一个vmr=2。包含多段TTS之间的等待间隔'],
  'tts_total_duration_ms': ['TTS Len', 'TTS合成音频总时长(ms)。计算: 所有interact WS tts_duration值的累加。注意: 这是原始合成时长，实际播放约为此值的2/3（约1.5倍速播放）'],
  'lip_move_ms': ['Lip Move', '嘴巴实际动的累计时长(ms)。计算: 每段(vmr=2时刻 - 该段首个vmr=0/1时刻)之和。不含段间等待。对比Avatar Dur可看段间间隔'],
  'lip_sync_diff_ms': ['Lip Sync', '唇形同步偏差(ms)。计算: actual_audio_duration_ms - lip_move_ms。正值=客户端听到的声音比嘴动的时间长（嘴停后音频仍在缓冲播放）'],
  'audio_end_to_playback_ms': ['Wait Play', '说完话到听到回复(ms)。计算: actualAudioStart - tAudioEnd。actualAudioStart = AnalyserNode检测到RMS能量>阈值的时刻，仅在finalAsrTime设置后才开始检测'],
  'actual_audio_duration_ms': ['Play Dur', '客户端实际听到声音的时长(ms)。计算: actualAudioEnd - actualAudioStart。通过AnalyserNode每50ms轮询RMS能量，阈值=10，检测有声→无声的时间差'],
  'vmr_to_actual_audio_ms': ['Lip→Play', '嘴巴开始动到客户端听到声音(ms)。计算: actualAudioStart - firstVmr1Time。反映WebRTC推流 + jitter buffer + 音频解码的传输延迟'],
  'total_interaction_ms': ['Total', '端到端总耗时(ms)。计算: 终点 - tClick。终点优先级: actualAudioEnd(客户端声音停止) > avatarSpeakEnd(数字人说完) > ttsStartTime(TTS开始) > finalAsrTime(ASR结束)'],
  'cycle': ['Cycle', '自动测试轮次序号'],
  'page_url': ['Page', '测试页面URL (location.href)'],
  // WebRTC 持续监测指标
  'audio_jitter': ['A.Jitter', '音频RTP抖动(ms)。数据源: inbound-rtp(audio).jitter × 1000'],
  'video_jitter': ['V.Jitter', '视频RTP抖动(ms)。数据源: inbound-rtp(video).jitter × 1000'],
  'video_frames_decoded': ['V.Decoded', '视频解码帧数。计算: inbound-rtp(video).framesDecoded的两次采样增量'],
  'video_frames_dropped': ['V.Dropped', '视频丢弃帧数。计算: inbound-rtp(video).framesDropped的两次采样增量'],
  'video_fps': ['V.FPS', '视频帧率。数据源: inbound-rtp(video).framesPerSecond'],
  'quality_limitation': ['Q.Limit', '编码质量受限原因。数据源: outbound-rtp(video).qualityLimitationReason (cpu/bandwidth/none)'],
  'available_outgoing_bitrate': ['Out BW', '可用上行带宽(bps)。数据源: candidate-pair.availableOutgoingBitrate'],
  'audio_jb_delay_ms': ['A.JB', '音频jitter buffer延迟(ms)。计算: (jitterBufferDelay增量 / emittedCount增量) × 1000。每500ms子采样，2s聚合取最新值'],
  'video_jb_delay_ms': ['V.JB', '视频jitter buffer延迟(ms)。计算同A.JB，取video inbound-rtp数据'],
  'av_sync_diff_ms': ['AV Sync', '音视频同步偏差(ms)。计算: 最新videoJB - 最新audioJB（跨PC聚合，因Guidex音频和视频分别在不同PeerConnection上）。正值=视频比音频延迟更大'],
  'connection_count': ['Conns', '活跃PeerConnection数量'],
};
const getShortName = (raw: string) => FIELD_DICT[raw]?.[0] ?? raw;

// sheetjs-style is loaded dynamically on export to avoid blocking page render

const TASK_COLORS = ['#3b82f6', '#ef4444', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16'];
const EXTRA_COLORS = ['#3b82f6', '#ef4444', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16', '#78716c', '#9333ea'];

// Standard fields that can appear in results
const STANDARD_FIELDS: { key: keyof ProbeResult; label: string; unit: string; fmt: (v: any) => string }[] = [
  { key: 'latency_ms', label: 'Latency', unit: 'ms', fmt: (v: number) => `${v.toFixed(1)}ms` },
  { key: 'jitter_ms', label: 'Jitter', unit: 'ms', fmt: (v: number) => `${v.toFixed(2)}ms` },
  { key: 'packet_loss_pct', label: 'Loss%', unit: '%', fmt: (v: number) => `${v.toFixed(2)}%` },
  { key: 'dns_resolve_ms', label: 'DNS', unit: 'ms', fmt: (v: number) => `${v.toFixed(1)}ms` },
  { key: 'tls_handshake_ms', label: 'TLS', unit: 'ms', fmt: (v: number) => `${v.toFixed(1)}ms` },
  { key: 'status_code', label: 'Code', unit: '', fmt: (v: number) => String(v) },
  { key: 'download_bps', label: 'Download', unit: 'Mbps', fmt: (v: number) => `${(v / 1e6).toFixed(2)} Mbps` },
  { key: 'upload_bps', label: 'Upload', unit: 'Mbps', fmt: (v: number) => `${(v / 1e6).toFixed(2)} Mbps` },
];

export default function Results() {
  const [taskId, setTaskId] = useState('');
  const [timeRange, setTimeRange] = useState('1h');
  const [customFrom, setCustomFrom] = useState('');
  const [customTo, setCustomTo] = useState('');
  const [hiddenLines, setHiddenLines] = useState<Set<string>>(new Set());

  const toggleLine = (dataKey: string) => {
    setHiddenLines(prev => {
      const next = new Set(prev);
      next.has(dataKey) ? next.delete(dataKey) : next.add(dataKey);
      return next;
    });
  };

  const isCustom = timeRange === 'custom';

  const fromTime = () => {
    if (isCustom && customFrom) return new Date(customFrom).toISOString();
    const now = new Date();
    const hours: Record<string, number> = { '1h': 1, '6h': 6, '24h': 24, '7d': 168 };
    now.setHours(now.getHours() - (hours[timeRange] ?? 1));
    return now.toISOString();
  };

  const toTime = () => {
    if (isCustom && customTo) return new Date(customTo).toISOString();
    return '';
  };

  // Scale limit by time range; larger ranges pull more data
  const limitByRange: Record<string, string> = { '1h': '2000', '6h': '5000', '24h': '15000', '7d': '50000', 'custom': '10000' };
  // Larger ranges: slower refresh (no point refreshing 7d data every 10s)
  const refreshByRange: Record<string, number | false> = { '1h': 10000, '6h': 30000, '24h': 60000, '7d': false, 'custom': false };
  const params = new URLSearchParams({ limit: limitByRange[timeRange] ?? '2000', from: fromTime() });
  const to = toTime();
  if (to) params.set('to', to);
  if (taskId) params.set('task_id', taskId);

  const { data, isLoading } = useQuery({
    queryKey: ['results', taskId, timeRange, customFrom, customTo],
    queryFn: () => api.getResults(params.toString()),
    refetchInterval: refreshByRange[timeRange] ?? 10000,
  });

  const { data: tasksData } = useQuery({
    queryKey: ['tasks'],
    queryFn: () => api.getTasks(),
  });

  // Fetch latest results (one per task) to discover ALL task_ids including external probes
  const { data: latestData } = useQuery({
    queryKey: ['latestResults'],
    queryFn: api.getLatestResults,
  });

  const resultsDesc: ProbeResult[] = data?.data ?? [];
  const resultsAsc: ProbeResult[] = [...resultsDesc].reverse();
  const tasks: Task[] = tasksData?.data ?? [];
  const latestResults: ProbeResult[] = latestData?.data ?? [];

  const taskMap = useMemo(() => {
    const m = new Map<string, Task>();
    tasks.forEach(t => m.set(t.id, t));
    return m;
  }, [tasks]);

  // --- Auto-detect which fields are present in the data ---
  // A field is "present" if at least one result has a non-null, non-zero value for it.
  // This avoids showing columns like latency_ms=0 when the probe doesn't produce that metric.
  const presentStdFields = useMemo(() =>
    STANDARD_FIELDS.filter(f => resultsDesc.some(r => {
      const v = (r as any)[f.key];
      return v != null && v !== 0;
    })),
    [resultsDesc]
  );

  // Detect extra fields present in results (all types for table, numeric for charts)
  const presentExtraFields = useMemo(() => {
    const allKeys = new Set<string>();
    resultsDesc.forEach(r => {
      if (r.extra) Object.keys(r.extra).forEach(k => {
        const v = (r.extra as any)[k];
        if (v != null && v !== '' && v !== 0) allKeys.add(k);
      });
    });
    return [...allKeys].sort();
  }, [resultsDesc]);

  // Numeric-only extra fields (for chart lines)
  const numericExtraFields = useMemo(() =>
    presentExtraFields.filter(k =>
      resultsDesc.some(r => r.extra && typeof (r.extra as any)[k] === 'number' && (r.extra as any)[k] !== 0)
    ),
    [presentExtraFields, resultsDesc]
  );

  // Chartable fields: all numeric standard fields + all numeric extra fields
  const chartableStdFields = presentStdFields.filter(f =>
    f.key !== 'status_code' && resultsDesc.some(r => typeof (r as any)[f.key] === 'number')
  );

  const involvedTaskIds = useMemo(() => {
    const ids = new Set<string>();
    resultsDesc.forEach(r => ids.add(r.task_id));
    return [...ids];
  }, [resultsDesc]);

  const isMultiTask = !taskId && involvedTaskIds.length > 1;

  // Task name labels for multi-task chart
  const taskNameColorMap = useMemo(() => {
    const m = new Map<string, string>();
    involvedTaskIds.forEach((tid, i) => {
      const name = taskMap.get(tid)?.name ?? tid.replace('ext_', '');
      m.set(name, TASK_COLORS[i % TASK_COLORS.length]);
    });
    return m;
  }, [involvedTaskIds, taskMap]);

  // Downsample: keep at most MAX_CHART_POINTS for chart rendering
  const MAX_CHART_POINTS = 1000;

  // Multi-task chart data
  const multiTaskChartData = useMemo(() => {
    if (!isMultiTask) return [];
    const byTask = new Map<string, ProbeResult[]>();
    resultsAsc.forEach(r => {
      if (!byTask.has(r.task_id)) byTask.set(r.task_id, []);
      byTask.get(r.task_id)!.push(r);
    });
    const allTimes = resultsAsc.map(r => r.timestamp);
    const uniqueTimes = [...new Set(allTimes)].sort();
    const lookup = new Map<string, Map<string, number>>();
    byTask.forEach((results, tid) => {
      const m = new Map<string, number>();
      results.forEach(r => m.set(r.timestamp, r.latency_ms));
      lookup.set(tid, m);
    });
    const sampledTimes = downsample(uniqueTimes, MAX_CHART_POINTS);
    return sampledTimes.map((ts) => {
      const row: Record<string, any> = { time: new Date(ts).getTime() };
      involvedTaskIds.forEach(tid => {
        const taskName = taskMap.get(tid)?.name ?? tid.replace('ext_', '');
        row[taskName] = lookup.get(tid)?.get(ts) ?? undefined;
      });
      return row;
    });
  }, [isMultiTask, resultsAsc, involvedTaskIds, taskMap]);

  // Fields where we want MAX (not avg) to preserve spikes/anomalies
  const maxFields = new Set(['Loss%', 'Jitter', 'packet_loss_pct', 'jitter_ms',
    'effective_loss_pct', 'out_of_order_pct', 'retransmits']);

  // Single task chart data — bucket-aggregate to preserve anomalies
  const singleTaskChartData = useMemo(() => {
    if (isMultiTask) return [];
    if (resultsAsc.length <= MAX_CHART_POINTS) {
      // No need to aggregate, use raw data
      return resultsAsc.map(r => {
        const extra = (r.extra ?? {}) as Record<string, any>;
        const row: Record<string, any> = { time: new Date(r.timestamp).getTime() };
        chartableStdFields.forEach(f => {
          const v = (r as any)[f.key];
          if (v != null) row[f.label] = f.key === 'download_bps' || f.key === 'upload_bps' ? v / 1e6 : v;
        });
        numericExtraFields.forEach(k => {
          if (extra[k] != null && typeof extra[k] === 'number') row[`extra:${k}`] = extra[k];
        });
        return row;
      });
    }

    // Bucket-aggregate: split into MAX_CHART_POINTS buckets, compute avg/max per field
    const bucketSize = Math.ceil(resultsAsc.length / MAX_CHART_POINTS);
    const chartData: Record<string, any>[] = [];

    for (let i = 0; i < resultsAsc.length; i += bucketSize) {
      const bucket = resultsAsc.slice(i, Math.min(i + bucketSize, resultsAsc.length));
      const midpoint = bucket[Math.floor(bucket.length / 2)];
      const row: Record<string, any> = { time: new Date(midpoint.timestamp).getTime() };

      // For each field, collect values from the bucket
      chartableStdFields.forEach(f => {
        const vals: number[] = [];
        bucket.forEach(r => {
          let v = (r as any)[f.key];
          if (v != null) {
            if (f.key === 'download_bps' || f.key === 'upload_bps') v = v / 1e6;
            vals.push(v);
          }
        });
        if (vals.length > 0) {
          // Use MAX for anomaly fields (loss, jitter), AVG for everything else
          row[f.label] = maxFields.has(f.label) || maxFields.has(f.key)
            ? Math.max(...vals)
            : vals.reduce((s, v) => s + v, 0) / vals.length;
        }
      });

      numericExtraFields.forEach(k => {
        const vals: number[] = [];
        bucket.forEach(r => {
          const extra = (r.extra ?? {}) as Record<string, any>;
          if (extra[k] != null && typeof extra[k] === 'number') vals.push(extra[k]);
        });
        if (vals.length > 0) {
          row[`extra:${k}`] = maxFields.has(k)
            ? Math.max(...vals)
            : vals.reduce((s, v) => s + v, 0) / vals.length;
        }
      });

      chartData.push(row);
    }
    return chartData;
  }, [isMultiTask, resultsAsc, chartableStdFields, numericExtraFields, MAX_CHART_POINTS, maxFields]);

  // Build chart line definitions, each tagged with a yAxisId by unit group
  const chartLines = useMemo(() => {
    const lines: { key: string; name: string; color: string; yAxisId: string }[] = [];
    let ci = 0;
    chartableStdFields.forEach(f => {
      const yAxisId = (f.key === 'download_bps' || f.key === 'upload_bps') ? 'bps' : 'default';
      lines.push({ key: f.label, name: `${f.label} (${f.unit})`, color: EXTRA_COLORS[ci++ % EXTRA_COLORS.length], yAxisId });
    });
    numericExtraFields.forEach(k => {
      // available_outgoing_bitrate is bps-scale, everything else is small-scale
      const yAxisId = k === 'available_outgoing_bitrate' ? 'bps' : 'default';
      lines.push({ key: `extra:${k}`, name: getShortName(k), color: EXTRA_COLORS[ci++ % EXTRA_COLORS.length], yAxisId });
    });
    return lines;
  }, [chartableStdFields, presentExtraFields]);

  const hasAnyData = resultsDesc.length > 0;

  const queryClient = useQueryClient();
  const clearMutation = useMutation({
    mutationFn: () => api.clearResults(taskId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['results'] }),
  });

  const handleClear = () => {
    if (!taskId) return;
    const taskName = taskMap.get(taskId)?.name ?? taskId;
    if (window.confirm(`Clear all history for "${taskName}"?`)) {
      clearMutation.mutate();
    }
  };

  // Determine which Y-axis groups have visible (non-hidden) lines
  const hasBpsAxis = chartLines.some(l => l.yAxisId === 'bps' && !hiddenLines.has(l.key));
  const hasDefaultAxis = chartLines.some(l => l.yAxisId === 'default' && !hiddenLines.has(l.key));

  // Detect default-axis unit from visible lines
  const defaultAxisUnit = useMemo(() => {
    const visibleStd = chartableStdFields.filter(f => {
      const key = f.label;
      return !hiddenLines.has(key) && f.key !== 'download_bps' && f.key !== 'upload_bps';
    });
    if (visibleStd.length === 0) return '';
    const units = new Set(visibleStd.map(f => f.unit));
    return units.size === 1 ? [...units][0] : '';
  }, [chartableStdFields, hiddenLines]);

  // ---- Export: chart PNG + data Excel ----
  const handleExport = async () => {
    let XLSX: any;
    try {
      const mod = await import('xlsx');
      XLSX = mod.default || mod;
    } catch (e) {
      console.error('Failed to load xlsx:', e);
      alert('Export module failed to load.');
      return;
    }
    const taskLabel = taskId ? (taskMap.get(taskId)?.name ?? taskId).replace(/[^a-zA-Z0-9_-]/g, '_') : 'all';
    const now = new Date().toISOString().slice(0, 16).replace(/[T:]/g, '-');
    const baseName = `probex-${taskLabel}-${timeRange}-${now}`;

    // 1. Export chart as PNG
    const svgEl = document.querySelector('.recharts-responsive-container svg') as SVGSVGElement | null;
    if (svgEl) {
      try {
        const svgData = new XMLSerializer().serializeToString(svgEl);
        const rect = svgEl.getBoundingClientRect();
        const scale = 2;
        const canvas = document.createElement('canvas');
        canvas.width = rect.width * scale;
        canvas.height = rect.height * scale;
        const ctx = canvas.getContext('2d')!;
        ctx.scale(scale, scale);
        ctx.fillStyle = '#fff';
        ctx.fillRect(0, 0, rect.width, rect.height);
        const img = new Image();
        const blob = new Blob([svgData], { type: 'image/svg+xml;charset=utf-8' });
        const url = URL.createObjectURL(blob);
        await new Promise<void>((resolve) => {
          img.onload = () => { ctx.drawImage(img, 0, 0, rect.width, rect.height); URL.revokeObjectURL(url); resolve(); };
          img.onerror = () => { URL.revokeObjectURL(url); resolve(); };
          img.src = url;
        });
        canvas.toBlob((pngBlob) => {
          if (!pngBlob) return;
          const a = document.createElement('a');
          a.href = URL.createObjectURL(pngBlob);
          a.download = baseName + '-chart.png';
          a.click();
          URL.revokeObjectURL(a.href);
        }, 'image/png');
      } catch (e) {
        console.warn('Chart PNG export failed:', e);
      }
    }

    // 2. Export data table as Excel
    const wb = XLSX.utils.book_new();

    const getDescription = (raw: string) => FIELD_DICT[raw]?.[1] ?? '';

    // Build headers with short names
    const rawHeaders: string[] = ['Time'];
    if (isMultiTask) rawHeaders.push('Task');
    rawHeaders.push('Status');
    presentStdFields.forEach(f => rawHeaders.push(f.label));
    presentExtraFields.forEach(k => rawHeaders.push(k));
    rawHeaders.push('Error');

    const shortHeaders = rawHeaders.map(h => getShortName(h));

    const dataRows = resultsDesc.slice(0, 5000).map(r => {
      const extra = (r.extra ?? {}) as Record<string, any>;
      const row: any[] = [new Date(r.timestamp).toLocaleString()];
      if (isMultiTask) row.push(taskMap.get(r.task_id)?.name ?? r.task_id);
      row.push(r.success ? 'OK' : 'FAIL');
      presentStdFields.forEach(f => {
        const v = (r as any)[f.key];
        if (v == null) { row.push(''); return; }
        if (f.key === 'download_bps' || f.key === 'upload_bps') row.push(Number((v / 1e6).toFixed(2)));
        else if (typeof v === 'number') row.push(Number(v.toFixed(2)));
        else row.push(v);
      });
      presentExtraFields.forEach(k => {
        const v = extra[k];
        if (v == null) row.push('');
        else if (typeof v === 'number') row.push(Number(v % 1 === 0 ? v : Number(v.toFixed(2))));
        else row.push(String(v));
      });
      row.push(r.error || '');
      return row;
    });

    // Sheet 1: Results data with short column names
    const ws = XLSX.utils.aoa_to_sheet([shortHeaders, ...dataRows]);
    const dataWidths = shortHeaders.map((_h: string, i: number) => {
      let max = _h.length;
      dataRows.slice(0, 200).forEach((row: any[]) => {
        const v = String(row[i] ?? '');
        if (v.length > max) max = v.length;
      });
      return max;
    });
    ws['!cols'] = dataWidths.map((w: number) => ({ wch: Math.max(8, Math.min(w + 2, 24)) }));
    XLSX.utils.book_append_sheet(wb, ws, 'Results');

    // Sheet 2: Dictionary — explains each column
    const dictRows = [['Column', 'Field Name', 'Description']];
    rawHeaders.forEach((raw, i) => {
      dictRows.push([shortHeaders[i], raw, getDescription(raw) || raw]);
    });
    const wsDict = XLSX.utils.aoa_to_sheet(dictRows);
    wsDict['!cols'] = [{ wch: 12 }, { wch: 30 }, { wch: 60 }];
    XLSX.utils.book_append_sheet(wb, wsDict, 'Dictionary');

    XLSX.writeFile(wb, baseName + '.xlsx');
  };

  return (
    <div>
      <h1 style={{ fontSize: '1.5rem', fontWeight: 600, marginBottom: '1rem' }}>Results</h1>

      <div style={{ display: 'flex', gap: '1rem', marginBottom: '1rem', alignItems: 'center' }}>
        <select value={taskId} onChange={e => { setTaskId(e.target.value); setHiddenLines(new Set()); }} style={selectStyle}>
          <option value="">All Tasks</option>
          {tasks.map((t) => (
            <option key={t.id} value={t.id}>{t.name} ({t.target}) [{t.probe_type}]</option>
          ))}
          {/* External probe virtual task IDs — discovered from latest results (one per task) */}
          {(() => {
            const taskIds = new Set(tasks.map(t => t.id));
            const externalIds = [...new Set(latestResults.map(r => r.task_id))].filter(id => !taskIds.has(id));
            return externalIds.map(id => (
              <option key={id} value={id}>{id.replace('ext_', '')} [external]</option>
            ));
          })()}
        </select>
        <select value={timeRange} onChange={e => setTimeRange(e.target.value)} style={selectStyle}>
          <option value="1h">Last 1 hour</option>
          <option value="6h">Last 6 hours</option>
          <option value="24h">Last 24 hours</option>
          <option value="7d">Last 7 days</option>
          <option value="custom">Custom range</option>
        </select>
        {isCustom && (
          <>
            <input type="datetime-local" value={customFrom}
              onChange={e => setCustomFrom(e.target.value)}
              style={{ ...selectStyle, fontSize: '0.8rem' }}
              title="From" />
            <span style={{ color: '#6b7280', fontSize: '0.875rem' }}>to</span>
            <input type="datetime-local" value={customTo}
              onChange={e => setCustomTo(e.target.value)}
              style={{ ...selectStyle, fontSize: '0.8rem' }}
              title="To" />
          </>
        )}
        {taskId && (
          <button onClick={handleClear} disabled={clearMutation.isPending}
            style={{ padding: '0.5rem 0.75rem', border: '1px solid #fca5a5', borderRadius: 6, background: '#fff',
              color: '#ef4444', fontSize: '0.8rem', cursor: 'pointer' }}>
            {clearMutation.isPending ? 'Clearing...' : 'Clear History'}
          </button>
        )}
      </div>

      {isLoading ? <p>Loading...</p> : (
        <>
          {/* Chart */}
          {hasAnyData && (
            <div style={{ background: '#fff', borderRadius: 8, padding: '1rem', border: '1px solid #e5e7eb', marginBottom: '1rem' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.5rem' }}>
                <h2 style={{ fontSize: '1rem', fontWeight: 500, margin: 0 }}>
                  {isMultiTask ? 'Latency by Task' : 'Metrics Over Time'}
                </h2>
                <div style={{ display: 'flex', gap: '0.5rem' }}>
                  <button onClick={() => setHiddenLines(new Set())}
                    style={legendBtnStyle} title="Show all metrics">All</button>
                  <button onClick={() => setHiddenLines(new Set(chartLines.map(l => l.key)))}
                    style={legendBtnStyle} title="Hide all metrics">None</button>
                  <button onClick={() => {
                    const allKeys = new Set(chartLines.map(l => l.key));
                    setHiddenLines(prev => {
                      const next = new Set<string>();
                      allKeys.forEach(k => { if (!prev.has(k)) next.add(k); });
                      return next;
                    });
                  }} style={legendBtnStyle} title="Invert selection">Invert</button>
                  <button onClick={handleExport}
                    style={{ ...legendBtnStyle, background: '#059669', color: '#fff', border: 'none' }}
                    title="Export chart & table to Excel">Export</button>
                </div>
              </div>
              <ResponsiveContainer width="100%" height={300}>
                {isMultiTask ? (
                  <LineChart data={multiTaskChartData}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="time" type="number" scale="time" domain={['dataMin', 'dataMax']}
                      ticks={generateTimeTicks(multiTaskChartData)}
                      tickFormatter={formatXTick} tick={{ fontSize: 10 }} angle={-25} textAnchor="end" height={50} />
                    <YAxis tick={{ fontSize: 11 }} unit="ms" />
                    <Tooltip labelFormatter={(v) => typeof v === 'number' ? formatTooltipTime(v) : v} />
                    <Legend onClick={(e) => toggleLine(e.dataKey as string)} wrapperStyle={{ cursor: 'pointer' }}
                      formatter={(value, entry) => (
                        <span style={{
                          color: hiddenLines.has(entry.dataKey as string) ? '#d1d5db' : (entry.color ?? '#333'),
                          textDecoration: hiddenLines.has(entry.dataKey as string) ? 'line-through' : 'none',
                        }}>{value}</span>
                      )} />
                    {[...taskNameColorMap.entries()].map(([name, color]) => (
                      <Line key={name} type="monotone" dataKey={name} stroke={color} name={name} dot={false} hide={hiddenLines.has(name)} connectNulls={false} />
                    ))}
                  </LineChart>
                ) : (
                  <LineChart data={singleTaskChartData}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="time" type="number" scale="time" domain={['dataMin', 'dataMax']}
                      ticks={generateTimeTicks(singleTaskChartData)}
                      tickFormatter={formatXTick} tick={{ fontSize: 10 }} angle={-25} textAnchor="end" height={50} />
                    <YAxis yAxisId="default" tick={{ fontSize: 11 }} unit={defaultAxisUnit}
                      hide={!hasDefaultAxis} domain={['auto', 'auto']} />
                    <YAxis yAxisId="bps" orientation="right" tick={{ fontSize: 11 }} unit="Mbps"
                      hide={!hasBpsAxis} domain={['auto', 'auto']} />
                    <Tooltip labelFormatter={(v) => typeof v === 'number' ? formatTooltipTime(v) : v} />
                    <Legend onClick={(e) => toggleLine(e.dataKey as string)} wrapperStyle={{ cursor: 'pointer' }}
                      formatter={(value, entry) => (
                        <span style={{
                          color: hiddenLines.has(entry.dataKey as string) ? '#d1d5db' : (entry.color ?? '#333'),
                          textDecoration: hiddenLines.has(entry.dataKey as string) ? 'line-through' : 'none',
                        }}>{value}</span>
                      )} />
                    {chartLines.map(line => (
                      <Line key={line.key} type="monotone" dataKey={line.key} stroke={line.color}
                        yAxisId={line.yAxisId} name={line.name} dot={false} hide={hiddenLines.has(line.key)} connectNulls />
                    ))}
                  </LineChart>
                )}
              </ResponsiveContainer>
            </div>
          )}

          {/* Table */}
          <div style={{ background: '#fff', borderRadius: 8, border: '1px solid #e5e7eb', maxHeight: 420, overflowY: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.875rem' }}>
              <thead style={{ position: 'sticky', top: 0, background: '#fff', zIndex: 1 }}>
                <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                  <th style={thStyle}>Time</th>
                  {isMultiTask && <th style={thStyle}>Task</th>}
                  <th style={thStyle}>Status</th>
                  {presentStdFields.map(f => <th key={f.key} style={thStyle}>{f.label}</th>)}
                  {presentExtraFields.map(k => <th key={k} style={{ ...thStyle, fontSize: '0.75rem' }} title={k}>{getShortName(k)}</th>)}
                  <th style={thStyle}>Error</th>
                </tr>
              </thead>
              <tbody>
                {resultsDesc.slice(0, 500).map((r) => {
                  const task = taskMap.get(r.task_id);
                  const extra = (r.extra ?? {}) as Record<string, any>;
                  return (
                    <tr key={r.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                      <td style={tdStyle}>{new Date(r.timestamp).toLocaleTimeString()}</td>
                      {isMultiTask && (
                        <td style={tdStyle}>
                          <span style={{
                            display: 'inline-block', padding: '1px 6px', borderRadius: 3, fontSize: '0.75rem',
                            background: taskNameColorMap.get(task?.name ?? r.task_id.replace('ext_', '')) ?? '#e5e7eb', color: '#fff',
                          }}>
                            {task?.name ?? r.task_id.replace('ext_', '')}
                          </span>
                        </td>
                      )}
                      <td style={tdStyle}>
                        <span style={{ color: r.success ? '#22c55e' : '#ef4444' }}>
                          {r.success ? 'OK' : 'FAIL'}
                        </span>
                      </td>
                      {presentStdFields.map(f => {
                        const v = (r as any)[f.key];
                        return <td key={f.key} style={tdStyle}>{v != null ? f.fmt(v) : '-'}</td>;
                      })}
                      {presentExtraFields.map(k => {
                        const v = extra[k];
                        let display = '-';
                        if (v != null) {
                          if (typeof v === 'boolean') display = v ? 'Yes' : 'No';
                          else if (typeof v === 'number') display = v % 1 === 0 ? String(v) : v.toFixed(2);
                          else display = String(v);
                        }
                        return <td key={k} style={tdStyle}>{display}</td>;
                      })}
                      <td style={tdStyle} title={r.error}>{r.error ? r.error.slice(0, 40) : '-'}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            {resultsDesc.length === 0 && <p style={{ padding: '2rem', textAlign: 'center', color: '#6b7280' }}>No results for this filter</p>}
          </div>
        </>
      )}
    </div>
  );
}

// Track the last displayed day to show date only at day boundaries on X axis.
let _lastTickDay = '';

function formatXTick(epochMs: number): string {
  const d = new Date(epochMs);
  const day = `${d.getMonth() + 1}/${d.getDate()}`;
  const time = d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  if (day !== _lastTickDay) {
    _lastTickDay = day;
    return `${day} ${time}`;
  }
  return time;
}

/** Generate evenly-spaced tick values so the X-axis never gets overcrowded. */
function generateTimeTicks(data: Record<string, any>[], maxTicks = 8): number[] {
  if (data.length === 0) return [];
  const times = data.map(d => d.time as number).filter(t => typeof t === 'number' && isFinite(t));
  if (times.length === 0) return [];
  const min = times[0];
  const max = times[times.length - 1];
  if (min === max) return [min];
  const count = Math.min(maxTicks, times.length);
  const step = (max - min) / (count - 1);
  const ticks: number[] = [];
  for (let i = 0; i < count; i++) {
    ticks.push(Math.round(min + step * i));
  }
  return ticks;
}

function formatTooltipTime(epochMs: number): string {
  return new Date(epochMs).toLocaleString();
}

// Downsample an array to at most maxPoints entries, evenly spaced.
function downsample<T>(arr: T[], maxPoints: number): T[] {
  if (arr.length <= maxPoints) return arr;
  const step = arr.length / maxPoints;
  const result: T[] = [];
  for (let i = 0; i < maxPoints; i++) {
    result.push(arr[Math.floor(i * step)]);
  }
  // Always include last element
  if (result[result.length - 1] !== arr[arr.length - 1]) {
    result.push(arr[arr.length - 1]);
  }
  return result;
}

const selectStyle: React.CSSProperties = {
  padding: '0.5rem', border: '1px solid #d1d5db', borderRadius: 6,
  fontSize: '0.875rem', background: '#fff',
};
const thStyle: React.CSSProperties = { padding: '0.75rem 0.5rem', fontWeight: 500 };
const tdStyle: React.CSSProperties = { padding: '0.5rem' };
const legendBtnStyle: React.CSSProperties = {
  padding: '2px 10px', border: '1px solid #d1d5db', borderRadius: 4,
  background: '#f9fafb', fontSize: '0.75rem', cursor: 'pointer', color: '#374151',
};

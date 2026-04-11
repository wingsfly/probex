# 面向音视频实时流的探针完善方案

## 一、问题分析：当前探针体系的差距

### 1.1 现有探针能力

| 探针 | 能力 | 与 RTC 相关性 |
|------|------|-------------|
| ICMP | RTT、抖动、丢包 | **低** — ICMP 走不同路径、被限速，无法反映 UDP 数据面质量 |
| TCP | 连接延迟 | **低** — RTC 基于 UDP，TCP 行为差异大 |
| HTTP | DNS/TLS/TTFB 分解 | **中** — 可用于信令链路质量评估 |
| DNS | 解析延迟 | **低** — 仅解析阶段相关 |
| iperf3 | 带宽、jitter、丢包、乱序 | **高** — UDP 模式最接近 RTC 流量特征 |

### 1.2 关键差距

根据调研文档的核心发现和行业实践，现有体系缺少以下关键能力：

**差距 1：无 RTP 模拟探针**
- iperf3 的 UDP 模式虽能测乱序/丢包，但包大小、发送模式与真实 RTP 流差异大
- RTP 音频流：160-200 字节/包、50 pps（20ms 帧间隔）
- RTP 视频流：~1200 字节/包、可变速率、有关键帧突发
- 真实 RTC 场景需要模拟这种流量模式才能准确评估

**差距 2：无 MOS 评分能力**
- 行业标准是 ITU-T G.107 E-model，将网络指标映射为 R-factor 和 MOS
- callstats.io、Twilio、Vonage 等均内置 MOS 估算
- 文档中已给出 E-model 公式，但未在探针层面实现自动计算

**差距 3：无 TURN/STUN 连通性探测**
- WebRTC 依赖 ICE 协商，TURN/STUN 可达性是通话建立的前提
- 行业工具（Trickle ICE、testRTC probeRTC）均包含此类探测
- 文档强调 TURN relay 是解决 ECMP 乱序的关键方案

**差距 4：无 WebRTC 客户端指标采集**
- callstats.io、Peermetrics 等均通过 JS SDK 采集 getStats() 数据
- 客户端 MOS、帧丢失率、jitter buffer 延迟等是真实体验的直接指标
- 文档中详细列出了需要采集的 getStats() 字段

**差距 5：无乱序深度分析**
- iperf3 仅给出 OoO 计数和百分比，不提供乱序深度分布
- 乱序深度决定 jitter buffer 能否恢复（深度 < buffer 窗口内的包可恢复）
- 这是评估"有效丢包率"的关键输入

**差距 6：无 QoE 综合评估**
- 各探针独立运行，缺少跨指标的综合质量评分
- 需要融合 RTT、jitter、丢包、乱序、带宽等指标给出统一 QoE 评级

---

## 二、行业参考方案

### 2.1 商业平台架构

#### callstats.io（8x8）

```
浏览器端 JS SDK → 采集 getStats() → 推送到云端
云端聚合分析 → 计算每参与者 MOS → 可视化仪表盘
```

- 核心价值：**被动采集真实通话质量**，零网络额外开销
- 支持 Twilio、Vonage、Jitsi、Mediasoup 等主流 SFU/MCU
- 计算 MOS 使用 E-model 变体 + 编码器特定损伤因子

#### testRTC / Cyara 产品矩阵

| 产品 | 定位 | 技术实现 |
|------|------|---------|
| probeRTC | 合成监控 | 部署在各地的 Docker 代理，运行真实浏览器实例模拟通话 |
| watchRTC | 通话中监控 | JS SDK 嵌入客户端，实时采集 getStats() |
| qualityRTC | 通话前诊断 | 浏览器端零安装，测试网络/设备/连通性 |
| testingRTC | 负载测试 | 批量启动浏览器实例，每个映射一个参会者 |

**关键洞察**：testRTC 的合成监控使用**真实浏览器 + 注入的媒体流**，而非简单的 UDP 探测。这是最准确但最重资源的方案。

#### Twilio / Agora 通话前诊断

- **Twilio Preflight API**：发起环回通话（loopback），测试信令 + 媒体连通性 + 质量
- **Agora lastmileProbeTest**：2 秒出主观质量评分，30 秒出客观指标（丢包率、jitter、可用带宽）
- 共同点：都是在**应用层模拟真实 RTC 交互**，而非底层网络探测

### 2.2 开源工具

| 工具 | 用途 | 与 ProbeX 的关系 |
|------|------|-----------------|
| rtcscore (JS) | 从 getStats() 估算音视频 MOS | 可集成到客户端 SDK |
| Peermetrics | 自托管 WebRTC 分析平台 | 参考其采集架构 |
| VoIPmonitor | SIP/RTP 抓包 + MOS 计算 | 参考其 MOS 算法实现 |
| Homer/SIPCAPTURE | VoIP 信令抓包分析 | 信令层监控参考 |
| webrtc-issue-detector | WebRTC 问题诊断 + MOS | 参考其诊断逻辑 |

### 2.3 行业共识的核心指标

ITU-T、SRTP/RTP 相关 RFC、以及各商业平台共同关注的 RTC 质量指标：

| 层级 | 指标 | 来源 | 权重 |
|------|------|------|------|
| 网络层 | RTT | ICE candidate-pair / UDP echo | 极高 |
| 网络层 | Jitter | RTP inter-arrival / iperf3 UDP | 高 |
| 网络层 | Packet Loss | RTCP RR / iperf3 UDP | 极高 |
| 网络层 | Reorder Rate + Depth | iperf3 UDP / 自定义 UDP 探针 | 高（ECMP 场景） |
| 传输层 | TURN/STUN 可达性 | ICE 探测 | 关键（建连前提） |
| 传输层 | 可用带宽 | GCC BWE / iperf3 TCP | 中 |
| 媒体层 | MOS (E-model) | 计算模型 | 极高 |
| 媒体层 | 帧率 / 帧丢失率 | getStats() framesDecoded/Dropped | 高 |
| 媒体层 | Jitter Buffer 延迟 | getStats() jitterBufferDelay | 中 |
| 媒体层 | NACK / FIR / PLI 请求数 | getStats() nackCount/firCount | 中 |
| 媒体层 | 音频隐藏事件 | getStats() concealedSamples | 高 |

---

## 三、设计方案

### 3.1 架构总览

```
┌─────────────────────────────────────────────────────────┐
│                    ProbeX Controller                     │
│                                                         │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────┐  │
│  │ 现有探针  │ │ RTC 探针  │ │ QoE 引擎 │ │ 客户端采集│  │
│  │ icmp,tcp │ │(新增 4 个)│ │(MOS 计算)│ │ API 入口  │  │
│  │ http,dns │ │          │ │          │ │           │  │
│  │ iperf3   │ │          │ │          │ │           │  │
│  └──────────┘ └──────────┘ └──────────┘ └───────────┘  │
│                       ↓                      ↑          │
│              ┌────────────────┐    ┌──────────────────┐ │
│              │  Result Store  │    │ WebRTC Stats API │ │
│              │  + Alert Eval  │    │ POST /webrtc/stats│ │
│              └────────────────┘    └──────────────────┘ │
└─────────────────────────────────────────────────────────┘
         ↑                                    ↑
    Remote Agent                    浏览器 JS SDK
    (分布式探测)                 (getStats() → POST)
```

### 3.2 新增探针设计

#### 探针 1：`udp-rtp-sim` — RTP 流量模拟探针

**目标**：发送模拟 RTP 特征的 UDP 流量（包大小、发送速率、序号），在接收端精确测量乱序深度、jitter、丢包。

**原理**：
```
发送端(Agent)                              接收端(Target echo server)
  │                                           │
  │── [seq=1, ts, payload=160B] ──────────►   │
  │── [seq=2, ts, payload=160B] ──────────►   │
  │── ...每 20ms 一个包...                     │
  │                                           │
  │◄── [echo: seq, 发送时间, 到达时间] ────── │
  │                                           │
  分析: 乱序深度、jitter 分布、丢包模式
```

**配置参数**：

```json
{
  "mode": "audio",
  "packet_size": 160,
  "pps": 50,
  "duration": 10,
  "echo_port": 60099
}
```

| 参数 | 说明 | 默认值 |
|------|------|-------|
| mode | `audio` (160B/50pps) 或 `video` (1200B/变速率) 或 `custom` | audio |
| packet_size | 包大小(字节) | 160 (audio) / 1200 (video) |
| pps | 每秒发包数 | 50 (audio) / 30 (video) |
| duration | 测试时长(秒) | 10 |
| echo_port | 回声服务器端口 | 60099 |

**输出指标**（Extra 字段）：

```json
{
  "sent_packets": 500,
  "received_packets": 485,
  "lost_packets": 15,
  "loss_pct": 3.0,
  "out_of_order": 42,
  "out_of_order_pct": 8.4,
  "reorder_depth_avg": 2.3,
  "reorder_depth_p95": 5,
  "reorder_depth_max": 12,
  "jitter_avg_ms": 4.2,
  "jitter_p95_ms": 12.8,
  "jitter_p99_ms": 28.3,
  "rtt_avg_ms": 52.1,
  "rtt_p95_ms": 68.4,
  "effective_loss_pct_60ms": 5.2,
  "effective_loss_pct_120ms": 2.1,
  "effective_loss_pct_200ms": 0.8,
  "mos_estimated": 3.7,
  "r_factor": 78.5
}
```

**亮点设计**：
- `reorder_depth_*`: 乱序深度分布——这是 iperf3 不提供但对 jitter buffer 配置至关重要的指标
- `effective_loss_pct_Xms`: 在不同 jitter buffer 大小下的有效丢包率——直接回答"设多大 buffer 够用"
- `mos_estimated` / `r_factor`: 基于 E-model 内联计算 MOS

**实现要点**：
- 发送端: 自定义 UDP 客户端，包头含 [seq_num(4B) + send_timestamp_us(8B) + padding]
- 接收端: `probex-echo` 微服务（Go 实现），原样回弹或记录 + 返回统计
- 乱序深度: 维护已收到的最大 seq，每个乱序包的深度 = max_seq_received - pkt_seq

#### 探针 2：`stun-turn` — TURN/STUN 连通性探针

**目标**：验证 TURN/STUN 服务器的可达性、响应时间、以及能否获取 relay 候选。

**原理**：
```
Agent                     STUN/TURN Server
  │                           │
  │── STUN Binding Request ──►│
  │◄── Binding Response ──────│  → srflx 候选(NAT 外部地址)
  │                           │
  │── TURN Allocate Request ──►│
  │◄── Allocate Response ─────│  → relay 候选(TURN 分配地址)
  │                           │
  测量: STUN RTT, TURN RTT, 候选类型, 是否成功
```

**配置参数**：

```json
{
  "stun_url": "stun:stun.l.google.com:19302",
  "turn_url": "turn:turn.example.com:3478",
  "turn_username": "user",
  "turn_password": "pass",
  "timeout": "5s"
}
```

**输出指标**：

```json
{
  "stun_reachable": true,
  "stun_rtt_ms": 23.4,
  "stun_mapped_address": "203.0.113.5:54321",
  "turn_reachable": true,
  "turn_rtt_ms": 45.2,
  "turn_relayed_address": "198.51.100.1:49200",
  "turn_auth_ok": true
}
```

**参考**：
- Trickle ICE (webrtc.github.io) 的探测逻辑
- pion/stun (Go) 开源库可直接使用
- Metered / ICE Tester 等在线工具的探测模型

#### 探针 3：`traceroute-udp` — UDP 路径分析探针

**目标**：以 UDP 数据面的视角分析路由路径，检测 ECMP 节点，对比与 ICMP traceroute 的路径差异。

**原理**：
```
发送 TTL=1 的 UDP 包 → Hop1 返回 ICMP TTL Exceeded
发送 TTL=2 的 UDP 包 → Hop2 返回 ICMP TTL Exceeded
...
重复 N 次，每次用不同源端口 → 暴露 per-packet ECMP
```

**配置参数**：

```json
{
  "max_hops": 30,
  "probes_per_hop": 3,
  "ecmp_probes": 5,
  "port": 33434,
  "timeout": "3s"
}
```

**输出指标**：

```json
{
  "hops": [
    {
      "ttl": 1,
      "addresses": ["10.0.0.1"],
      "rtt_avg_ms": 1.2,
      "ecmp_detected": false
    },
    {
      "ttl": 4,
      "addresses": ["172.16.1.1", "172.16.2.1"],
      "rtt_avg_ms": 15.3,
      "ecmp_detected": true
    }
  ],
  "total_hops": 12,
  "ecmp_hops": [4, 9],
  "path_mtu": 1500
}
```

**价值**：这是文档中 `reorder-diagnosis.sh` Stage 1-2 的自动化版本，可持续运行而非一次性诊断。

#### 探针 4：`mos-composite` — QoE 综合评估探针（虚拟探针）

**目标**：不直接发包，而是**聚合同一 target 的其他探针结果**，计算综合 QoE 评分。

**原理**：
```
拉取最近 N 分钟内同 target 的探测结果
  ├── icmp  → baseline RTT, jitter
  ├── iperf3 (UDP) → 乱序率, 丢包, 带宽
  ├── udp-rtp-sim → 有效丢包率, 乱序深度, MOS
  ├── http → DNS/TLS 延迟
  └── stun-turn → TURN 可达性
              ↓
       E-model + 视频帧丢失模型
              ↓
       综合 QoE 评分 + 分级 + 建议
```

**输出指标**：

```json
{
  "overall_score": "GOOD",
  "mos_audio": 4.1,
  "mos_video": 3.6,
  "r_factor": 82,
  "video_frame_loss_pct": 3.2,
  "effective_loss_pct": 1.8,
  "contributing_factors": {
    "rtt_ms": 52,
    "jitter_ms": 8.3,
    "packet_loss_pct": 0.5,
    "reorder_pct": 12.0,
    "reorder_depth_p95": 4,
    "jitter_buffer_recommendation_ms": 120
  },
  "grade": "B",
  "recommendations": [
    "jitter_buffer: 建议设置为 120ms (当前默认 60ms)",
    "fec: 建议启用 FlexFEC，冗余度 25%",
    "bitrate: 建议视频码率不超过 500kbps 以降低帧丢失率"
  ]
}
```

**评分模型**（基于文档中的 E-model 和行业实践）：

```
// 音频 MOS (ITU-T G.107 E-model)
R = 93.2 - Id - Ie_eff
Id = 0.024 × delay_ms + 0.11 × (delay_ms - 177.3) × H(delay_ms - 177.3)
Ie_eff = effective_loss% × codec_factor(Opus=2.5, G.711=5.0)
MOS_audio = 1 + 0.035×R + R×(R-60)×(100-R)×7×10⁻⁶

// 视频帧丢失率
frame_loss = 1 - (1 - effective_loss)^(packets_per_frame)

// 有效丢包率 (乱序转化)
effective_loss = real_loss + reorder_pct × P(depth > buffer)

// 综合评级
A: MOS≥4.0 & frame_loss<2%  → Excellent
B: MOS≥3.6 & frame_loss<5%  → Good
C: MOS≥3.1 & frame_loss<10% → Fair
D: MOS<3.1 | frame_loss≥10% → Poor
```

### 3.3 WebRTC 客户端指标采集 API

**目标**：为浏览器端 JS SDK 提供数据上报接口，采集 `getStats()` 真实通话指标。

#### API Endpoint

```
POST /api/v1/webrtc/stats
```

**请求体**：

```json
{
  "session_id": "call-abc-123",
  "peer_id": "user-alice",
  "timestamp": "2026-04-11T10:30:00Z",
  "candidate_pair": {
    "rtt_ms": 52.0,
    "local_candidate_type": "srflx",
    "remote_candidate_type": "relay",
    "available_outgoing_bitrate": 1500000
  },
  "audio_inbound": {
    "packets_received": 15000,
    "packets_lost": 45,
    "jitter_ms": 8.2,
    "concealed_samples": 320,
    "concealment_events": 4,
    "jitter_buffer_delay_ms": 85
  },
  "video_inbound": {
    "packets_received": 45000,
    "packets_lost": 230,
    "jitter_ms": 12.5,
    "frames_decoded": 900,
    "frames_dropped": 12,
    "nack_count": 85,
    "fir_count": 2,
    "pli_count": 5,
    "jitter_buffer_delay_ms": 110,
    "frame_width": 1280,
    "frame_height": 720,
    "frames_per_second": 28
  }
}
```

**服务端处理**：
- 存入 `webrtc_stats` 表
- 实时计算 MOS（使用 rtcscore 算法的 Go 移植版）
- 触发告警评估（帧丢失率、MOS 低于阈值）
- 与同 target 的网络探针数据关联，呈现完整 QoE 视图

#### 配套 JS SDK（轻量版）

```javascript
// probex-webrtc.js (~2KB)
class ProbeXWebRTC {
  constructor(endpoint, sessionId, peerId) {
    this.endpoint = endpoint;
    this.sessionId = sessionId;
    this.peerId = peerId;
  }

  start(pc, intervalMs = 2000) {
    this.timer = setInterval(() => this.collect(pc), intervalMs);
  }

  stop() { clearInterval(this.timer); }

  async collect(pc) {
    const stats = await pc.getStats();
    const payload = { session_id: this.sessionId, peer_id: this.peerId, timestamp: new Date().toISOString() };

    stats.forEach(report => {
      if (report.type === 'candidate-pair' && report.nominated) {
        payload.candidate_pair = {
          rtt_ms: (report.currentRoundTripTime || 0) * 1000,
          available_outgoing_bitrate: report.availableOutgoingBitrate,
        };
      }
      if (report.type === 'inbound-rtp' && report.kind === 'audio') {
        payload.audio_inbound = {
          packets_received: report.packetsReceived,
          packets_lost: report.packetsLost,
          jitter_ms: (report.jitter || 0) * 1000,
          concealed_samples: report.concealedSamples,
          concealment_events: report.concealmentEvents,
          jitter_buffer_delay_ms: report.jitterBufferEmittedCount > 0
            ? (report.jitterBufferDelay / report.jitterBufferEmittedCount) * 1000 : 0,
        };
      }
      if (report.type === 'inbound-rtp' && report.kind === 'video') {
        payload.video_inbound = {
          packets_received: report.packetsReceived,
          packets_lost: report.packetsLost,
          jitter_ms: (report.jitter || 0) * 1000,
          frames_decoded: report.framesDecoded,
          frames_dropped: report.framesDropped,
          nack_count: report.nackCount,
          fir_count: report.firCount,
          pli_count: report.pliCount,
          frames_per_second: report.framesPerSecond,
        };
      }
    });

    fetch(this.endpoint + '/api/v1/webrtc/stats', {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(payload),
    }).catch(() => {});
  }
}
```

### 3.4 数据模型扩展

#### 新增表：`webrtc_stats`

```sql
CREATE TABLE IF NOT EXISTS webrtc_stats (
    id             TEXT PRIMARY KEY,
    session_id     TEXT NOT NULL,
    peer_id        TEXT NOT NULL,
    timestamp      INTEGER NOT NULL,
    rtt_ms         REAL,
    audio_loss_pct REAL,
    audio_jitter_ms REAL,
    audio_concealment_events INTEGER,
    video_loss_pct REAL,
    video_jitter_ms REAL,
    video_frames_decoded INTEGER,
    video_frames_dropped INTEGER,
    video_nack_count INTEGER,
    video_fps      REAL,
    mos_audio      REAL,
    mos_video      REAL,
    raw            TEXT  -- 完整 JSON
);
CREATE INDEX IF NOT EXISTS idx_webrtc_session ON webrtc_stats(session_id, timestamp);
```

#### ProbeResult.Extra 扩展字段规范

对 `udp-rtp-sim` 探针，Extra 需包含乱序深度直方图：

```json
{
  "reorder_depth_histogram": {
    "0": 420,
    "1": 30,
    "2": 18,
    "3": 8,
    "5+": 4
  }
}
```

### 3.5 告警规则增强

新增 RTC 专用告警指标：

| 指标 | 说明 | 建议警告阈值 | 建议严重阈值 |
|------|------|------------|------------|
| `mos_audio` | 音频 MOS | < 3.6 | < 3.1 |
| `mos_video` | 视频 MOS | < 3.5 | < 3.0 |
| `effective_loss_pct` | 有效丢包率（含乱序） | > 2% | > 5% |
| `reorder_depth_p95` | 95分位乱序深度 | > 5 | > 10 |
| `frame_loss_pct` | 视频帧丢失率 | > 5% | > 10% |
| `video_frames_dropped` | 视频丢帧数(getStats) | 持续增长 | 快速增长 |
| `jitter_buffer_delay_ms` | Jitter buffer 实际延迟 | > 150ms | > 250ms |

---

## 四、实施路线

### Phase 4a（核心能力）

1. **`udp-rtp-sim` 探针** + `probex-echo` 回声服务 — 最高价值，直接补齐 RTP 模拟和乱序深度分析
2. **QoE / MOS 计算引擎** — 内置 E-model，对所有 UDP 探测结果自动输出 MOS
3. **`stun-turn` 探针** — TURN/STUN 连通性验证

### Phase 4b（数据采集）

4. **WebRTC Stats API** — 接收浏览器端 getStats() 数据
5. **JS SDK** — 轻量采集库
6. **webrtc_stats 存储 + 前端页面** — 查看实时通话质量

### Phase 4c（高级分析）

7. **`traceroute-udp` 探针** — UDP 路径分析，ECMP 检测
8. **`mos-composite` 虚拟探针** — 跨探针聚合 + 综合 QoE + 优化建议
9. **RTC 专用仪表盘** — 热力图、MOS 趋势、乱序深度分布可视化

---

## 五、与现有架构的集成

### 复用模式

- 所有新探针实现 `probe.Prober` 接口 (`Name()` + `Probe()`)
- 结果存入已有的 `probe_results` 表，扩展信息放 `Extra` JSON
- 复用已有的 Task/Runner/Alert 体系，无需新建调度机制
- Agent 分布式架构直接支持新探针在远端运行
- `probex-echo` 作为独立微服务部署在目标端（类似 iperf3 server）

### 不改动的部分

- HTTP/DNS/TCP/ICMP 探针保持不变
- iperf3 探针保持不变（大带宽测试场景仍不可替代）
- 前端路由、组件架构不变，新增页面按现有模式
- 告警系统只需扩展可选的 metric 枚举值

---

## 六、参考标准与工具

| 参考 | 说明 |
|------|------|
| ITU-T G.107 E-model | R-factor / MOS 计算标准 |
| ITU-T G.114 | 单向延迟建议 (<150ms 推荐, <400ms 可接受) |
| RFC 3550 (RTP) | 序号、jitter 计算方法 |
| RFC 4737 | 包乱序度量标准 |
| RFC 5766 (TURN) | TURN 协议实现 |
| RFC 8445 (ICE) | ICE 候选收集与连通性检查 |
| W3C WebRTC Stats | getStats() API 规范 |
| pion/stun (Go) | STUN/TURN 协议 Go 实现 |
| rtcscore (JS) | 基于 getStats() 的 MOS 估算库 |
| callstats.io | 商业 WebRTC 监控平台参考 |
| testRTC/Cyara | 合成 WebRTC 监控参考 |
| VoIPmonitor | 开源 VoIP 监控 + MOS 计算参考 |

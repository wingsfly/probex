# 进一步监测与优化实施指南

## 1. 真实延迟监测方案

### 1.1 为什么 ICMP Ping 不能反映真实延迟

诊断中发现 Hop 9 延迟 141ms 但目标 ICMP 延迟仅 50ms，原因可能是：

| 因素 | 说明 |
|------|------|
| ICMP 与 UDP 走不同路径 | 路由器对 ICMP 和 UDP 使用不同的转发策略/优先级 |
| ICMP 限速/低优先级 | 很多路由器对 ICMP 做速率限制或放在低优先级队列 |
| 回程路径不对称 | 去程走 A 路径，回程走 B 路径，ICMP RTT ≠ 单程延迟 × 2 |
| 路由器 CPU 处理 | ICMP 回复由路由器 CPU 生成，与数据面转发延迟不同 |

### 1.2 真实应用层延迟的测量方法

#### 方法一：WebRTC 内置统计（最准确）

WebRTC 的 `RTCPeerConnection.getStats()` 提供数据面真实 RTT，不依赖 ICMP：

```javascript
// 每 2 秒采集一次，上报到 ProbeX 或自有监控
setInterval(async () => {
  const stats = await peerConnection.getStats();
  const metrics = {};

  stats.forEach(report => {
    // 候选对统计 — 包含真实 ICE 层 RTT
    if (report.type === 'candidate-pair' && report.nominated) {
      metrics.rtt_ms = report.currentRoundTripTime * 1000;
      metrics.available_outgoing_bitrate = report.availableOutgoingBitrate;
      metrics.bytes_sent = report.bytesSent;
      metrics.bytes_received = report.bytesReceived;
    }

    // 入站 RTP — 接收端视角
    if (report.type === 'inbound-rtp' && report.kind === 'video') {
      metrics.video_packets_received = report.packetsReceived;
      metrics.video_packets_lost = report.packetsLost;
      metrics.video_jitter = report.jitter * 1000; // ms
      metrics.video_frames_decoded = report.framesDecoded;
      metrics.video_frames_dropped = report.framesDropped;
      metrics.video_nack_count = report.nackCount;
      metrics.video_fir_count = report.firCount;
      metrics.video_pli_count = report.pliCount;
      // Jitter buffer 实际延迟
      if (report.jitterBufferEmittedCount > 0) {
        metrics.jitter_buffer_ms =
          (report.jitterBufferDelay / report.jitterBufferEmittedCount) * 1000;
      }
    }

    if (report.type === 'inbound-rtp' && report.kind === 'audio') {
      metrics.audio_packets_lost = report.packetsLost;
      metrics.audio_jitter = report.jitter * 1000;
      metrics.audio_concealed_samples = report.concealedSamples;
      metrics.audio_concealment_events = report.concealmentEvents;
    }
  });

  console.log('WebRTC Metrics:', metrics);
  // 可以 POST 到 ProbeX API 存储分析
}, 2000);
```

关键指标解读：

| 指标 | 含义 | 正常值 | 告警值 |
|------|------|-------|-------|
| `currentRoundTripTime` | ICE 层真实 RTT（非 ICMP） | < 200ms | > 400ms |
| `jitter` | RTP 包间到达时间变化 | < 30ms | > 100ms |
| `jitterBufferDelay` | 实际 jitter buffer 延迟 | < 100ms | > 200ms |
| `framesDropped` | 被丢弃的视频帧 | 0 | > 0 持续增长 |
| `nackCount` | NACK 重传请求 | 低 | 持续快速增长 |
| `concealedSamples` | 音频掩盖（丢包补偿） | 低 | 持续增长 |

#### 方法二：UDP 应用层 RTT 探测

自建 UDP echo 服务，测量 UDP 数据面的真实 RTT：

```python
# 服务端 (目标机器上运行)
# udp_echo_server.py
import socket, time
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.bind(('0.0.0.0', 60099))
while True:
    data, addr = sock.recvfrom(1024)
    sock.sendto(data, addr)  # 原样回传
```

```python
# 客户端探测 (可集成到 ProbeX)
# udp_rtt_probe.py
import socket, time, struct

def measure_udp_rtt(host, port=60099, count=20):
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.settimeout(3)
    rtts = []
    for i in range(count):
        payload = struct.pack('!d', time.time())  # 时间戳
        start = time.time()
        sock.sendto(payload, (host, port))
        try:
            data, _ = sock.recvfrom(1024)
            rtt = (time.time() - start) * 1000
            rtts.append(rtt)
        except socket.timeout:
            pass  # 丢包
        time.sleep(0.05)
    sock.close()
    return {
        'count': count,
        'received': len(rtts),
        'loss_pct': (count - len(rtts)) / count * 100,
        'avg_rtt': sum(rtts) / len(rtts) if rtts else 0,
        'min_rtt': min(rtts) if rtts else 0,
        'max_rtt': max(rtts) if rtts else 0,
    }
```

#### 方法三：HTTP/HTTPS TTFB（应用层延迟）

已在 ProbeX 中实现的 HTTP 探针的 Total 延迟就是应用层真实延迟，包含：

```
TTFB = DNS + TCP Connect + TLS Handshake + Server Processing + 首字节传输
```

对比：

| 方法 | 测量的是什么 | 准确度 | 适用场景 |
|------|------------|--------|---------|
| ICMP Ping | 网络层 RTT | 低 — 可能被限速/走不同路径 | 基线参考 |
| HTTP TTFB | 应用层全链路 | 中 — 含服务器处理时间 | Web 服务监测 |
| UDP Echo RTT | UDP 数据面 RTT | **高** — 与 WebRTC 走相同路径 | 最接近 WebRTC 真实体验 |
| WebRTC Stats RTT | ICE 候选对 RTT | **最高** — 就是 WebRTC 本身 | 生产环境监测 |

**建议**：在目标服务器部署 UDP Echo 服务，ProbeX 新增 `udp-echo` 探针，与 ICMP ping 对比即可看出差异。

---

## 2. 实时视频流优化方案

### 2.1 编码层优化

#### A. 降低关键帧大小和频率

关键帧（I-frame）通常是普通帧的 10-50 倍大小，丢包影响最大：

```javascript
// 服务端 SFU 或发送端配置
const sender = pc.getSenders().find(s => s.track?.kind === 'video');
const params = sender.getParameters();

// 限制关键帧间隔 — 减少 I-frame 频率
// 太短：带宽浪费；太长：丢了 I-frame 要等很久恢复
// 乱序环境建议：3-5 秒
params.encodings[0].keyFrameInterval = 150; // 约 5 秒 @30fps

await sender.setParameters(params);
```

#### B. Simulcast 分层发送

在乱序/丢包环境中，Simulcast 让 SFU 可以根据接收质量动态切换分辨率层：

```javascript
const transceiver = pc.addTransceiver(videoTrack, {
  sendEncodings: [
    {
      rid: 'low',
      maxBitrate: 150_000,      // 150kbps
      scaleResolutionDownBy: 4,  // 1/4 分辨率
      maxFramerate: 15,
    },
    {
      rid: 'mid',
      maxBitrate: 500_000,      // 500kbps
      scaleResolutionDownBy: 2,  // 1/2 分辨率
      maxFramerate: 30,
    },
    {
      rid: 'high',
      maxBitrate: 1_500_000,    // 1.5Mbps
      scaleResolutionDownBy: 1,  // 全分辨率
      maxFramerate: 30,
    },
  ],
});
```

各层的抗乱序能力：

| 层 | 码率 | 每帧包数 | 帧丢失率(2%包丢失) | 体感 |
|---|------|---------|-------------------|------|
| low | 150kbps | ~0.4 | 0.8% | 流畅 |
| mid | 500kbps | ~1.4 | 2.8% | 偶发异常 |
| high | 1.5Mbps | ~4.2 | 8.1% | 明显卡顿 |

SFU 检测到接收端 NACK/PLI 增多时，自动切换到 low 层保持流畅。

#### C. SVC (可伸缩视频编码)

VP9/AV1 的 SVC 模式比 Simulcast 更高效：

```javascript
// VP9 SVC — 单流中包含多层
const codec = RTCRtpSender.getCapabilities('video').codecs
  .find(c => c.mimeType === 'video/VP9');

// SDP 中协商 SVC
// a=rtpmap:98 VP9/90000
// a=fmtp:98 profile-id=0;tier-flag=0;level-idx=5

params.encodings[0].scalabilityMode = 'L3T3';
// L3T3 = 3 空间层 + 3 时间层
// 丢包时可以丢弃高层，保留基础层
```

SVC 优势：**丢掉一个增强层的包不影响基础层解码**，天然抗乱序。

### 2.2 传输层优化

#### A. FEC 前向纠错配置

```
SDP 协商中确保包含:

# ULP-FEC (通用)
a=rtpmap:116 red/90000
a=rtpmap:117 ulpfec/90000

# FlexFEC (更灵活，Chrome 支持)
a=rtpmap:118 flexfec-03/90000
```

FEC 冗余度建议：

| 网络质量 | FEC 冗余度 | 带宽开销 | 可恢复丢包 |
|---------|-----------|---------|-----------|
| 良好（loss<1%） | 10% | +10% | ~5% |
| 中等（loss 1-5%） | 25% | +25% | ~12% |
| **当前（OoO ~50%）** | **30-40%** | **+30-40%** | **~15-20%** |

#### B. NACK 重传优化

```javascript
// 确保 NACK 已启用（通常默认启用）
// SDP 中检查:
// a=rtcp-fb:* nack
// a=rtcp-fb:* nack pli

// 服务端 SFU 配置建议:
// - NACK 等待时间: RTT + jitter_buffer (约 200ms)
// - 最大重传次数: 2 (超过就请求关键帧)
// - RTX (重传流) 应独立于主流:
// a=rtpmap:99 rtx/90000
// a=fmtp:99 apt=96
```

#### C. 带宽自适应 (BWE) 调优

WebRTC 内置的 GCC (Google Congestion Control) 会根据丢包和延迟调整码率。在高乱序环境需注意：

```javascript
// 问题：乱序被 GCC 误判为拥塞，导致不必要的降码率
// 解决：调整 BWE 参数（需 SFU 支持或自定义 RTCPeerConnection）

// 方法 1：增加允许的丢包容忍度
// 在 SFU 侧（如 mediasoup、Janus）配置：
// maxPacketLossRate: 0.05  // 允许 5% 丢包不触发降速
// 默认是 0.02 (2%)，乱序导致的假丢包容易触发

// 方法 2：设置码率下限防止过度降速
const params = sender.getParameters();
params.encodings[0].minBitrate = 300_000; // 300kbps 下限
params.encodings[0].maxBitrate = 2_000_000;
await sender.setParameters(params);
```

### 2.3 接收端优化

#### Jitter Buffer 配置

```javascript
// Chrome 79+ 支持
const receiver = pc.getReceivers().find(r => r.track?.kind === 'video');
// 增大 jitter buffer 目标延迟
// 默认 ~60ms，乱序环境建议 120-150ms
receiver.jitterBufferTarget = 150; // ms

// 音频接收端
const audioReceiver = pc.getReceivers().find(r => r.track?.kind === 'audio');
audioReceiver.jitterBufferTarget = 120; // ms
```

| Jitter Buffer | 可容忍的乱序 | 额外延迟 | 适用场景 |
|--------------|-------------|---------|---------|
| 60ms (默认) | 乱序深度 < 3 包 | 基线 | 良好网络 |
| 120ms | 乱序深度 < 6 包 | +60ms | **当前推荐** |
| 200ms | 乱序深度 < 10 包 | +140ms | 极差网络 |
| 300ms | 几乎所有乱序 | +240ms | 只适合单向直播 |

**权衡**：增大 jitter buffer 会增加端到端延迟。对交互式通话，总延迟应 < 400ms（ITU-T G.114）。

### 2.4 优化效果预估矩阵

当前基线（无优化）→ 逐步叠加各优化后的预期效果：

| 优化步骤 | 有效丢包率 | 帧丢失率(30fps) | 额外延迟 | MOS |
|---------|-----------|----------------|---------|-----|
| 无优化 | ~2% | ~5.9% | 0 | 3.6 |
| +JB 120ms | ~0.5% | ~1.5% | +60ms | 3.9 |
| +JB + Opus FEC | ~0.3% | ~1.5% | +60ms | 4.0 |
| +JB + FEC + FlexFEC 30% | ~0.1% | ~0.3% | +60ms | 4.1 |
| +以上 + Simulcast | ~0.1% | ~0.1%(自动降级) | +60ms | 4.2 |
| +TURN Relay(消除乱序) | ~0% | ~0% | +10-30ms | 4.3 |

---

## 3. TURN Relay 实施指南

### 3.1 方案原理

```
当前路径（ECMP 乱序）:
  Client ──[ECMP Hop3-4]──[国际出口]──[ECMP Hop9]── Server
                ⚠ per-packet 负载均衡

TURN Relay 方案:
  Client ──[本地网络]── TURN Server ──[专线/优化路由]── Server
                ✓ 单一路径，无 ECMP
```

TURN 服务器在 Client 和 Server 之间中继所有 UDP 流量。如果 TURN 到 Server 之间的路径没有 ECMP（例如同机房或走专线），就能从根本上消除乱序。

### 3.2 TURN Server 选型

| 方案 | 优势 | 劣势 | 成本 |
|------|------|------|------|
| **coturn** (开源) | 免费、功能完整、生产级 | 需自运维 | 服务器成本 |
| **Twilio TURN** | 全球节点、零运维 | 按流量计费 | ~$0.40/GB |
| **Cloudflare TURN** | 全球 300+ 节点 | 需 Cloudflare 账号 | 免费层可用 |
| **自建 + cloud provider** | 灵活选择节点位置 | 需部署维护 | VPS 成本 |

**推荐**：先用 coturn 自建验证效果，确认有效后再决定长期方案。

### 3.3 coturn 部署步骤

#### Step 1: 选择 TURN 服务器位置

关键原则：**TURN 到目标服务器的路径上不能有 ECMP**

| 位置方案 | 适用场景 |
|---------|---------|
| 与目标同机房/同 VPC | 最佳 — TURN↔Server 延迟 < 1ms |
| 与目标同运营商同城市 | 好 — 避免跨运营商 ECMP |
| 目标所在国家的云服务器 | 可接受 — 避免国际段 ECMP |

根据诊断数据，目标 `101.46.59.52` 在 AS136907，建议 TURN 服务器部署在**同 AS 或同地区**。

#### Step 2: 安装 coturn

```bash
# Ubuntu/Debian
apt update && apt install -y coturn

# CentOS/RHEL
yum install -y coturn

# Docker (推荐)
docker run -d \
  --name coturn \
  --network host \
  -v /etc/turnserver.conf:/etc/turnserver.conf \
  coturn/coturn
```

#### Step 3: 配置 coturn

```bash
# /etc/turnserver.conf

# 基础配置
listening-port=3478
tls-listening-port=5349
listening-ip=0.0.0.0
relay-ip=<TURN_SERVER_PUBLIC_IP>
external-ip=<TURN_SERVER_PUBLIC_IP>

# 认证
lt-cred-mech
user=probex:SecurePassword123
realm=turn.yourdomain.com

# TLS 证书 (推荐)
cert=/etc/ssl/certs/turn.pem
pkey=/etc/ssl/private/turn.key

# 安全限制
no-multicast-peers
no-cli
no-tcp-relay

# 带宽限制 (根据服务器带宽调整)
total-quota=100         # 最大同时用户数
bps-capacity=50000000   # 50Mbps 总带宽

# 日志
log-file=/var/log/turnserver.log
verbose

# 端口范围 (确保防火墙开放)
min-port=49152
max-port=65535
```

#### Step 4: 防火墙规则

```bash
# TURN 服务器需要开放:
# TCP/UDP 3478  — TURN 主端口
# TCP 5349      — TURN over TLS
# UDP 49152-65535 — 中继端口范围

# iptables
iptables -A INPUT -p tcp --dport 3478 -j ACCEPT
iptables -A INPUT -p udp --dport 3478 -j ACCEPT
iptables -A INPUT -p tcp --dport 5349 -j ACCEPT
iptables -A INPUT -p udp --dport 49152:65535 -j ACCEPT

# 或 ufw
ufw allow 3478/tcp
ufw allow 3478/udp
ufw allow 5349/tcp
ufw allow 49152:65535/udp
```

#### Step 5: 验证 TURN 服务

```bash
# 使用 turnutils_uclient 测试
turnutils_uclient -u probex -w SecurePassword123 <TURN_SERVER_IP>

# 或使用 Trickle ICE 在线工具
# https://webrtc.github.io/samples/src/content/peerconnection/trickle-ice/
# 输入 turn:<TURN_SERVER_IP>:3478 和凭证，检查是否能获得 relay 候选
```

#### Step 6: WebRTC 客户端配置

```javascript
const rtcConfig = {
  iceServers: [
    {
      urls: [
        'turn:<TURN_SERVER_IP>:3478?transport=udp',
        'turn:<TURN_SERVER_IP>:3478?transport=tcp',
        'turns:<TURN_SERVER_IP>:5349?transport=tcp',
      ],
      username: 'probex',
      credential: 'SecurePassword123',
    },
  ],
  // 关键：强制使用 TURN relay，不走直连
  iceTransportPolicy: 'relay',
  // 如果只想在直连质量差时回退到 TURN：
  // iceTransportPolicy: 'all',  // 让 ICE 自动选择
};

const pc = new RTCPeerConnection(rtcConfig);
```

#### Step 7: 验证效果

部署后，通过 ProbeX 对比：

```
TURN 前: 对目标 iperf3 UDP 测试 → OoO ~54%
TURN 后: 对 TURN 服务器 iperf3 UDP 测试 → OoO 应 < 5%
```

同时通过 WebRTC Stats 对比：
```javascript
// 观察 candidate-pair 类型
stats.forEach(report => {
  if (report.type === 'candidate-pair' && report.nominated) {
    console.log('Type:', report.remoteCandidateId); // 应显示 relay
    console.log('RTT:', report.currentRoundTripTime * 1000, 'ms');
  }
});
```

### 3.4 TURN 方案的代价

| 项目 | 影响 |
|------|------|
| 延迟 | +10-50ms（取决于 TURN 位置） |
| 带宽成本 | 所有媒体流经 TURN，带宽翻倍消耗 |
| 单点故障 | TURN 挂了 = 通话断了（需高可用部署） |
| 运维 | 需要监控 TURN 服务器健康、带宽、连接数 |

### 3.5 高可用部署

```
                    ┌─── TURN A (主) ───┐
Client ── DNS 轮询 ──┤                    ├── Server
                    └─── TURN B (备) ───┘
```

```javascript
// 配置多个 TURN 服务器，ICE 会自动选择可用的
iceServers: [
  {
    urls: 'turn:turn-a.yourdomain.com:3478',
    username: 'probex', credential: '...',
  },
  {
    urls: 'turn:turn-b.yourdomain.com:3478',
    username: 'probex', credential: '...',
  },
],
```

---

## 4. 时段波动监测方案

### 4.1 为什么不同时段表现不同

| 时段 | 可能的影响 |
|------|----------|
| 工作日白天 9:00-18:00 | 企业流量高峰，运营商链路繁忙 |
| 晚间 19:00-23:00 | 居民视频流量高峰（Netflix/YouTube/直播） |
| 凌晨 2:00-6:00 | 流量最低，通常质量最好 |
| 月末/季末 | 部分运营商结算周期，可能调整路由 |

### 4.2 ProbeX 7×24 持续监测方案

建议创建以下任务组（已可通过 ProbeX Web UI 配置）：

#### 核心任务

| 任务 | 类型 | 间隔 | 目的 |
|------|------|------|------|
| ICMP Baseline | icmp | 10s | 延迟/抖动/丢包基线 |
| UDP Reorder - 20M | iperf3 (UDP, -R, 20M) | 10min | 乱序率趋势 |
| UDP Reorder - 5M | iperf3 (UDP, -R, 5M) | 10min | 低负载对比 |
| TCP Throughput | iperf3 (TCP, -R) | 15min | TCP 重传率对比 |
| HTTP Latency | http | 30s | 应用层延迟 |

#### 为什么用两个不同带宽的 UDP 测试

对比 5M 和 20M 的乱序率变化趋势：
- **两者同步波动** → 路径级问题（ECMP 策略变化）
- **20M 波动大，5M 稳定** → 拥塞相关（高带宽时触发更多乱序）
- **两者都在特定时段恶化** → 运营商链路容量不足

### 4.3 自动化定时诊断

使用 ProbeX 诊断脚本设置定时全面检查：

```bash
# crontab -e
# 每天 6 个时段运行完整诊断（覆盖低谷和高峰）
0 3 * * * cd /path/to/probex && sudo ./scripts/reorder-diagnosis.sh 101.46.59.52 60020 20M >> /var/log/probex-diagnosis.log 2>&1
0 9 * * * cd /path/to/probex && sudo ./scripts/reorder-diagnosis.sh 101.46.59.52 60020 20M >> /var/log/probex-diagnosis.log 2>&1
0 12 * * * cd /path/to/probex && sudo ./scripts/reorder-diagnosis.sh 101.46.59.52 60020 20M >> /var/log/probex-diagnosis.log 2>&1
0 15 * * * cd /path/to/probex && sudo ./scripts/reorder-diagnosis.sh 101.46.59.52 60020 20M >> /var/log/probex-diagnosis.log 2>&1
0 19 * * * cd /path/to/probex && sudo ./scripts/reorder-diagnosis.sh 101.46.59.52 60020 20M >> /var/log/probex-diagnosis.log 2>&1
0 22 * * * cd /path/to/probex && sudo ./scripts/reorder-diagnosis.sh 101.46.59.52 60020 20M >> /var/log/probex-diagnosis.log 2>&1
```

### 4.4 时段分析维度

收集一周数据后，按以下维度分析：

#### 热力图分析

将一周的乱序率数据按 **小时 × 星期** 绘制热力图：

```
        Mon   Tue   Wed   Thu   Fri   Sat   Sun
 0:00   12%   11%   13%   10%   11%   8%    7%
 3:00    5%    6%    5%    5%    4%    3%    3%
 6:00   15%   14%   16%   13%   14%   6%    5%
 9:00   45%   52%   48%   55%   50%   20%   15%
12:00   50%   48%   52%   49%   53%   25%   18%
15:00   55%   53%   50%   56%   48%   22%   16%
18:00   52%   55%   54%   51%   45%   30%   25%
21:00   48%   50%   52%   48%   42%   35%   30%
```

#### 关联分析

将乱序率与以下指标做相关性分析：

| 相关指标 | 如何获取 | 判断 |
|---------|---------|------|
| 延迟变化 | ProbeX ICMP 任务 | 延迟增大时乱序是否增大 |
| 路径变化 | 定期 traceroute | 路径切换时乱序是否突变 |
| 带宽利用率 | iperf3 TCP 测试 | 可用带宽降低时乱序是否增大 |

### 4.5 告警规则

| 指标 | 条件 | 级别 | 动作 |
|------|------|------|------|
| 乱序率 | > 30% 持续 10min | WARNING | 通知 |
| 乱序率 | > 60% 持续 10min | CRITICAL | 通知 + 考虑切换到 TURN |
| 丢包率 | > 2% 持续 5min | WARNING | 通知 |
| 延迟 | > 300ms 持续 5min | WARNING | 通知 |
| 路径变化 | traceroute 跳数或 IP 变化 | INFO | 记录，与乱序关联分析 |

### 4.6 长期趋势追踪

每周生成一份自动报告，包含：

1. **乱序率周均值/峰值/最低值**及与上周对比
2. **最差时段**和**最佳时段**的分布
3. **路径变化记录**（如果 traceroute 发现新路径）
4. **与运营商沟通的依据数据**（带时间戳的量化证据）

---

## 附录：指标参考标准

| 指标 | 优秀 | 良好 | 可接受 | 差 |
|------|------|------|-------|---|
| RTT | < 100ms | < 200ms | < 400ms | > 400ms |
| Jitter | < 10ms | < 30ms | < 50ms | > 50ms |
| 丢包率 | < 0.1% | < 1% | < 5% | > 5% |
| 乱序率 | < 1% | < 5% | < 15% | > 15% |
| MOS | > 4.0 | > 3.5 | > 3.0 | < 3.0 |
| 视频帧丢失率 | < 1% | < 5% | < 10% | > 10% |

来源: ITU-T G.107, G.114, RFC 4737

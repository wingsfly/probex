# ProbeX 分布式容器化监测方案设计

## 一、方案概述

### 目标架构

```
                          ┌──────────────────────────────┐
                          │     Hub (控制/汇聚节点)       │
                          │                              │
                          │  ┌────────┐  ┌────────────┐  │
                          │  │ SQLite │  │ Web UI     │  │
                          │  │ (TSDB) │  │ 多节点视图  │  │
                          │  └────────┘  └────────────┘  │
                          │       ▲                      │
                          │       │  PocketBase API      │
                          │  ┌────┴──────────────────┐   │
                          │  │ Hub Core (Go)         │   │
                          │  │ - 节点管理            │   │
                          │  │ - 任务/探针下发       │   │
                          │  │ - 数据聚合           │   │
                          │  │ - 告警评估           │   │
                          │  └──────────────────────┘   │
                          │       ▲           ▲          │
                          └───────┼───────────┼──────────┘
                   WebSocket/SSH  │           │  WebSocket/SSH
                   (TLS+ED25519) │           │  (TLS+ED25519)
                          ┌──────┴───┐ ┌─────┴────┐
                          │ Agent A  │ │ Agent B  │ ...
                          │ (Docker) │ │ (Docker) │
                          │          │ │          │
                          │ 本地探测  │ │ 本地探测  │
                          │ 本地Web  │ │ 本地Web  │
                          │ 脚本执行  │ │ 脚本执行  │
                          └──────────┘ └──────────┘
```

### 核心设计原则

1. **Agent 主动连接 Hub**（WebSocket dial-out）——Agent 在 NAT/防火墙后也能工作，无需暴露端口
2. **Hub 下发任务和脚本**——Agent 不需要本地配置文件，即插即用
3. **双向数据流**——Hub 推任务给 Agent，Agent 推结果给 Hub
4. **Agent 本地也可查看**——每个 Agent 自带轻量 Web UI，断网时仍可用
5. **安全通信**——ED25519 密钥对 + TLS，参考 Beszel 的安全模型

---

## 二、行业方案调研对比

### 通信模型对比

| 方案 | 通信方向 | 协议 | 安全 | 任务下发 | Docker 部署 |
|------|---------|------|------|---------|------------|
| **Beszel** | Agent→Hub (WS) + Hub→Agent (SSH fallback) | WebSocket + SSH | ED25519 密钥对 | 不支持（只采集系统指标） | Hub + Agent 两个镜像 |
| **Netdata** | Child→Parent (TCP streaming) | 自定义二进制 + LZ4 | API Key + TLS | 不支持（本地配置） | 同一镜像不同角色 |
| **Grafana Alloy** | Agent→Backend (HTTP push) | Prometheus Remote Write / OTLP | TLS / mTLS | 不支持（本地配置文件） | 单镜像 + 配置挂载 |
| **Prometheus** | Server→Exporter (HTTP pull) | HTTP scrape | TLS / BasicAuth | 不支持（Server 侧配置） | Server + Exporter 两镜像 |
| **Uptime Kuma** | Hub 主动探测（无 Agent） | HTTP/TCP/Ping | N/A | N/A | 单镜像 |

### 关键发现

1. **没有一个现有方案支持从 Hub 下发自定义探针/脚本到 Agent**——这是 ProbeX 的差异化能力
2. **Beszel 的 WebSocket + SSH 双通道** 是最适合参考的通信模型——Agent 主动连接（穿 NAT），SSH 作为 fallback
3. **PocketBase（Beszel 使用）** 提供了免费的 REST API + 实时订阅 + 用户认证，但我们已有自己的 API 层，不需要换
4. **Netdata 的分层聚合（1min→10min→2hr→8hr）** 是我们应该采用的数据保留策略
5. **CBOR 编码**（Beszel）比 JSON 紧凑 30-50%，但增加了复杂度——我们暂用 JSON，后续可优化

### 我们的选择：借鉴 Beszel 通信 + 增加任务下发能力

```
Beszel 做的:  Hub ←── metrics ── Agent   (Agent 只采集固定指标)
我们要做的:   Hub ── tasks/scripts ──→ Agent  (Hub 下发什么，Agent 执行什么)
              Hub ←── results ────── Agent  (Agent 回传结果)
```

---

## 三、通信协议设计

### 3.1 连接建立流程

```
Agent 启动
  │
  ├─ 1. 生成 Agent ID (UUID) 或从持久化恢复
  │
  ├─ 2. WebSocket 连接 Hub
  │     GET /api/v1/ws/agent?token=<TOKEN>&agent_id=<ID>&name=<NAME>&labels=<JSON>
  │     Upgrade: websocket
  │
  ├─ 3. Hub 验证 Token
  │     ├─ 成功: 101 Switching Protocols → 注册/更新 Agent → 发送 SyncMessage
  │     └─ 失败: 401 → Agent 等待重试
  │
  ├─ 4. Hub 下发 SyncMessage (全量任务+脚本清单)
  │     {
  │       "type": "sync",
  │       "tasks": [...],
  │       "scripts": [{"name":"rtp-sim","content":"#!/bin/bash\n...","hash":"sha256:..."}]
  │     }
  │
  ├─ 5. Agent 对比本地任务/脚本，增删改
  │
  └─ 6. 进入双向消息循环
        Hub→Agent: TaskUpdate, ScriptUpdate, TaskDelete, Ping
        Agent→Hub: ProbeResult, Heartbeat, Pong
```

### 3.2 消息类型

```
// Hub → Agent
TaskSync      全量任务同步（连接建立时）
TaskUpdate    单个任务创建/更新
TaskDelete    删除任务
ScriptSync    全量脚本同步
ScriptUpdate  单个脚本推送
Ping          心跳检测

// Agent → Hub
ResultBatch   批量探测结果（每 5-10 秒一批）
Heartbeat     心跳回复 + Agent 状态（CPU/Mem/探针数）
Pong          Ping 回复
```

### 3.3 安全模型

```
Token 认证:
  - Hub 启动时生成随机 Token（可在 UI 中查看/轮换）
  - Agent 通过 Token 连接 Hub
  - WebSocket 建议运行在 TLS 上（wss://）

脚本安全:
  - Hub 下发的脚本有 SHA256 哈希校验
  - Agent 本地缓存脚本到 /data/scripts/，按哈希判断是否需要更新
  - Agent 可配置是否接受脚本下发（安全敏感环境可关闭）
```

### 3.4 断线重连与数据缓冲

```
Agent 断线时:
  ├─ 探测任务继续执行（不依赖 Hub 连接）
  ├─ 结果写入本地 SQLite 缓冲
  ├─ 本地 Web UI 仍可查看
  └─ 指数退避重连（1s → 2s → 4s → ... → 60s max）

重连后:
  ├─ Hub 重新发送 SyncMessage
  ├─ Agent 回放缓冲的结果（带原始时间戳）
  └─ Hub 幂等处理（按 result ID 去重）
```

---

## 四、Docker 部署架构

### 4.1 镜像设计

```
probex/hub:latest        ← 控制/汇聚节点（~20MB）
probex/agent:latest      ← 监测节点（~15MB）
```

两个镜像使用同一个 Go 二进制（通过子命令区分），但打包不同的默认配置。

### 4.2 Hub 部署

```yaml
# docker-compose.hub.yml
services:
  probex-hub:
    image: probex/hub:latest
    ports:
      - "8080:8080"          # Web UI + API
    volumes:
      - ./hub-data:/data     # SQLite + 报告 + 配置持久化
    environment:
      - PROBEX_TOKEN=<生成的安全Token>   # Agent 连接凭证
    restart: unless-stopped
```

### 4.3 Agent 部署（监测节点）

```yaml
# docker-compose.agent.yml
services:
  probex-agent:
    image: probex/agent:latest
    network_mode: host        # 需要宿主网络才能准确探测
    ports:
      - "8081:8081"           # Agent 本地 Web UI (可选)
    volumes:
      - ./agent-data:/data    # 本地缓冲 + 脚本缓存
    environment:
      - PROBEX_HUB_URL=wss://hub.example.com:8080/api/v1/ws/agent
      - PROBEX_TOKEN=<与Hub相同的Token>
      - PROBEX_AGENT_NAME=beijing-node-1
      - PROBEX_AGENT_LABELS=region=cn-north,isp=telecom
    restart: unless-stopped
```

### 4.4 单机一键部署（Hub + Agent 同机）

```yaml
# docker-compose.yml
services:
  hub:
    image: probex/hub:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    environment:
      - PROBEX_TOKEN=mysecrettoken

  agent:
    image: probex/agent:latest
    network_mode: host
    environment:
      - PROBEX_HUB_URL=ws://host.docker.internal:8080/api/v1/ws/agent
      - PROBEX_TOKEN=mysecrettoken
      - PROBEX_AGENT_NAME=local
    depends_on:
      - hub
```

### 4.5 Dockerfile 设计

```dockerfile
# Dockerfile (多阶段构建)
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /probex ./cmd/probex

FROM alpine:3.20
RUN apk add --no-cache ca-certificates iperf3 bash python3
COPY --from=builder /probex /usr/local/bin/probex
COPY scripts/probes /etc/probex/scripts/
EXPOSE 8080 8081
ENTRYPOINT ["probex"]
```

```
probex hub    --config /data/hub.yaml      # Hub 模式
probex agent  --hub wss://... --token ...  # Agent 模式
```

---

## 五、后端技术架构

### 5.1 Hub 核心模块

```
cmd/probex/
  ├── main.go           # 统一入口，hub/agent 子命令
  ├── hub.go            # Hub 启动逻辑
  └── agent.go          # Agent 启动逻辑

internal/
  ├── hub/
  │   ├── server.go       # HTTP + WebSocket 服务
  │   ├── wsmanager.go    # WebSocket 连接管理（Agent 注册/消息路由）
  │   ├── taskdist.go     # 任务分发（Agent selector 匹配 → 推送）
  │   ├── scriptdist.go   # 脚本分发（检测变更 → 推送）
  │   └── aggregator.go   # 多节点数据聚合 + 分层降采样
  │
  ├── agent/
  │   ├── client.go       # WebSocket 客户端（连接 Hub）
  │   ├── executor.go     # 任务执行器（复用现有 Runner）
  │   ├── scriptmgr.go    # 脚本缓存管理（哈希校验 + 本地存储）
  │   ├── buffer.go       # 离线结果缓冲（SQLite WAL）
  │   └── localui.go      # Agent 本地 Web UI 服务
  │
  ├── protocol/
  │   ├── message.go      # 消息类型定义（JSON 序列化）
  │   └── codec.go        # 编码/解码（JSON，未来可切 CBOR）
  │
  ├── probe/              # 现有探针框架（不变）
  ├── store/              # 现有存储层（不变）
  ├── api/                # 现有 REST API（Hub 复用）
  └── alert/              # 现有告警系统（Hub 复用）
```

### 5.2 WebSocket 消息协议

```go
// internal/protocol/message.go

type MessageType string

const (
    // Hub → Agent
    MsgTaskSync     MessageType = "task_sync"
    MsgTaskUpdate   MessageType = "task_update"
    MsgTaskDelete   MessageType = "task_delete"
    MsgScriptSync   MessageType = "script_sync"
    MsgScriptUpdate MessageType = "script_update"
    MsgPing         MessageType = "ping"

    // Agent → Hub
    MsgResultBatch  MessageType = "result_batch"
    MsgHeartbeat    MessageType = "heartbeat"
    MsgPong         MessageType = "pong"
)

type Message struct {
    Type    MessageType     `json:"type"`
    Payload json.RawMessage `json:"payload"`
}

// Hub → Agent payloads
type TaskSyncPayload struct {
    Tasks   []*model.Task   `json:"tasks"`
    Scripts []ScriptInfo     `json:"scripts"`
}

type ScriptInfo struct {
    Name    string `json:"name"`
    Hash    string `json:"hash"`     // sha256
    Content string `json:"content"`  // 脚本内容（仅在需要更新时填充）
}

// Agent → Hub payloads
type ResultBatchPayload struct {
    AgentID string                `json:"agent_id"`
    Results []*model.ProbeResult  `json:"results"`
}

type HeartbeatPayload struct {
    AgentID    string   `json:"agent_id"`
    Uptime     int64    `json:"uptime_sec"`
    TaskCount  int      `json:"task_count"`
    CPUPercent float64  `json:"cpu_pct"`
    MemMB      float64  `json:"mem_mb"`
    BufferedResults int `json:"buffered_results"` // 离线缓冲中的结果数
}
```

### 5.3 数据分层聚合（参考 Beszel + Netdata）

```
原始数据 (每次探测一条)
  │
  │ 保留 1 小时
  ▼
1 分钟聚合 (avg/min/max/p95/count per minute)
  │
  │ 保留 24 小时
  ▼
10 分钟聚合
  │
  │ 保留 7 天
  ▼
1 小时聚合
  │
  │ 保留 30 天
  ▼
8 小时聚合
  │
  │ 保留 1 年
  ▼
丢弃
```

Hub 后台定时任务（每分钟运行一次）执行聚合和清理。

### 5.4 Agent 本地能力

Agent 不只是一个数据采集器，它是一个**精简版 Hub**：

```
Agent 本地:
  ├── SQLite（存储近期结果 + 离线缓冲）
  ├── Runner（执行探测任务）
  ├── Script Manager（管理从 Hub 下发的脚本）
  ├── Alert Evaluator（可选，本地告警）
  └── Mini Web UI（端口 8081）
       ├── 本节点探测结果图表
       ├── 本节点任务列表
       ├── 连接状态（Hub 连通性）
       └── Agent 状态（CPU/Mem/探针数）
```

---

## 六、前端可视化设计

### 6.1 技术选型

| 层面 | 选型 | 理由 |
|------|------|------|
| UI 框架 | React（现有） | 已有代码基础，不换框架 |
| 标准图表 | Recharts（现有） | 折线图、面积图、柱状图 |
| 高级图表 | Apache ECharts (echarts-for-react) | 热力图、仪表盘、地理地图 |
| 实时推送 | SSE (Server-Sent Events) | 单向推送，自动重连，比 WebSocket 简单 |
| CSS | 内联样式（现有风格） | 保持现有代码一致性 |

### 6.2 页面结构

```
Hub Web UI 导航:
  ├── Overview     总览（多节点状态网格 + 关键指标卡片）
  ├── Nodes        节点管理（列表 + 拓扑图）
  ├── Tasks        任务管理（现有，增加节点选择器）
  ├── Probes       探针目录（现有）
  ├── Results      结果查看（现有，增加节点筛选）
  ├── Heatmap      热力图（时间 × 节点 × 指标）
  ├── Reports      报告（现有）
  ├── Alerts       告警（现有，增加节点维度）
  └── Settings     设置（Token 管理、数据保留策略）
```

### 6.3 Overview 总览页（核心新页面）

```
┌─────────────────────────────────────────────────────────┐
│  ProbeX Dashboard                            5 Nodes ●  │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐      │
│  │ 节点总数 │ │ 在线    │ │ 告警中   │ │ 离线    │      │
│  │    5    │ │   4  ● │ │   1  ▲ │ │   1  ○ │      │
│  └─────────┘ └─────────┘ └─────────┘ └─────────┘      │
│                                                         │
│  ┌─── 节点状态网格 ───────────────────────────────────┐  │
│  │                                                    │  │
│  │  ┌──────────────┐  ┌──────────────┐                │  │
│  │  │ beijing-1  ● │  │ shanghai-1 ● │                │  │
│  │  │ Latency ▁▃▂▄ │  │ Latency ▁▁▂▁ │                │  │
│  │  │ 45ms  OK 98% │  │ 32ms  OK 99% │                │  │
│  │  │ MOS: 4.2  ▲  │  │ MOS: 4.4     │                │  │
│  │  └──────────────┘  └──────────────┘                │  │
│  │  ┌──────────────┐  ┌──────────────┐                │  │
│  │  │ guangzhou ●  │  │ hongkong  ▲  │                │  │
│  │  │ Latency ▁▂▅▃ │  │ Latency ▅█▇▅ │                │  │
│  │  │ 28ms  OK 99% │  │ 120ms WARN   │                │  │
│  │  │ MOS: 4.3     │  │ MOS: 3.2  !  │                │  │
│  │  └──────────────┘  └──────────────┘                │  │
│  │                                                    │  │
│  └────────────────────────────────────────────────────┘  │
│                                                         │
│  ┌─── 关键指标趋势（所有节点叠加）──────────────────────┐  │
│  │                                                    │  │
│  │  Latency (ms)     Packet Loss (%)    MOS Score     │  │
│  │  ┌────────────┐   ┌────────────┐   ┌────────────┐ │  │
│  │  │  ╱\  /\    │   │            │   │ ────────── │ │  │
│  │  │ ╱  \/  \   │   │    ╱╲      │   │  ─── ──── │ │  │
│  │  │╱        \  │   │ __╱  ╲__   │   │  ────────  │ │  │
│  │  └────────────┘   └────────────┘   └────────────┘ │  │
│  │  — beijing  — shanghai  — guangzhou  — hongkong   │  │
│  │                                                    │  │
│  └────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 6.4 Nodes 节点管理页

```
┌─────────────────────────────────────────────────────┐
│  Nodes                              [+ Add Node]    │
├─────────────────────────────────────────────────────┤
│                                                     │
│  View:  [Grid]  [Table]  [Map]                      │
│                                                     │
│  Table 视图:                                         │
│  ┌────────────────────────────────────────────────┐  │
│  │ Name        │ Region  │ Status │ Tasks│ Uptime │  │
│  │ beijing-1   │ cn-n    │ ● OK   │ 5    │ 3d 2h │  │
│  │ shanghai-1  │ cn-e    │ ● OK   │ 4    │ 5d 8h │  │
│  │ hongkong    │ ap-se   │ ▲ WARN │ 3    │ 1d 4h │  │
│  │ us-west     │ us-w    │ ○ OFF  │ 0    │ -     │  │
│  └────────────────────────────────────────────────┘  │
│                                                     │
│  Map 视图 (React-Leaflet / ECharts geo):             │
│  ┌────────────────────────────────────────────────┐  │
│  │                    ○ us-west                   │  │
│  │                          ● beijing             │  │
│  │                         ● shanghai             │  │
│  │                        ● guangzhou             │  │
│  │                         ▲ hongkong             │  │
│  │  ● = healthy  ▲ = warning  ○ = offline         │  │
│  └────────────────────────────────────────────────┘  │
│                                                     │
│  点击节点 → 节点详情页（该节点所有任务+结果+探针）    │
│                                                     │
└─────────────────────────────────────────────────────┘
```

### 6.5 Heatmap 热力图页（新页面，用 ECharts）

```
┌─────────────────────────────────────────────────────┐
│  Heatmap                                            │
├─────────────────────────────────────────────────────┤
│                                                     │
│  Metric: [Latency ▼]  Period: [Last 24h ▼]          │
│                                                     │
│  ┌────────────────────────────────────────────────┐  │
│  │        00  03  06  09  12  15  18  21  (hour)  │  │
│  │ beijing  ■  ■  □  ■  ■  ■  ■  ■  ← 热力色块   │  │
│  │ shanghai ■  ■  □  ■  ■  ■  ■  ■               │  │
│  │ guangz   ■  ■  □  ■  ■  ■  ■  □               │  │
│  │ hongkong ■  ■  □  ■  ■  ■  ■  ■               │  │
│  │                                                │  │
│  │  □ 好(<50ms)  ■ 中(50-200ms)  ■ 差(>200ms)    │  │
│  └────────────────────────────────────────────────┘  │
│                                                     │
│  点击色块 → 弹出该时段该节点的详细数据               │
│                                                     │
└─────────────────────────────────────────────────────┘
```

### 6.6 节点详情页

```
┌─────────────────────────────────────────────────────┐
│  ← Nodes / beijing-1                               │
│  Region: cn-north  ISP: telecom  Status: ● Online   │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐  │
│  │ Tasks   │ │ Uptime  │ │ Avg RTT │ │ MOS     │  │
│  │    5    │ │  99.8%  │ │  42ms   │ │  4.2    │  │
│  └─────────┘ └─────────┘ └─────────┘ └─────────┘  │
│                                                     │
│  ┌─── 该节点的 Results 图表（复用 Results 组件）───┐  │
│  │ [自动按该节点 agent_id 过滤]                    │  │
│  └─────────────────────────────────────────────────┘  │
│                                                     │
│  ┌─── 该节点的任务列表 ───────────────────────────┐  │
│  │ ICMP Ping        → 101.46.59.52    5s   Active │  │
│  │ RTP Sim          → 101.46.59.52    30s  Active │  │
│  │ HTTP Homepage    → guidex-ap...    30s  Active │  │
│  └─────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

### 6.7 实时数据推送

```
方案: SSE (Server-Sent Events)

Hub 端:
  GET /api/v1/stream/results?node=<node_id>
  Response: text/event-stream

  data: {"type":"result","payload":{...}}
  data: {"type":"node_status","payload":{"id":"beijing-1","status":"healthy"}}
  data: {"type":"alert","payload":{"rule":"High Latency","state":"firing"}}

前端:
  const es = new EventSource('/api/v1/stream/results');
  es.onmessage = (e) => {
    const msg = JSON.parse(e.data);
    // 更新 React Query 缓存或直接更新 state
  };
```

---

## 七、开发路线

### Phase 1: WebSocket 通信层（基础设施）

**目标**: Hub 和 Agent 通过 WebSocket 建立连接、双向通信

| 任务 | 文件 | 说明 |
|------|------|------|
| 消息协议定义 | `internal/protocol/message.go` | 所有消息类型和 payload |
| Hub WS 管理器 | `internal/hub/wsmanager.go` | 接受 Agent 连接、维护连接池、消息路由 |
| Agent WS 客户端 | `internal/agent/client.go` | 连接 Hub、断线重连、消息收发 |
| Hub WS 路由 | `internal/api/handler_ws.go` | `/api/v1/ws/agent` 端点 |

### Phase 2: 任务/脚本分发

**目标**: Hub 能将任务和脚本推送到目标 Agent

| 任务 | 文件 | 说明 |
|------|------|------|
| 任务分发逻辑 | `internal/hub/taskdist.go` | Agent selector 匹配 + 推送 |
| 脚本分发逻辑 | `internal/hub/scriptdist.go` | 脚本变更检测 + 推送 |
| Agent 脚本管理 | `internal/agent/scriptmgr.go` | 接收、校验、缓存脚本 |
| Agent 任务执行 | `internal/agent/executor.go` | 接收任务 → Runner 执行 |

### Phase 3: 数据回传与聚合

**目标**: Agent 结果回传 Hub，Hub 执行聚合

| 任务 | 文件 | 说明 |
|------|------|------|
| Agent 结果缓冲 | `internal/agent/buffer.go` | 离线缓冲 + 批量上报 |
| Hub 数据聚合 | `internal/hub/aggregator.go` | 分层降采样（1min→10min→1hr→8hr）|
| 聚合表结构 | `internal/store/sqlite/migrations.go` | `results_1m`, `results_10m` 等表 |

### Phase 4: Docker 化

**目标**: 一个 Dockerfile，两种模式（hub/agent）

| 任务 | 文件 | 说明 |
|------|------|------|
| 统一入口 | `cmd/probex/main.go` | `probex hub` / `probex agent` 子命令 |
| Dockerfile | `Dockerfile` | 多阶段构建 |
| Hub compose | `deploy/docker-compose.hub.yml` | Hub 部署模板 |
| Agent compose | `deploy/docker-compose.agent.yml` | Agent 部署模板 |
| 一键部署 | `deploy/docker-compose.yml` | Hub + 本地 Agent |

### Phase 5: 多节点前端

**目标**: Hub Web UI 支持多节点视图

| 任务 | 文件 | 说明 |
|------|------|------|
| Overview 页 | `web/src/pages/Overview.tsx` | 节点状态网格 + 关键指标 |
| Nodes 页 | `web/src/pages/Nodes.tsx` | 节点管理（表格/地图）|
| Heatmap 页 | `web/src/pages/Heatmap.tsx` | ECharts 热力图 |
| 节点详情页 | `web/src/pages/NodeDetail.tsx` | 单节点详情 |
| SSE 推送 | `internal/api/handler_sse.go` | 实时事件流 |
| Results 增强 | `web/src/pages/Results.tsx` | 增加节点筛选维度 |
| ECharts 集成 | `package.json` | 添加 echarts-for-react |

### Phase 6: Agent 本地 UI

**目标**: Agent 独立可用，断网时仍能查看本节点数据

| 任务 | 文件 | 说明 |
|------|------|------|
| Agent 本地 API | `internal/agent/localapi.go` | 精简版 REST API |
| Agent Web 前端 | `web/src/AgentApp.tsx` | 精简版 UI（本节点数据）|

---

## 八、与现有代码的关系

### 复用不变

- `internal/probe/*` — 全部探针框架、脚本引擎、元数据系统
- `internal/model/*` — 所有数据模型
- `internal/store/*` — SQLite 存储层（Hub 和 Agent 各自有独立 DB）
- `internal/alert/*` — 告警系统（Hub 使用）
- `internal/report/*` — 报告生成（Hub 使用）
- `web/src/pages/Tasks.tsx, Results.tsx, Probes.tsx, Alerts.tsx, Reports.tsx` — 现有页面（Hub 复用）
- `web/src/components/SchemaForm.tsx` — 动态表单

### 需要修改

- `internal/api/server.go` — 增加 WebSocket 和 SSE 路由
- `web/src/App.tsx` — 增加 Overview/Nodes/Heatmap 路由
- `cmd/` — 重组为统一入口

### 新增

- `internal/protocol/` — 通信协议
- `internal/hub/` — Hub 核心逻辑
- `internal/agent/client.go` — Agent WebSocket 客户端
- `web/src/pages/Overview.tsx, Nodes.tsx, Heatmap.tsx, NodeDetail.tsx` — 多节点页面
- `Dockerfile`, `deploy/` — 容器化部署

---

## 九、参考来源

| 系统 | 借鉴内容 |
|------|---------|
| **Beszel** | WebSocket + SSH 双通道、ED25519 安全模型、PocketBase 实时 API 思路、分层时序聚合 |
| **Netdata** | Parent-Child 流式传输、LZ4 压缩思路、per-second 采集能力、断线数据回放 |
| **Grafana** | 模板变量多节点筛选、Panel 组件化设计、Dashboard 布局模式 |
| **Prometheus** | 指标格式规范、Recording Rules 聚合思路、PromQL 启发的查询模式 |
| **Uptime Kuma** | 状态页设计、90天 uptime 条形图、简洁的单页部署模式 |

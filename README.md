# ProbeX

Distributed network quality monitoring platform. Deploy probes across your infrastructure to continuously measure latency, jitter, packet loss, throughput, DNS/TLS performance, and more.

## Features

- **Multi-mode Deployment**: Standalone (single node), Hub (central controller), or Agent (remote probe)
- **Probe Types**: ICMP ping, HTTP(S), DNS, TCP/UDP, WebRTC (via Chrome extension), Guidex digital human interaction
- **Real-time Dashboard**: Live metrics visualization with dual Y-axis charts, heatmaps, and status overview
- **Alerting**: Configurable threshold-based alerts with notification support
- **Scheduled Tasks**: Cron-based probe scheduling with concurrent task execution
- **Results & Reporting**: Historical data with custom date range filtering, Excel export (short column names + Chinese Dictionary sheet)
- **External Probe API**: Push-based integration for browser extensions and custom probes

## Architecture

```
┌──────────────────┐         gRPC          ┌──────────────────┐
│   ProbeX Agent   │ ◄───────────────────► │    ProbeX Hub    │
│  (remote probe)  │   heartbeat / poll    │  (controller)    │
└──────────────────┘                       │  - task scheduler│
                                           │  - data store    │
┌──────────────────┐         gRPC          │  - alerting      │
│   ProbeX Agent   │ ◄───────────────────► │  - aggregation   │
│  (remote probe)  │                       └────────┬─────────┘
└──────────────────┘                                │
                                                    │ HTTP API
┌──────────────────┐         HTTP POST              ▼
│ Chrome Extension │ ─────────────────────► ┌──────────────────┐
│ (external probe) │                        │    Web Frontend   │
└──────────────────┘                        │  React + TypeScript│
                                            └──────────────────┘
```

## Deployment Modes

ProbeX supports three deployment modes via a single binary. **Standalone is the default** — running `probex` without arguments is equivalent to `probex standalone`.

| Mode | Command | Description |
|------|---------|-------------|
| **Standalone** | `probex` or `probex standalone` | Single-node, runs both hub and local agent. Suitable for most scenarios. |
| **Hub** | `probex hub` | Central controller only. Accepts remote agent connections, no local probing. |
| **Agent** | `probex agent` | Remote probe node. Connects to a hub, executes probes locally. |

## Quick Start

### Binary

```bash
# Build
make build

# Start backend (standalone mode, default)
./bin/probex

# API:    http://localhost:8080/api/v1
# Health: http://localhost:8080/health
```

> Backend only provides API endpoints. To access the Web UI, you need to start the frontend separately — see [Local Development](#local-development).

### Docker (Standalone)

```bash
docker compose -f deploy/docker-compose.yml up -d
# API: http://localhost:8080/api/v1
```

Or build and run directly:

```bash
docker build -t probex .
docker run -p 8080:8080 -v probex-data:/data probex
```

> The `CMD` defaults to `standalone`. The container runs the backend API only. Start the frontend separately for the Web UI.

### Docker (Distributed: Hub + Agents)

```bash
docker compose -f deploy/docker-compose.distributed.yml up -d
```

This starts a hub + 2 agents (east/west). Configure via environment variables in `deploy/.env`:

| Variable | Default | Description |
|----------|---------|-------------|
| `PROBEX_HUB_TOKEN` | `test-token-123` | Shared auth token between hub and agents |
| `PROBEX_AGENT_EAST_NAME` | `agent-east` | Agent name / region label |
| `PROBEX_AGENT_WEST_NAME` | `agent-west` | Agent name / region label |

### Hub + Agent (Binary)

```bash
# Start hub on central server
./bin/probex hub --token my-secret-token

# Start agent(s) on remote machines
./bin/probex agent --hub ws://hub-host:8080/api/v1/ws/agent --token my-secret-token --name agent-bj --labels '{"region":"beijing"}'
```

## Local Development

### One Command

```bash
make dev
```

Starts backend (`:8080`) + Vite frontend (`:3000`) together. `Ctrl+C` stops both.

### Step by Step

```bash
# Terminal 1: backend
make dev-backend

# Terminal 2: frontend
make web-install
make dev-frontend
```

Open `http://localhost:3000` for the dev UI.

> Note: In dev mode, `http://localhost:8080` is the backend API only (returns 404 at `/`). The frontend is served by Vite on `:3000`.

## Configuration

### Hub (`configs/controller.yaml`)
- HTTP/gRPC server addresses
- SQLite storage path
- Data retention policies (default: 30 days raw, 1 year aggregated)
- Runner concurrency

### Agent (`configs/agent.yaml`)
- Agent name and region labels
- Heartbeat/poll intervals
- Controller URL

## Web Frontend

The web UI (`web/`) is built with React + TypeScript and provides:

| Page | Description |
|------|-------------|
| Dashboard | Live metrics overview |
| Nodes | Network node management |
| Probes | Probe configuration |
| Tasks | Scheduled task management |
| Results | Test results with charts, custom date range, Excel export |
| Agents | Remote agent management |
| Alerts | Alert rules and history |
| Heatmap | Visual metric heatmap |
| Reports | Analytics and reporting |

## External Probe Integration

ProbeX accepts push-based metrics from external probes (e.g. the [WebRTC Chrome Extension](https://github.com/wingsfly/probex-webrtc-guidex-extension)):

```
POST /api/v1/external/probe
Content-Type: application/json

{
  "probe_id": "webrtc-ext-001",
  "task_id": "...",
  "metrics": { ... }
}
```

## Tech Stack

- **Backend**: Go, Chi router, SQLite, gRPC, Cobra CLI
- **Frontend**: React, TypeScript, Recharts, xlsx
- **Extension**: Chrome MV3, WebRTC getStats API, WebSocket hooks

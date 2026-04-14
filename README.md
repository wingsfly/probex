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

## Quick Start

### One Command (Local Development)

```bash
make dev
```

This starts backend and frontend together.

### Step by Step

1. Start backend API (`:8080`):

```bash
make dev-backend
```

2. Start frontend UI (`:3000`) in another terminal:

```bash
make web-install
make dev-frontend
```

3. Open the UI: `http://localhost:3000`

4. Backend endpoints:
   - API: `http://localhost:8080/api/v1`
   - Health: `http://localhost:8080/health`

> Note: `http://localhost:8080` is backend-only and returns 404 at `/`. The frontend is served by Vite on `:3000` during local dev.

### Hub + Agent Mode

```bash
# Start hub
./bin/probex hub --config configs/controller.yaml

# Start agent(s) on remote machines
./bin/probex agent --config configs/agent.yaml
```

### Docker

```bash
cp deploy/.env.example deploy/.env
docker compose -f deploy/docker-compose.yml up -d
```

If you need the web UI while using Docker backend, still run:

```bash
cd web
npm install
npm run dev
```

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

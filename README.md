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

### 1. Start Backend

**Binary:**

```bash
make build
./bin/probex                # standalone mode (default), API on :8080
```

**Or Docker:**

```bash
docker compose -f deploy/docker-compose.yml up -d
```

### 2. Start Frontend

```bash
cd web
npm install
npm run dev                 # Vite dev server on :3000
```

### 3. Open Web UI

- **Web UI: http://localhost:3000**
- API: http://localhost:8080/api/v1
- Health: http://localhost:8080/health

> `make dev` can start both backend and frontend in one command — see [Local Development](#local-development).

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

ProbeX accepts push-based metrics from external probes via REST API. No authentication required.

### API Endpoints

```
# Register a probe
POST /api/v1/probes/register
{ "name": "netflow-office-gw", "description": "..." }

# Push results
POST /api/v1/probes/netflow-office-gw/push
{ "agent_id": "...", "node_id": "a3f0b12c", "results": [{ "success": true, ... }] }
```

### Node ID

Every ProbeX node generates a persistent 8-char hex ID stored in `~/.probex/node_id`. This enables the server to correlate results from the same physical machine even when user-chosen probe names collide across hosts.

### Built-in External Scripts

#### netflow-collector — NIC Flow Monitor

Monitors real-time network interface traffic (rx/tx throughput), NOT maximum bandwidth.

```bash
pip3 install psutil

# Local — auto-detect interface, 5s interval
python3 scripts/external/netflow-collector.py

# Remote hub, custom ID template
python3 scripts/external/netflow-collector.py \
  --controller http://192.168.70.101:8080 --id %i2

# Manual ID + specific interface + 3s interval
python3 scripts/external/netflow-collector.py \
  --controller http://192.168.70.101:8080 --id office-gw --iface eth0 --interval 3
```

ID template placeholders: `%h`=hostname, `%i`=IP, `%iN`=last N IP octets, `%f`=interface, `%o`=OS.

#### WebRTC Chrome Extension

See [probex-webrtc-guidex-extension](https://github.com/wingsfly/probex-webrtc-guidex-extension) for WebRTC quality monitoring and Guidex digital human interaction testing.

## Tech Stack

- **Backend**: Go, Chi router, SQLite, gRPC, Cobra CLI
- **Frontend**: React, TypeScript, Recharts, xlsx
- **External Scripts**: Python (psutil), persistent node ID
- **Chrome Extension**: MV3, WebRTC getStats API, WebSocket hooks

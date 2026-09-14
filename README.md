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

### Client Filter Labels

The Results client selector displays `Agent ID + Probe Name` while continuing
to filter by the original `agent_id`. For GuideX business probes, Probe Name
means the extension's configured WebRTC name (for example `GuideX Macbook`),
not the shared `guidex-runtime-v4` / `guidex-interaction` adapter name. It is
associated using the same agent ID in latest-per-task WebRTC results and the
registered WebRTC schema. Multiple observed names are listed together; they
are not assumed to be the current extension setting. Missing associations show
`ProbeName 未知`. Other tasks display their selected probe name. This is a UI-only
change: reporting fields, API filter parameters and stored records are unchanged.

### GuideX Runtime v4 Development

The `codex/guidex-v4-timeline` branch displays the current v4 fields only, without
old-field aliases or inferred historical milestones. Fields are **not finalized**:
clear obsolete `ext_guidex-runtime-v4` test results and their aggregates during
schema iteration, but preserve history after explicit user sign-off. This is not
scheduled or startup deletion and does not affect Legacy, WebRTC, other tasks,
or existing platform retention policies. Reload the extension and GuideX page
before collecting samples with updated fields. See `AGENTS.md` for the workflow.
The existing HK frontend release and rollback procedure is documented in
[yghk frontend deployment](deploy/yghk-frontend.md).

The current timeline uses 14 English milestones and English display values.
`Mic_Ready` is observed UI readiness relative to `start_at`, replacing separate
preparation fields without summing their offsets. The implicit `start=0` is not
reported. `Last_STT` is paired with the current `STT Text` snapshot; redundant
audio-size, upload-window, STT/model-status, and derivable duration fields are
not registered for Runtime v4. Results keeps `Mic_Ready`, `1st_Audio`,
`Speech_Started`, and `1st_STT` on the GuideX charts, and replaces later
speech-length-sensitive cumulative milestones with key intervals such as
`LastAudio_To_STT`, `LastAudio_To_Answer`, `STT_To_Answer`,
`Answer_Dur`, `LastAudio_To_Play`,
`Play_Dur`, and `Interrupt_ACK`. Labels and registration descriptions
have no numeric prefixes; `1st` still means first. Cross-stage intervals use `_To_`
between endpoints; answer and playback durations use `Answer_Dur` and `Play_Dur`.
These two duration labels retain the `answer_stream` / `avatar_speak_duration`
data keys and formulas. `Audio_To_Speech` (`audio_start_to_speech_started`) is
no longer registered, reported, or displayed; the `Speech_Started` milestone remains.
Business ordering is independent of these display names;
the table and export still retain the complete current observed fields. Results
shows the trend chart, table/pagination, then the turn timing view.
The Runtime v4 trend keeps response latency and duration metrics in the same
chart but uses independent scales: response values use the left axis, while
`Answer_Dur` and `Play_Dur` use the right axis. Sparse series connect only
their actually reported samples; missing values are not synthesized. The
client/page selector requests dimensions from the same active time range as
the chart, so an `All pages` count no longer includes pages seen only outside
the selected 1h/6h/24h/7d or custom interval.
The `LastAudio_To_*` intervals use the last successful audio append in the same
turn, not the server speech-stop notification. `LastAudio_To_STT` ends at final STT,
`LastAudio_To_Answer` at the first nonempty answer, and `LastAudio_To_Play` at the
first business playback notification. Auto-tests share this formula; the separate
`Test_To_Play` metric and all `Stop_To_*` metrics are removed. These use new
`last_audio_to_*` data keys, not aliases or relabeled historical values. Missing
anchors stay null, and zero/negative deltas are preserved.
Use extension 1.1.5 with this UI. Confirmed normal `presence_left` session
closures are successful (`Done`, `End_By=session_ended`) but excluded from
trend aggregation and the single-turn chart. Details and raw exports retain
them, including when there are no chartable turns. Their standard latency and
complete-interaction duration remain null; partial milestones are not invented.
Explicit errors, timeouts, disconnects and unknown end causes remain failures.
Both backend and frontend need deployment for this change, followed by extension
and idle GuideX page reloads. Source changes alone do not update deployed assets,
injected pages or historical records.

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
- IP access control (CIDR allowlist)
- SQLite storage path
- Data retention policies (default: 30 days raw, 1 year aggregated)
- Runner concurrency

### IP Access Control

Restrict which networks can access the API and Web UI via `allowed_networks` (CIDR notation). Empty or omitted = allow all.

```yaml
server:
  http_addr: ":8080"
  allowed_networks:
    - "192.168.70.0/24"    # office LAN
    - "10.147.20.0/24"     # VPN
    - "127.0.0.0/8"        # localhost
```

Bare IPs without mask (e.g. `"10.0.0.1"`) are treated as /32. Requests from non-matching IPs receive HTTP 403 Forbidden.

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

ProbeX accepts push-based metrics from external probes via REST API. When authentication
is enabled, use an ingest token (`X-Ingest-Token`) or an authorized login session.

### API Endpoints

```
# Register a probe
POST /api/v1/probes/register
{ "name": "netflow-office-gw", "description": "..." }

# Push results
POST /api/v1/probes/netflow-office-gw/push
{ "agent_id": "...", "node_id": "a3f0b12c", "results": [{ "success": true, ... }] }
```

### Idempotent Push

Each item in `results` may include a stable `result_id` (non-blank, at most 128
bytes). Generate it once when the sample is collected, and preserve it and its
payload on every retry. Different samples must have different IDs, even if their
timestamps or metric values are identical.

```json
{"agent_id":"browser-example","node_id":"pg-example","results":[{"result_id":"sample-unique-id","timestamp":"2026-09-10T04:27:22.989Z","success":true}]}
```

IDs are scoped by probe name, effective task ID, agent ID and node ID. The first
accepted result wins; retries do not overwrite it or evaluate alerts again.
Concurrent requests are deduplicated atomically by the database primary key,
including after backend restarts. Responses include `accepted` (new + duplicate),
`inserted` and `duplicates` counts. The guarantee lasts while the raw result is
retained; existing retention/explicit cleanup can remove its deduplication key.
This change requires no database migration and does not clean historical rows.

Clients omitting `result_id` retain the previous append-only behavior. Deploy the
backend before upgrading the browser extension. For extension 1.1.1, reload both
the extension and every monitored GuideX page; updating files alone cannot
replace old listeners in already-open pages. See the sibling extension's
`docs/reporting-delivery.md` for the rollout and offline regression checks.

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

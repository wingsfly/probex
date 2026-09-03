# ProbeX Docker Deploy

> 中文版见 [README.zh.md](README.zh.md)。

This folder supports two deployment paths:

1. Prebuilt image (recommended for users)
2. Local image build (recommended for development)

## 1) Run Without Local Build

By default, compose files pull prebuilt images and do not compile source code locally.

```bash
cp deploy/.env.example deploy/.env
docker compose -f deploy/docker-compose.yml up -d
```

This starts backend services on `:8080`.
It does not start the Vite frontend dev server on `:3000`.

Distributed mode:

```bash
cp deploy/.env.example deploy/.env
docker compose -f deploy/docker-compose.distributed.yml up -d
```

If needed, edit `deploy/.env` and set a different image tag or registry mirror.

For distributed mode, you can also configure:

- `PROBEX_HUB_TOKEN`: shared token used by hub and agents
- `PROBEX_HUB_WS_URL`: agent connect URL, e.g. `ws://<hub-host>:8080/api/v1/ws/agent`
- `PROBEX_AGENT_EAST_NAME`: agent-east display name
- `PROBEX_AGENT_WEST_NAME`: agent-west display name

## 2) Run With Local Build

Use the override file when you explicitly want to build from local source:

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.standalone.build.yml up -d --build
```

Distributed mode:

```bash
docker compose -f deploy/docker-compose.distributed.yml -f deploy/docker-compose.distributed.build.yml up -d --build
```

## 3) All-in-One Deploy (`docker-compose.probex.yml`)

`docker-compose.probex.yml` runs the backend (standalone) **and** an
nginx-served frontend together on one host. It bind-mounts `../data`,
`../configs/controller.yaml`, and `../scripts/probes` (read-only, for
script-probe hot reload).

### Environment-specific config → `docker-compose.override.yml`

Per-host mounts, secrets, and paths (an ASR data dir, SSH creds, etc.) belong in
`docker-compose.override.yml`, which is **gitignored** so it never touches the
shared compose file:

```yaml
services:
  backend:
    volumes:
      - /host/path/data:/host/path/data
      - ../configs/ssh:/root/.ssh:ro
```

Because the main file is `docker-compose.probex.yml` (not the default
`docker-compose.yml`), compose does **not** auto-load the override. You **must
pass both `-f` flags** on every command:

```bash
docker compose \
  -f deploy/docker-compose.probex.yml \
  -f deploy/docker-compose.override.yml \
  up -d --build backend frontend
```

Host-local edits to a tracked file that shouldn't be committed (e.g.
`configs/controller.yaml` with a host-specific `script_dir`) can be shielded
from `git pull` with `git update-index --skip-worktree <file>`.

### Update workflow

- **Frontend-only change** — rebuild just the frontend with `--no-deps` so the
  backend (and live monitoring) is **not** restarted:
  ```bash
  docker compose -f deploy/docker-compose.probex.yml -f deploy/docker-compose.override.yml \
    up -d --build --no-deps frontend
  ```
- **Backend change** — rebuild `backend` (this restarts it, a few seconds of
  monitoring gap). If a rebuild seems to run stale code, confirm the source is
  current (`git rev-parse HEAD`) then `build --no-cache backend`.
- The bind-mounted `../data` (sqlite db) and `../configs/controller.yaml`
  survive rebuilds.

### Script probes: hot reload

`../scripts/probes` is bind-mounted, so editing a script's **logic** takes
effect immediately (each run re-execs the file). Adding/removing a script or
changing its `PROBEX_META` metadata needs a rescan: click **Rescan Scripts** on
the Probes page, or `POST /api/v1/probes/rescan`. No restart required.

## Frontend During Local Development

If you want to use the web UI in local dev, run frontend separately:

```bash
cd web
npm install
npm run dev
```

Then open:

- Frontend: `http://localhost:3000`
- Backend API: `http://localhost:8080/api/v1`

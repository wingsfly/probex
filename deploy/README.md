# ProbeX Docker Deploy

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

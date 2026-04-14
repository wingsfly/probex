# ProbeX

ProbeX backend and frontend run as separate processes in local development.

## Quick Start (Local Development)

1. Start backend API (`:8080`):

```bash
make dev-backend
```

2. Start frontend UI (`:3000`) in another terminal:

```bash
make web-install
make dev-frontend
```

3. Open the UI:

- `http://localhost:3000`

4. Backend endpoints:

- API: `http://localhost:8080/api/v1`
- Health: `http://localhost:8080/health`

Important:

- `http://localhost:8080` is backend-only and returns `404 page not found` at `/`.
- This is expected because the frontend is served by Vite on `:3000` during local dev.

## One Command (Local)

```bash
make dev
```

This starts backend and frontend together.

## Docker Backend

Docker compose in `deploy/` starts backend services, not the Vite dev frontend.

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

.PHONY: all build probex controller agent cli clean test run hub dev dev-backend dev-frontend web-install \
        docker-up docker-down docker-restart docker-logs docker-ps docker-build docker-rebuild

# ---- Docker (all-in-one: backend + nginx frontend) ----
COMPOSE = docker compose -f deploy/docker-compose.probex.yml

all: build

build: probex

# Unified binary (recommended)
probex:
	go build -o bin/probex ./cmd/probex

# Legacy binaries (kept for backward compatibility)
controller:
	go build -o bin/probex-controller ./cmd/controller

agent:
	go build -o bin/probex-agent ./cmd/agent

cli:
	go build -o bin/probex ./cmd/cli

clean:
	rm -rf bin/ data/

test:
	go test ./...

run: probex
	./bin/probex standalone

hub: probex
	./bin/probex hub

dev-backend: probex
	./bin/probex standalone

web-install:
	cd web && npm install

dev-frontend:
	cd web && npm run dev

dev: probex
	@echo "Starting backend + frontend (Ctrl+C to stop both)..."
	@bash -lc 'set -e; \
	trap "kill 0" EXIT INT TERM; \
	./bin/probex standalone & \
	cd web && npm run dev & \
	wait'

docker-up:
	$(COMPOSE) up -d

docker-down:
	$(COMPOSE) down

docker-restart:
	$(COMPOSE) restart

docker-logs:
	$(COMPOSE) logs -f

docker-ps:
	$(COMPOSE) ps

docker-build:
	$(COMPOSE) build

docker-rebuild:
	$(COMPOSE) up -d --build

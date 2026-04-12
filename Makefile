.PHONY: all build probex controller agent cli clean test run dev

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

dev: probex
	./bin/probex standalone

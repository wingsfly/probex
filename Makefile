.PHONY: all build controller agent cli clean test run dev

all: build

build: controller agent cli

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

run: controller
	./bin/probex-controller

dev: build
	./bin/probex-controller

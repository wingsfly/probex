# ProbeX — Multi-stage Docker build
# Usage:
#   docker build -t probex .
#   docker run probex standalone   # single-node (default)
#   docker run probex hub          # distributed hub
#   docker run probex agent        # distributed agent

FROM golang:1.26.2-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /probex ./cmd/probex

FROM alpine:3.21
RUN apk add --no-cache ca-certificates iperf3 bash python3
COPY --from=builder /probex /usr/local/bin/probex
COPY scripts/probes /etc/probex/scripts/

VOLUME /data
EXPOSE 8080 8081
ENV PROBEX_SCRIPT_DIR=/etc/probex/scripts

ENTRYPOINT ["probex"]
CMD ["standalone"]

# LLMObs Platform Orchestrator

[![Go Version](https://img.shields.io/badge/go-1.21%2B-blue.svg)](https://golang.org)
[![OpenAPI Spec](https://img.shields.io/badge/OpenAPI-3.1.0-brightgreen.svg)](contracts/openapi/v1.yaml)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Open-standard, unified Go orchestration engine for the **LLM Observability & Infrastructure Platform**. Replaces legacy shell scripts with a strongly-typed, hexagonal architecture providing a unified CLI and REST API daemon.

---

## Architecture Overview

Built following **Hexagonal Architecture (Ports & Adapters)**, **Zero-Inline-Comment Doctrine**, and strict **Single Responsibility Principle (SRP)**:

```
packages/platform-orchestrator/
├── config/                  # Environment schemas & cascading YAML configurations
├── contracts/               # OpenAPI 3.1.0 specifications
├── src/
│   ├── api/rest/            # HTTP handlers and router adhering to standard envelopes
│   ├── cmd/                 # Cobra CLI commands (up, down, scale, health, config, etc.)
│   ├── features/            # Isolated business domain modules
│   │   ├── backup/          # Disaster recovery and database dumping
│   │   ├── certs/           # Pure Go self-signed X.509 certificate generation
│   │   ├── cloudflare/      # Cloudflare Tunnel ingress management
│   │   ├── config/          # Dynamic resource limits, CPU/memory & network tuning
│   │   ├── gdpr/            # GDPR/CCPA data erasure across databases
│   │   ├── health/          # Concurrent TCP/HTTP diagnostic health probes with backoff
│   │   ├── ports/           # Host port contention detection and automated resolution
│   │   ├── prereqs/         # Docker, daemon, memory, and ulimit checks
│   │   ├── scale/           # Stateless service & simulated compute node scaling
│   │   ├── setup/           # Full 7-step bootstrapping pipeline with credential prompts
│   │   └── stack/           # Profile resolution, storage, self-healing, and compose lifecycle
│   ├── infra/               # Infrastructure adapters (Docker Engine, OTel tracing)
│   └── shared/              # Hexagonal port interfaces, path resolver, and API envelopes
└── tests/                   # Unit and integration test suites
```

---

## Build & Test

Compile the orchestrator binary into the project's root `bin/` directory:

```bash
# From packages/platform-orchestrator
make build

# Run unit tests
make test

# Run linter
make lint
```

---

## CLI Command Reference

### 1. Stack Lifecycle

```bash
# Start infrastructure stack with interactive profile selector
./bin/llmobs up

# Start full stack directly (all 10 services)
./bin/llmobs up full

# Start specific profiles
./bin/llmobs up db streaming
./bin/llmobs up stateless
./bin/llmobs up stateful

# Custom Docker network parameters
./bin/llmobs up full --network-name llmobs-network --network-subnet 172.28.0.0/16 --network-gateway 172.28.0.1

# Display live status of all containers
./bin/llmobs status

# Run concurrent health diagnostics across all endpoints (sub-20ms latency probes with auto-retry)
./bin/llmobs health

# Stream logs from containers
./bin/llmobs logs
./bin/llmobs logs 100

# Restart stack
./bin/llmobs restart full

# Stop all containers across all profiles cleanly and remove orphan resources
./bin/llmobs down
```

---

### 2. Configuration & Resource Tuning (`config`)

View, tune, and persist Docker resource limits (memory, CPU, network) without editing compose files manually:

```bash
# View current configuration and limits
./bin/llmobs config

# Interactive configuration prompt
./bin/llmobs config -i

# Set specific service resource limits
./bin/llmobs config --temporal-memory 4096M --clickhouse-memory 8192M --alloydb-memory 6144M

# Apply limits and automatically restart affected containers
./bin/llmobs config --temporal-memory 4096M --restart
```

---

### 3. Platform Bootstrapping & Credentials (`setup`)

Executes the automated 7-step platform bootstrapping pipeline:

```bash
# Run interactive setup (prompts for passwords; press Enter to keep default values)
./bin/llmobs setup -i

# Run setup with automated defaults
./bin/llmobs setup

# Override specific passwords via CLI flags
./bin/llmobs setup --db-password my_secret_pw --redis-password my_redis_pw

# Verify credentials against active local databases
./bin/llmobs verify-credentials
```

---

### 4. Horizontal Scaling (`scale`)

Scale stateless services or provision simulated compute nodes:

```bash
# Interactive scaling menu
./bin/llmobs scale

# Scale a specific stateless service to N replicas
./bin/llmobs scale service llmobs-temporal 3
./bin/llmobs scale service llmobs-traefik 2

# Scale simulated compute worker nodes
./bin/llmobs scale node 1 host.docker.internal
./bin/llmobs scale node 2 host.docker.internal

# List active compute nodes
./bin/llmobs scale list

# Teardown a compute node
./bin/llmobs scale down-node 2
```

---

### 5. Disaster Recovery & Backups (`backup-purge`)

Dump databases for disaster recovery or purge volumes:

```bash
# Dump databases to timestamped backups directory without touching volumes
./bin/llmobs backup-purge --backup-only

# Perform complete backup and purge Docker persistent volumes
./bin/llmobs backup-purge
```

---

### 6. Cloudflare Tunnel Management (`cloudflare`)

Manage zero-trust ingress tunnels:

```bash
# Setup Cloudflare tunnel credentials
./bin/llmobs cloudflare setup

# Start tunnel ingress
./bin/llmobs cloudflare start

# Check tunnel status
./bin/llmobs cloudflare status

# Stream tunnel logs
./bin/llmobs cloudflare logs

# Stop tunnel ingress
./bin/llmobs cloudflare stop
```

---

### 7. Security, Certificates & Compliance

```bash
# Generate self-signed TLS certificates (server.pem, ca.pem) natively in Go
./bin/llmobs certs

# Clean up any conflicting host ports (31410–31426)
./bin/llmobs free-ports

# Execute GDPR/CCPA Right-to-Erasure across all databases
./bin/llmobs gdpr-erasure --user-id "usr_12345" --actor-id "admin_ops"
```

---

### 8. REST API Daemon (`server`)

Run the orchestrator as a background daemon exposing a fully typed HTTP REST API conforming to the OpenAPI 3.1.0 contract:

```bash
# Start API daemon on default port 31427
./bin/llmobs server

# Start API daemon on custom port
./bin/llmobs server --port 8080
```

All responses conform to the standard envelope:

```json
{
  "meta": {
    "version": "v1",
    "timestamp": "2026-09-28T13:45:00Z",
    "traceId": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
  },
  "data": {},
  "errors": []
}
```

---

## Platform Service & Port Reference

| Service | Internal Port | Host Port | Protocol | Default Credentials | Health Endpoint |
|---|---|---|---|---|---|
| **Traefik Gateway** | 80, 8080, 443 | `31410`, `31411`, `31419` | HTTP / HTTPS | — | `http://localhost:31410/ping` |
| **Redis Ledger** | 6379 | `31413` | TCP | `:llmobs_redis_s3cret_2024` | `localhost:31413` |
| **Kafka Broker** | 9092 | `31414` | TCP | — | `localhost:31414` |
| **Grafana Dashboard** | 3000 | `31415` | HTTP | `admin` / `llmobs_admin_password` | `http://localhost:31415/api/health` |
| **Grafana Tempo** | 3200, 4317 | `31416`, `31423` | HTTP / gRPC | — | `http://localhost:31416/ready` |
| **OTel Collector** | 4317, 4318 | `31418`, `31417` | gRPC / HTTP | — | `http://localhost:31417/` |
| **AlloyDB (PostgreSQL)** | 5432 | `31420` | PostgreSQL | `admin` / `llmobs_s3cret_2026` | `localhost:31420` |
| **ClickHouse Analytics** | 8123, 9000 | `31421`, `31422` | HTTP / TCP | `default` / `llmobs_clickhouse_s3cret_2026` | `http://localhost:31421/ping` |
| **Temporal Engine** | 7233, 8080 | `31424`, `31425` | gRPC / HTTP UI | — | `localhost:31424` |
| **Service Registry** | 31426 | `31426` | HTTP | — | `http://localhost:31426/health` |
| **Platform Orchestrator API** | 31427 | `31427` | HTTP | — | `http://localhost:31427/health` |

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
├── config/                  # Environment schemas & cascading YAML configurations (default.yaml)
├── contracts/               # OpenAPI 3.1.0 specifications
├── src/
│   ├── api/rest/            # HTTP handlers and router adhering to standard envelopes
│   ├── cmd/                 # Cobra CLI commands (up, down, service, datasource, dashboard, alert, health, config, etc.)
│   ├── features/            # Isolated business domain modules
│   │   ├── backup/          # Disaster recovery and database dumping
│   │   ├── certs/           # Pure Go self-signed X.509 certificate generation
│   │   ├── cloudflare/      # Cloudflare Tunnel ingress management
│   │   ├── config/          # Dynamic resource limits, CPU/memory & network tuning
│   │   ├── gdpr/            # GDPR/CCPA data erasure across databases
│   │   ├── grafana/         # Dynamic datasources, dashboards, unified alerting, and contact points
│   │   ├── health/          # Concurrent TCP/HTTP diagnostic health probes with backoff
│   │   ├── ports/           # Host port contention detection and automated resolution
│   │   ├── prereqs/         # Docker, daemon, memory, and ulimit checks
│   │   ├── scale/           # Stateless service & simulated compute node scaling
│   │   ├── services/        # Dynamic external services catalog & connectivity probe engine
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
# Build binary
cd packages/platform-orchestrator
go build -o ../../bin/llmobs ./main.go

# Run all unit test suites
go test -v ./...
```

---

## Complete CLI Command Reference

### 1. Platform Stack Lifecycle

```bash
# Start infrastructure stack with interactive profile selector
./bin/llmobs up

# Start full stack directly (all 10 core services)
./bin/llmobs up full

# Start specific profile subsets
./bin/llmobs up db streaming
./bin/llmobs up stateless
./bin/llmobs up stateful

# Custom Docker network parameters
./bin/llmobs up full --network-name llmobs-network --network-subnet 172.28.0.0/16 --network-gateway 172.28.0.1

# Display live status of all containers
./bin/llmobs status

# Stream container logs
./bin/llmobs logs
./bin/llmobs logs 100

# Restart the platform stack
./bin/llmobs restart full

# Stop all containers cleanly and remove orphan resources
./bin/llmobs down
```

---

### 2. Health Verification & Diagnostic Probes (`health`)

```bash
# Basic concurrent TCP/HTTP connectivity checks across all active endpoints
./bin/llmobs health

# Deep functional verification (PostgreSQL auth, Redis RESP, ClickHouse queries, Grafana API)
./bin/llmobs health --deep

# Target specific services
./bin/llmobs health --deep --services alloydb,clickhouse,redis,grafana

# Target specific profile groups
./bin/llmobs health --deep --profiles stateful
```

---

### 3. External Services & Custom Connections (`service`)

Dynamically register, probe, manage, and bridge ANY external service or custom data connection (LLM APIs, External DBs, Vector Stores, Caches, Queues):

```bash
# Register an external LLM Provider (OpenAI, Anthropic, Groq, Ollama)
./bin/llmobs service add OpenAI --type openai --url https://api.openai.com/v1 --category llm --auth-token $OPENAI_API_KEY

# Register an external Database
./bin/llmobs service add Prod-Postgres --type postgres --host db.prod.internal --port 5432 --category database --database analytics --auth-user admin --auth-pass secret

# Register a Vector Database (Qdrant, Pinecone, Milvus, Weaviate)
./bin/llmobs service add Qdrant-Cluster --type qdrant --url https://qdrant.internal:6333 --category vector-db

# List all registered services in tabular format
./bin/llmobs service list
./bin/llmobs service list --category database

# Get JSON definition of a service
./bin/llmobs service get OpenAI

# Run a live diagnostic health probe (HTTP status or TCP socket probe)
./bin/llmobs service test Prod-Postgres

# Automatically bridge the data connection to Grafana as a datasource
./bin/llmobs service sync-to-grafana Prod-Postgres

# Unregister a service
./bin/llmobs service delete OpenAI
```

---

### 4. Grafana Datasources (`datasource`)

Dynamically provision, test, update, list, and delete ANY Grafana datasource without editing YAML provisioning files:

```bash
# List all configured Grafana datasources
./bin/llmobs datasource list

# Add a Prometheus datasource
./bin/llmobs datasource add Prometheus --type prometheus --url http://prometheus:9090

# Add an AlloyDB / PostgreSQL datasource
./bin/llmobs datasource add CustomPostgres --type postgres --url host.docker.internal:5432 --user admin --password secret --database analytics

# Test connection health of a datasource
./bin/llmobs datasource test AlloyDB

# Synchronize default platform datasources from environment
./bin/llmobs datasource sync
./bin/llmobs datasource sync alloydb clickhouse redis tempo

# Delete a datasource by Name or UID
./bin/llmobs datasource delete CustomPostgres
```

---

### 5. Grafana Dashboards (`dashboard`)

Dynamically import, export, search, view, and delete Grafana dashboards:

```bash
# List all configured dashboards
./bin/llmobs dashboard list

# Search dashboards by keyword
./bin/llmobs dashboard list --query "telemetry"

# Import dashboard from a local JSON file
./bin/llmobs dashboard import ./dashboards/llm-telemetry.json --overwrite

# Import dashboard directly from Grafana.com URL
./bin/llmobs dashboard import https://grafana.com/api/dashboards/1860/revisions/latest/download

# Fetch full JSON definition of a dashboard by UID
./bin/llmobs dashboard get <uid>

# Export dashboard JSON to a file
./bin/llmobs dashboard export <uid> --output ./backup-dash.json

# Delete a dashboard by UID
./bin/llmobs dashboard delete <uid>
```

---

### 6. Grafana Unified Alerting & Contact Points (`alert`)

Dynamically manage alert rules and notification channels (Slack, Webhooks, Email, PagerDuty):

```bash
# List all configured alert rules
./bin/llmobs alert list

# Add or update an alert rule from a JSON file
./bin/llmobs alert add ./alerts/high-latency-rule.json

# View details of an alert rule
./bin/llmobs alert get <uid>

# Delete an alert rule
./bin/llmobs alert delete <uid>

# List all notification contact points
./bin/llmobs alert contact-point list

# Add a Slack notification receiver
./bin/llmobs alert contact-point add Slack-Alerts --type slack --webhook-url https://hooks.slack.com/services/...

# Add a Webhook notification receiver
./bin/llmobs alert contact-point add Ops-Webhook --type webhook --url https://webhook.internal/alerts

# Send a test notification to verify contact point connectivity
./bin/llmobs alert contact-point test Slack-Alerts

# Delete a contact point
./bin/llmobs alert contact-point delete Slack-Alerts
```

---

### 7. Resource Tuning & Configuration (`config`)

View, tune, and persist Docker container resource limits (CPU, memory, network):

```bash
# View active platform configuration and resource limits
./bin/llmobs config

# Interactive configuration wizard
./bin/llmobs config -i

# Set specific service resource limits
./bin/llmobs config --temporal-memory 4096M --clickhouse-memory 8192M --alloydb-memory 6144M

# Apply limits and automatically recreate containers
./bin/llmobs config --temporal-memory 4096M --restart
```

---

### 8. Platform Bootstrapping & Setup (`setup`)

Executes the automated 7-step platform bootstrapping pipeline:

```bash
# Interactive setup (prompts for passwords with safe defaults)
./bin/llmobs setup -i

# Automated non-interactive setup
./bin/llmobs setup

# Custom credentials via CLI flags
./bin/llmobs setup --db-password my_secret_pw --redis-password my_redis_pw

# Verify credentials against running databases
./bin/llmobs verify-credentials
```

---

### 9. Horizontal Scaling (`scale`)

Scale stateless services or provision simulated worker nodes:

```bash
# Interactive scaling menu
./bin/llmobs scale

# Scale a stateless service to N replicas
./bin/llmobs scale service llmobs-temporal 3
./bin/llmobs scale service llmobs-traefik 2

# Scale simulated worker nodes
./bin/llmobs scale node 1 host.docker.internal
./bin/llmobs scale node 2 host.docker.internal

# List active compute nodes
./bin/llmobs scale list

# Teardown a compute node
./bin/llmobs scale down-node 2
```

---

### 10. Disaster Recovery & Compliance (`backup-purge`, `gdpr-erasure`)

```bash
# Backup databases to timestamped archive
./bin/llmobs backup-purge --backup-only

# Backup and purge persistent Docker volumes
./bin/llmobs backup-purge

# Execute GDPR/CCPA Right-to-Erasure across AlloyDB and ClickHouse
./bin/llmobs gdpr-erasure --user-id "usr_12345" --actor-id "admin_ops"
```

---

### 11. Security, Certificates & Ingress

```bash
# Generate pure-Go self-signed TLS certificates (server.pem, ca.pem)
./bin/llmobs certs

# Detect and resolve host port contention (ports 31410–31427)
./bin/llmobs free-ports

# Manage Cloudflare Zero-Trust Ingress Tunnels
./bin/llmobs cloudflare setup
./bin/llmobs cloudflare start
./bin/llmobs cloudflare status
./bin/llmobs cloudflare logs
./bin/llmobs cloudflare stop
```

---

### 12. REST API Daemon (`server`)

Run the orchestrator as a background daemon exposing the HTTP REST API:

```bash
# Start API daemon on port 31427 (default)
./bin/llmobs server

# Start API daemon on custom port
./bin/llmobs server --port 8080
```

#### REST API Endpoints Overview

| Resource | Method | Path | Description |
| :--- | :--- | :--- | :--- |
| **Stack** | `POST` | `/api/v1/stack/up` | Start stack profiles |
| **Stack** | `POST` | `/api/v1/stack/down` | Stop stack |
| **Stack** | `GET` | `/api/v1/stack/status` | Container status |
| **Health** | `GET` | `/api/v1/health` | Concurrent health probe |
| **Health** | `GET` | `/api/v1/health/deep` | Deep functional verification |
| **Services** | `GET` / `POST` | `/api/v1/services` | List / Register external service |
| **Services** | `GET` / `DELETE` | `/api/v1/services/:id` | Get / Unregister service |
| **Services** | `POST` | `/api/v1/services/:id/test` | Live health check probe |
| **Services** | `POST` | `/api/v1/services/:id/sync-grafana` | Bridge service to Grafana |
| **Datasources** | `GET` / `POST` | `/api/v1/grafana/datasources` | List / Register datasource |
| **Datasources** | `GET` / `DELETE` | `/api/v1/grafana/datasources/:id` | Get / Delete datasource |
| **Datasources** | `POST` | `/api/v1/grafana/datasources/:id/test` | Test datasource connectivity |
| **Dashboards** | `GET` / `POST` | `/api/v1/grafana/dashboards` | Search / Import dashboard |
| **Dashboards** | `GET` / `DELETE` | `/api/v1/grafana/dashboards/:uid` | Get / Delete dashboard |
| **Alert Rules** | `GET` / `POST` | `/api/v1/grafana/alerts` | List / Add alert rule |
| **Alert Rules** | `GET` / `DELETE` | `/api/v1/grafana/alerts/:uid` | Get / Delete alert rule |
| **Contact Points** | `GET` / `POST` | `/api/v1/grafana/contact-points` | List / Add contact point |
| **Config** | `GET` / `PUT` | `/api/v1/config` | Read / Update resource limits |
| **Compliance** | `POST` | `/api/v1/gdpr/erasure` | GDPR data erasure |

All responses conform to the standard open envelope:

```json
{
  "meta": {
    "version": "v1",
    "timestamp": "2026-09-28T18:00:00Z",
    "traceId": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
  },
  "data": {},
  "errors": []
}
```

---

## Platform Port Reference

| Service | Internal Port | Host Port | Protocol | Health Endpoint |
|---|---|---|---|---|
| **Traefik Gateway** | 80, 8080, 443 | `31410`, `31411`, `31419` | HTTP / HTTPS | `http://localhost:31410/ping` |
| **Redis Ledger** | 6379 | `31413` | TCP | `localhost:31413` |
| **Kafka Broker** | 9092 | `31414` | TCP | `localhost:31414` |
| **Grafana Dashboard** | 3000 | `31415` | HTTP | `http://localhost:31415/api/health` |
| **Grafana Tempo** | 3200, 4317 | `31416`, `31423` | HTTP / gRPC | `http://localhost:31416/ready` |
| **OTel Collector** | 4317, 4318 | `31418`, `31417` | gRPC / HTTP | `http://localhost:31417/` |
| **AlloyDB (PostgreSQL)** | 5432 | `31420` | PostgreSQL | `localhost:31420` |
| **ClickHouse Analytics** | 8123, 9000 | `31421`, `31422` | HTTP / TCP | `http://localhost:31421/ping` |
| **Temporal Engine** | 7233, 8080 | `31424`, `31425` | gRPC / HTTP UI | `localhost:31424` |
| **Service Registry** | 31426 | `31426` | HTTP | `http://localhost:31426/health` |
| **Platform Orchestrator API** | 31427 | `31427` | HTTP | `http://localhost:31427/health` |

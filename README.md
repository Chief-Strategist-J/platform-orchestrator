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
│   │   ├── dns/             # Domain discovery, atomic /etc/hosts sync, and resolution probe engine
│   │   ├── gdpr/            # GDPR/CCPA data erasure across databases
│   │   ├── grafana/         # Dynamic datasources, dashboards, unified alerting, and contact points
│   │   ├── health/          # Concurrent TCP/HTTP diagnostic health probes with backoff
│   │   ├── ports/           # Host port contention detection and automated resolution
│   │   ├── prereqs/         # Docker, daemon, memory, and ulimit checks
│   │   ├── scale/           # Stateless service & simulated compute node scaling
│   │   ├── services/        # Dynamic external services catalog & connectivity probe engine
│   │   ├── setup/           # Full 7-step bootstrapping pipeline with credential prompts
│   │   ├── stack/           # Profile resolution, storage, self-healing, and compose lifecycle
│   │   └── traefik/         # Dynamic ingress routing, middlewares, TLS certs, and overview diagnostics
│   ├── infra/               # Infrastructure adapters (Docker Engine, OTel tracing)
│   └── shared/              # Hexagonal port interfaces, path resolver, and API envelopes
└── tests/                   # Unit and integration test suites
```

---

## Build & Test

```bash
cd packages/platform-orchestrator && \
go build \
  # ── OUTPUT TARGET ──────────────────────────────────────────────────────────
  -o ../../bin/llmobs \
  # File path: Destination binary relative to workspace root.
  # Convention: All compiled binaries land in the top-level bin/ directory.
  ./main.go
  # Entry point: Go package path for the main package.
```

```bash
go test \
  # ── VERBOSITY & SCOPE ──────────────────────────────────────────────────────
  -v \
  # Boolean flag: Streams each test name and PASS/FAIL status to stdout.
  # Omit in CI pipelines to reduce log noise; keep enabled during local dev.
  ./...
  # Package pattern: Recursively runs all test files in the module.
  # Alternative: ./src/features/grafana/... to target a single domain.
```

---

## Complete CLI Command Reference

### 1. Platform Stack Lifecycle (`up`, `down`, `status`, `logs`, `restart`)

```bash
./bin/llmobs up \
  # ── PROFILE SELECTION ──────────────────────────────────────────────────────
  [profile...] \
  # String (variadic): One or more named stack profiles.
  # Valid values:
  #   full      — All 10 core infrastructure services.
  #   db        — AlloyDB + ClickHouse only.
  #   streaming — Kafka + Redis only.
  #   stateless — Traefik + OTel + Temporal (no persistent volumes).
  #   stateful  — AlloyDB + ClickHouse + Redis + Kafka (persistent volumes).
  # Default: Interactive selector (prompts when no profile is supplied).
  #
  # Examples:
  #   ./bin/llmobs up full
  #   ./bin/llmobs up db streaming

  # ── DOCKER NETWORK PARAMETERS ──────────────────────────────────────────────
  --network-name=llmobs-network \
  # String: Name of the Docker bridge network created for inter-container routing.
  # Default: "llmobs-network".
  --network-subnet=172.28.0.0/16 \
  # CIDR string: IP address range allocated to the Docker bridge.
  # Format: "x.x.x.x/prefix". Must not conflict with existing host routes.
  --network-gateway=172.28.0.1
  # IPv4 string: Gateway address on the Docker bridge.
  # Default: First host address of the subnet.
```

```bash
./bin/llmobs status
# Outputs a live table of all containers: name, image, status, uptime, and mapped ports.
```

```bash
./bin/llmobs logs \
  # ── LOG TAIL DEPTH ─────────────────────────────────────────────────────────
  [lines]
  # Integer (optional): Number of tail lines to stream per container.
  # Default: 50.
  # Example: ./bin/llmobs logs 100
```

```bash
./bin/llmobs restart \
  # ── PROFILE SCOPE ──────────────────────────────────────────────────────────
  [profile...]
  # String (variadic): Same profile tokens as "up". Performs down then up.
  # Example: ./bin/llmobs restart full
```

```bash
./bin/llmobs down
# Stops all running containers and removes orphan networks and anonymous volumes.
```

---

### 2. Health Verification & Diagnostic Probes (`health`)

```bash
./bin/llmobs health \
  # ── PROBE DEPTH ────────────────────────────────────────────────────────────
  [--deep] \
  # Boolean flag: Activates deep functional verification beyond TCP reachability.
  # Deep checks performed per service:
  #   alloydb     — Authenticates via PostgreSQL wire protocol; runs SELECT 1.
  #   redis       — Issues RESP PING; validates PONG response.
  #   clickhouse  — Executes "SELECT 1" over HTTP query interface.
  #   grafana     — Calls /api/health; validates JSON {"database":"ok"}.
  # Omit flag for fast concurrent TCP/HTTP reachability checks only.

  # ── SERVICE FILTER ─────────────────────────────────────────────────────────
  --services=alloydb,clickhouse,redis,grafana \
  # Comma-delimited string: Subset of service names to probe.
  # Valid values: alloydb, clickhouse, redis, grafana, kafka, tempo, otel, temporal, traefik.
  # Default: All services registered in the active profile.

  # ── PROFILE FILTER ─────────────────────────────────────────────────────────
  --profiles=stateful
  # Comma-delimited string: Restricts probing to services within the named profile group.
  # Valid values: full, db, streaming, stateless, stateful.
```

---

### 3. External Services & Custom Connections (`service`)

Dynamically register, probe, manage, and bridge ANY external service or custom data connection (LLM APIs, external databases, vector stores, queues).

```bash
./bin/llmobs service add <NAME> \
  # ── SERVICE IDENTITY ───────────────────────────────────────────────────────
  --type=openai \
  # Enum: "openai" | "anthropic" | "groq" | "ollama" | "postgres" | "mysql" |
  #       "redis" | "qdrant" | "pinecone" | "milvus" | "weaviate" | "kafka" | "custom".
  # Determines which connectivity probe and Grafana datasource adapter are used.
  --category=llm \
  # Enum: "llm" | "database" | "vector-db" | "cache" | "queue" | "custom".
  # Used for list filtering and Grafana folder placement.

  # ── NETWORK COORDINATES ────────────────────────────────────────────────────
  --url=https://api.openai.com/v1 \
  # URL string: Full base URL for HTTP/HTTPS services.
  # Mutually exclusive with --host + --port (used for raw TCP services).
  --host=db.prod.internal \
  # Hostname or IP: Used for TCP-socket services (postgres, redis, kafka, etc.).
  --port=5432 \
  # Integer: 1 to 65535. TCP port the remote service listens on.

  # ── DATABASE COORDINATES ───────────────────────────────────────────────────
  --database=analytics \
  # String: Database name to authenticate against (postgres, mysql, clickhouse).

  # ── AUTHENTICATION ─────────────────────────────────────────────────────────
  --auth-token=$OPENAI_API_KEY \
  # String: Bearer token or API key. Stored encrypted in the service registry.
  --auth-user=admin \
  # String: Username for database or basic-auth services.
  --auth-pass=secret
  # String: Password for database or basic-auth services.
  #
  # Examples:
  #   ./bin/llmobs service add OpenAI --type openai --url https://api.openai.com/v1 --category llm --auth-token $OPENAI_API_KEY
  #   ./bin/llmobs service add Prod-Postgres --type postgres --host db.prod.internal --port 5432 --category database --database analytics --auth-user admin --auth-pass secret
  #   ./bin/llmobs service add Qdrant-Cluster --type qdrant --url https://qdrant.internal:6333 --category vector-db
```

```bash
./bin/llmobs service list \
  # ── OUTPUT FILTER ──────────────────────────────────────────────────────────
  [--category=database]
  # Enum (optional): Restricts table to services of a specific category.
  # Omit to list all registered services.
```

```bash
./bin/llmobs service get <NAME>
# Returns the full JSON definition of a registered service including masked credentials.
```

```bash
./bin/llmobs service test <NAME>
# Runs a live diagnostic probe (HTTP GET or TCP socket) and reports latency + status.
```

```bash
./bin/llmobs service sync-to-grafana <NAME>
# Automatically bridges the registered service to Grafana as a datasource.
# Applies the correct datasource type adapter based on --type.
```

```bash
./bin/llmobs service delete <NAME>
# Permanently removes the service registration from the registry.
```

---

### 4. Grafana Datasources (`datasource`)

Dynamically provision, test, update, list, and delete ANY Grafana datasource without editing YAML provisioning files.

```bash
./bin/llmobs datasource add <NAME> \
  # ── DATASOURCE TYPE ────────────────────────────────────────────────────────
  --type=prometheus \
  # Enum: "prometheus" | "postgres" | "clickhouse" | "redis" | "tempo" | "loki" | "jaeger".
  # Determines plugin ID sent to the Grafana Provisioning API.

  # ── NETWORK COORDINATES ────────────────────────────────────────────────────
  --url=http://prometheus:9090 \
  # URL string: Internal Docker network address or external URL of the datasource.

  # ── DATABASE AUTHENTICATION (postgres / clickhouse) ────────────────────────
  --user=admin \
  # String: Database user for SQL-type datasources.
  --password=secret \
  # String: Database password. Sent to Grafana over TLS; never stored in plaintext locally.
  --database=analytics
  # String: Target database name for SQL-type datasources.
  #
  # Examples:
  #   ./bin/llmobs datasource add Prometheus --type prometheus --url http://prometheus:9090
  #   ./bin/llmobs datasource add CustomPostgres --type postgres --url host.docker.internal:5432 --user admin --password secret --database analytics
```

```bash
./bin/llmobs datasource list
# Lists all configured Grafana datasources in tabular format (name, type, URL, UID).
```

```bash
./bin/llmobs datasource test <NAME>
# Calls Grafana datasource health check endpoint and reports status + round-trip time.
```

```bash
./bin/llmobs datasource sync \
  # ── SYNC TARGETS ───────────────────────────────────────────────────────────
  [service...]
  # String (variadic): Named platform datasources to synchronize from environment config.
  # Valid values: alloydb, clickhouse, redis, tempo.
  # Default: All four platform datasources when no arguments are supplied.
  #
  # Examples:
  #   ./bin/llmobs datasource sync
  #   ./bin/llmobs datasource sync alloydb clickhouse redis tempo
```

```bash
./bin/llmobs datasource delete <NAME>
# Deletes the datasource by Name or UID via the Grafana HTTP API.
```

---

### 5. Grafana Dashboards (`dashboard`)

Dynamically import, export, search, view, and delete Grafana dashboards.

```bash
./bin/llmobs dashboard list \
  # ── SEARCH FILTER ──────────────────────────────────────────────────────────
  [--query="telemetry"]
  # String (optional): Keyword filter applied against dashboard titles.
  # Omit to list all dashboards across all folders.
```

```bash
./bin/llmobs dashboard import <SOURCE> \
  # ── SOURCE ─────────────────────────────────────────────────────────────────
  # String: Either:
  #   File path — Local JSON dashboard file (e.g., ./dashboards/llm-telemetry.json).
  #   URL       — Grafana.com download endpoint.
  #               Example: https://grafana.com/api/dashboards/1860/revisions/latest/download

  # ── CONFLICT RESOLUTION ────────────────────────────────────────────────────
  [--overwrite]
  # Boolean flag: Replaces an existing dashboard with the same UID.
  # Omit to fail-fast if a dashboard with the same UID already exists.
  #
  # Examples:
  #   ./bin/llmobs dashboard import ./dashboards/llm-telemetry.json --overwrite
  #   ./bin/llmobs dashboard import https://grafana.com/api/dashboards/1860/revisions/latest/download
```

```bash
./bin/llmobs dashboard get <UID>
# Returns the full Grafana JSON model of the dashboard identified by UID.
```

```bash
./bin/llmobs dashboard export <UID> \
  # ── OUTPUT DESTINATION ─────────────────────────────────────────────────────
  --output=./backup-dash.json
  # File path: Writes dashboard JSON to the specified local file.
```

```bash
./bin/llmobs dashboard delete <UID>
# Permanently deletes the dashboard from Grafana by UID.
```

---

### 6. Grafana Unified Alerting & Contact Points (`alert`)

Dynamically manage alert rules and notification channels (Slack, Webhooks, Email, PagerDuty).

```bash
./bin/llmobs alert list
# Lists all alert rules in tabular format (UID, name, group, state).
```

```bash
./bin/llmobs alert add <FILE>
# Adds or updates an alert rule from a Grafana-format JSON file.
# Performs an upsert: creates if UID is absent, updates if UID already exists.
```

```bash
./bin/llmobs alert get <UID>
# Returns the full JSON definition of the alert rule identified by UID.
```

```bash
./bin/llmobs alert delete <UID>
# Permanently removes the alert rule from Grafana.
```

```bash
./bin/llmobs alert contact-point list
# Lists all notification contact points (name, type, UID).
```

```bash
./bin/llmobs alert contact-point add <NAME> \
  # ── RECEIVER TYPE ──────────────────────────────────────────────────────────
  --type=slack \
  # Enum: "slack" | "webhook" | "email" | "pagerduty" | "opsgenie" | "victorops".
  # Determines which Grafana notifier plugin is instantiated.

  # ── DELIVERY COORDINATES ───────────────────────────────────────────────────
  --webhook-url=https://hooks.slack.com/services/... \
  # URL string: Incoming webhook URL (slack, webhook types).
  --url=https://webhook.internal/alerts
  # URL string: Generic target URL for webhook type.
  #
  # Examples:
  #   ./bin/llmobs alert contact-point add Slack-Alerts --type slack --webhook-url https://hooks.slack.com/services/...
  #   ./bin/llmobs alert contact-point add Ops-Webhook --type webhook --url https://webhook.internal/alerts
```

```bash
./bin/llmobs alert contact-point test <NAME>
# Sends a synthetic test notification and reports delivery status.
```

```bash
./bin/llmobs alert contact-point delete <NAME>
# Permanently removes the contact point from Grafana.
```

---

### 7. Resource Tuning & Configuration (`config`)

View, tune, and persist Docker container resource limits (CPU, memory, network).

```bash
./bin/llmobs config \
  # ── INTERACTIVE MODE ───────────────────────────────────────────────────────
  [-i] \
  # Boolean flag: Launches an interactive TUI wizard for guided resource configuration.
  # Omit to print the current configuration in tabular format and exit.

  # ── MEMORY LIMITS ──────────────────────────────────────────────────────────
  --temporal-memory=4096M \
  # String: Memory limit for the Temporal Engine container.
  # Format: Integer followed by unit — "M" (mebibytes) or "G" (gibibytes).
  # Examples: "2048M", "4G".
  --clickhouse-memory=8192M \
  # String: Memory limit for the ClickHouse Analytics container.
  --alloydb-memory=6144M \
  # String: Memory limit for the AlloyDB (PostgreSQL) container.

  # ── APPLY & RESTART ────────────────────────────────────────────────────────
  [--restart]
  # Boolean flag: Automatically recreates affected containers after persisting limits.
  # Omit to persist limits to config without restarting running containers.
  #
  # Examples:
  #   ./bin/llmobs config
  #   ./bin/llmobs config -i
  #   ./bin/llmobs config --temporal-memory 4096M --clickhouse-memory 8192M --alloydb-memory 6144M
  #   ./bin/llmobs config --temporal-memory 4096M --restart
```

---

### 8. Platform Bootstrapping & Setup (`setup`)

Executes the automated 7-step platform bootstrapping pipeline.

```bash
./bin/llmobs setup \
  # ── INTERACTIVE MODE ───────────────────────────────────────────────────────
  [-i] \
  # Boolean flag: Enables interactive prompts for passwords with safe auto-generated defaults.
  # Omit for fully automated non-interactive bootstrapping using flag values or defaults.

  # ── CREDENTIAL OVERRIDES ───────────────────────────────────────────────────
  --db-password=my_secret_pw \
  # String: Master password for AlloyDB (PostgreSQL). Stored in .env and Docker secrets.
  --redis-password=my_redis_pw
  # String: AUTH password for the Redis Ledger.
  #
  # Examples:
  #   ./bin/llmobs setup -i
  #   ./bin/llmobs setup
  #   ./bin/llmobs setup --db-password my_secret_pw --redis-password my_redis_pw
```

```bash
./bin/llmobs verify-credentials
# Authenticates against AlloyDB and Redis using stored credentials and reports pass/fail.
```

---

### 9. Horizontal Scaling (`scale`)

Scale stateless services or provision simulated worker nodes.

```bash
./bin/llmobs scale
# Launches an interactive scaling menu when called with no subcommand.
```

```bash
./bin/llmobs scale service <SERVICE_NAME> <REPLICAS> \
  # ── SERVICE IDENTIFIER ─────────────────────────────────────────────────────
  # String: Docker Compose service name to scale.
  # Valid values: llmobs-temporal, llmobs-traefik, llmobs-otel-collector.
  # Note: Stateful services (AlloyDB, ClickHouse, Redis, Kafka) cannot be horizontally scaled.

  # ── REPLICA COUNT ──────────────────────────────────────────────────────────
  # Integer: Desired number of container replicas. Minimum: 1.
  #
  # Examples:
  #   ./bin/llmobs scale service llmobs-temporal 3
  #   ./bin/llmobs scale service llmobs-traefik 2
```

```bash
./bin/llmobs scale node <NODE_ID> <HOST> \
  # ── NODE IDENTIFIER ────────────────────────────────────────────────────────
  # Integer: Unique ID assigned to the simulated compute node (e.g., 1, 2, 3).

  # ── NODE HOST ──────────────────────────────────────────────────────────────
  # Hostname or IP: Docker host address for the simulated worker node.
  # Default for local dev: host.docker.internal.
  #
  # Examples:
  #   ./bin/llmobs scale node 1 host.docker.internal
  #   ./bin/llmobs scale node 2 host.docker.internal
```

```bash
./bin/llmobs scale list
# Lists all active compute nodes with their IDs, hosts, and status.
```

```bash
./bin/llmobs scale down-node <NODE_ID>
# Gracefully tears down and deregisters the compute node with the given ID.
```

---

### 10. Disaster Recovery & Compliance (`backup-purge`, `gdpr-erasure`)

```bash
./bin/llmobs backup-purge \
  # ── BACKUP-ONLY MODE ───────────────────────────────────────────────────────
  [--backup-only]
  # Boolean flag: Dumps AlloyDB and ClickHouse to timestamped archives without purging volumes.
  # Omit to perform backup AND purge of all persistent Docker volumes.
  #
  # Examples:
  #   ./bin/llmobs backup-purge --backup-only
  #   ./bin/llmobs backup-purge
```

```bash
./bin/llmobs gdpr-erasure \
  # ── SUBJECT IDENTIFICATION ─────────────────────────────────────────────────
  --user-id="usr_12345" \
  # String: Unique identifier of the data subject requesting erasure.
  # Applied across AlloyDB tables and ClickHouse event streams.

  # ── AUDIT TRAIL ────────────────────────────────────────────────────────────
  --actor-id="admin_ops"
  # String: Identity of the operator initiating erasure. Written to the audit log.
  # Format: Any string; recommend service account name or operator email.
```

---

### 11. Security, Certificates & Ingress

```bash
./bin/llmobs certs
# Generates pure-Go self-signed X.509 certificates (server.pem, ca.pem) in config/certs/.
# Certificate spec: 2048-bit RSA, SHA-256, 365-day validity, SAN for localhost + 127.0.0.1.
```

```bash
./bin/llmobs free-ports
# Scans host ports 31410-31427 for contention, identifies conflicting processes,
# and offers automated resolution (process termination or port reassignment).
```

```bash
./bin/llmobs cloudflare \
  # ── SUBCOMMAND ─────────────────────────────────────────────────────────────
  <subcommand>
  # Enum: "setup" | "start" | "status" | "logs" | "stop".
  #   setup  — Authenticates with Cloudflare Zero Trust and creates a named tunnel.
  #   start  — Launches cloudflared daemon routing ingress to the Traefik gateway.
  #   status — Reports tunnel health and active connector count.
  #   logs   — Streams cloudflared daemon stdout.
  #   stop   — Terminates the cloudflared daemon and closes the tunnel.
  #
  # Examples:
  #   ./bin/llmobs cloudflare setup
  #   ./bin/llmobs cloudflare start
  #   ./bin/llmobs cloudflare status
  #   ./bin/llmobs cloudflare logs
  #   ./bin/llmobs cloudflare stop
```

---

### 12. Traefik Ingress Gateway Management (`traefik`)

Manage and inspect dynamic Traefik ingress routing, load balancer services, middleware chains, and gateway health.

```bash
./bin/llmobs traefik ping \
  # ── API ENDPOINT & TIMEOUT ────────────────────────────────────────────────
  [--url=http://localhost:31411] \
  # String: Base URL of Traefik management API. Default: "http://localhost:31411".
  [--timeout=10]
  # Integer: HTTP client timeout in seconds. Default: 10.
  #
  # Aliases: status, health.
```

```bash
./bin/llmobs traefik overview
# Displays aggregate counts of active HTTP/TCP routers, services, and middlewares with error/warning counts.
```

```bash
./bin/llmobs traefik entrypoints
# Lists all active network entrypoints and their port bindings (e.g. web: :80, websecure: :443, tcp-db: :31420).
```

```bash
./bin/llmobs traefik router list
# Lists all HTTP routers with their matching rules, target services, and attached entrypoints.
```

```bash
./bin/llmobs traefik router get <NAME>
# Fetches complete JSON configuration of a specific HTTP router.
# Example: ./bin/llmobs traefik router get grafana-router
```

```bash
./bin/llmobs traefik router add <NAME> \
  # ── ROUTING RULE EXPRESSION ────────────────────────────────────────────────
  --rule="Host(\`app.llmobs.local\`) && PathPrefix(\`/api\`)" \
  # String (required): Traefik match rule expression (Host, PathPrefix, Headers, etc.).
  --service="app-service" \
  # String (required): Target backend load balancer service name.
  --entrypoints="websecure" \
  # String slice: EntryPoints to attach to the router. Default: ["websecure"].
  --middlewares="auth-mw,rate-limit-mw"
  # String slice (optional): Middleware chains to attach to this router.
  #
  # Example:
  #   ./bin/llmobs traefik router add custom-app --rule "Host(\`custom.llmobs.local\`)" --service "custom-svc"
```

```bash
./bin/llmobs traefik router delete <NAME>
# Removes an HTTP router from dynamic configuration (config/traefik/dynamic.yml).
# Aliases: rm.
# Example: ./bin/llmobs traefik router delete custom-app
```

```bash
./bin/llmobs traefik services
# Lists backend load balancer services, health statuses, and server URL endpoints.
```

```bash
./bin/llmobs traefik middlewares
# Lists registered security, retry, and rate-limiting middlewares.
```

```bash
./bin/llmobs traefik tcp list
# Lists all active TCP/gRPC routers and their HostSNI rules.
```

```bash
./bin/llmobs traefik tcp add <NAME> \
  --rule="HostSNI(\`alloydb.llmobs.local\`)" \
  --service="alloydb-tcp-svc" \
  --entrypoints="tcp-db"
# Registers or updates a TCP/gRPC router in dynamic configuration.
```

```bash
./bin/llmobs traefik tcp delete <NAME>
# Removes a TCP router from dynamic configuration.
```

---

### 13. DNS Discovery & `/etc/hosts` Synchronization (`dns`)

Discover platform routing domains, measure DNS resolution latency, and safely persist local records into `/etc/hosts` using demarcated blocks.

```bash
./bin/llmobs dns list \
  # ── TARGET HOSTS FILE & OUTPUT ─────────────────────────────────────────────
  [--hosts-file=/etc/hosts] \
  # String: Path to hosts file. Default: dynamic config or "/etc/hosts".
  [--json]
  # Boolean: Formats output as structured JSON instead of tabular text.
```

```bash
./bin/llmobs dns sync \
  # ── SYNC PARAMETERS ────────────────────────────────────────────────────────
  --ip=127.0.0.1 \
  # IPv4/IPv6 string: Target IP address to associate with discovered platform domains. Default: 127.0.0.1.
  [--dry-run] \
  # Boolean: Simulates modifications without mutating the target hosts file.
  [--domains=grafana.local,alloydb.local] \
  # String slice: Additional custom domain names to append to the sync block.
  [--hosts-file=/etc/hosts] \
  # String: Target hosts file path.
  [--json]
  # Boolean: Formats sync result report as JSON.
  #
  # Examples:
  #   sudo ./bin/llmobs dns sync
  #   ./bin/llmobs dns sync --dry-run
  #   sudo ./bin/llmobs dns sync --ip 192.168.1.100 --domains extra.local
```

```bash
./bin/llmobs dns check [domains...] \
  # ── VERIFICATION SCOPE ─────────────────────────────────────────────────────
  [--hosts-file=/etc/hosts] \
  [--json]
  # Measures DNS query round-trip latency and verifies IP reachability.
  # Aliases: test, probe.
  #
  # Examples:
  #   ./bin/llmobs dns check
  #   ./bin/llmobs dns check grafana.llmobs.local clickhouse.llmobs.local
```

```bash
./bin/llmobs dns discover
# Prints all platform domains discovered across Traefik routing rules and configured services.
```

---

### 14. REST API Daemon (`server`)

Run the orchestrator as a background daemon exposing the HTTP REST API.

```bash
./bin/llmobs server \
  # ── LISTEN PORT ────────────────────────────────────────────────────────────
  [--port=31427]
  # Integer: 1 to 65535. Host port the HTTP API daemon binds to.
  # Default: 31427.
  #
  # Examples:
  #   ./bin/llmobs server
  #   ./bin/llmobs server --port 8080
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
| **Traefik Ingress** | `GET` | `/api/v1/traefik/overview` | Gateway health & active resource summary |
| **Traefik Ingress** | `GET` | `/api/v1/traefik/entrypoints` | Active port and protocol bindings |
| **Traefik Ingress** | `GET` / `POST` | `/api/v1/traefik/routers` | List / Save HTTP routing rule |
| **Traefik Ingress** | `GET` / `DELETE` | `/api/v1/traefik/routers/:name` | Inspect / Remove HTTP routing rule |
| **Traefik Ingress** | `GET` | `/api/v1/traefik/services` | List HTTP load balancer services |
| **Traefik Ingress** | `GET` | `/api/v1/traefik/middlewares` | List active middleware pipeline configs |
| **Traefik Ingress** | `GET` / `POST` | `/api/v1/traefik/tcp/routers` | List / Save TCP ingress routing rule |
| **Traefik Ingress** | `DELETE` | `/api/v1/traefik/tcp/routers/:name` | Remove TCP ingress routing rule |
| **DNS Management** | `GET` | `/api/v1/dns/records` | Discover configured domain records |
| **DNS Management** | `POST` | `/api/v1/dns/sync` | Atomically synchronize /etc/hosts file |
| **DNS Management** | `GET` / `POST` | `/api/v1/dns/check` | Probe DNS resolution & IP reachability |

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

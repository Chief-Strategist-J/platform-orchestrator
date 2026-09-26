# LLMObs Platform Orchestrator

[![Go Version](https://img.shields.io/badge/go-1.21%2B-blue.svg)](https://golang.org)
[![OpenAPI Spec](https://img.shields.io/badge/OpenAPI-3.1.0-brightgreen.svg)](contracts/openapi/v1.yaml)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Open-standard, unified Go orchestration engine for the **LLM Observability & Infrastructure Platform**. Replaces fragmented shell scripts with a strongly-typed, hexagonal architecture supporting CLI and REST operations.

---

## Architecture Overview

Built following **Hexagonal Architecture (Ports & Adapters)**, **Zero-Inline-Comment Doctrine**, and strict **Single Responsibility Principle (SRP)**:

```
packages/platform-orchestrator/
├── config/                  # Environment schemas & cascading YAML configurations
├── contracts/               # OpenAPI 3.1.0 specifications
├── src/
│   ├── api/rest/            # HTTP handlers and router adhering to standard envelopes
│   ├── cmd/                 # Cobra CLI commands (up, down, scale, health, etc.)
│   ├── features/            # Isolated business domain modules
│   │   ├── backup/          # Disaster recovery and database dumping
│   │   ├── certs/           # Pure Go self-signed X.509 certificate generation
│   │   ├── cloudflare/      # Cloudflare Tunnel ingress management
│   │   ├── gdpr/            # GDPR/CCPA data erasure across databases
│   │   ├── health/          # Concurrent TCP/HTTP diagnostic health probes
│   │   ├── ports/           # Host port contention detection and resolution
│   │   ├── prereqs/         # Docker, daemon, memory, and ulimit checks
│   │   ├── scale/           # Stateless service & simulated compute node scaling
│   │   ├── setup/           # Full 7-step bootstrapping pipeline
│   │   └── stack/           # Profile resolution, storage, and compose lifecycle
│   ├── infra/               # Infrastructure adapters (Docker Engine, OTel tracing)
│   └── shared/              # Hexagonal port interfaces and API envelopes
└── tests/                   # Unit and integration test suites
```

---

## CLI Usage

Compile binary:
```bash
make build
```

Run commands:
```bash
# Start infrastructure stack with profile selector
./bin/llmobs up

# Start specific profiles
./bin/llmobs up db streaming

# Status of all containers
./bin/llmobs status

# Concurrent health diagnostics
./bin/llmobs health

# Interactive or CLI scaling
./bin/llmobs scale
./bin/llmobs scale service llmobs-temporal 3
./bin/llmobs scale node 2 host.docker.internal

# Full 7-step setup pipeline
./bin/llmobs setup

# Disaster recovery backup
./bin/llmobs backup-purge --backup-only

# Self-signed TLS certificates
./bin/llmobs certs

# Start OpenAPI REST API daemon
./bin/llmobs server
```

---

## REST API Specification

Conforms to [OpenAPI 3.1.0 Contract](contracts/openapi/v1.yaml). All responses adhere to the standard envelope:
```json
{
  "meta": {
    "version": "v1",
    "timestamp": "2026-09-26T12:00:00Z",
    "traceId": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
  },
  "data": {},
  "errors": []
}
```

---

## Testing

```bash
make test
```

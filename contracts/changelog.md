# Contract Changelog

## [1.1.0] - 2026-09-26
### Added
- Disaster recovery backup & volume purge endpoint (`POST /backup/execute`) conforming to standardized envelopes.
- System prerequisites diagnostic audit endpoint (`GET /prereqs/audit`).
- Platform setup bootstrapping pipeline endpoint (`POST /setup/bootstrap`).
- Cloudflare Tunnel ingress management endpoints (`POST /cloudflare/token`, `POST /cloudflare/start`, `POST /cloudflare/stop`, `GET /cloudflare/status`).
- GDPR & CCPA right-to-erasure endpoint (`POST /gdpr/erasure`) with multi-store deletion and audit event logging.
- Host port contention diagnostics and freeing endpoints (`GET /ports/status`, `POST /ports/free`).
- Comprehensive OpenAPI 3.1.0 request/response component schemas for all newly added endpoints.

## [1.0.0] - 2026-09-26
### Added
- Initial OpenAPI 3.1.0 contract for Platform Infrastructure Orchestrator (`v1.yaml`).
- Stack lifecycle endpoints: `/stack/up`, `/stack/down`, `/stack/status`.
- Horizontal scaling endpoints: `/scale/service`, `/scale/node`, `/scale/node/{nodeId}`.
- Diagnostic & security endpoints: `/health`, `/certs/generate`.
- Standard transport envelope schema with W3C `traceId`, `timestamp`, and structured errors.

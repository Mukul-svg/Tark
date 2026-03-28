# OwnLLM

OwnLLM is a Go-based control plane for provisioning Kubernetes clusters, deploying model-serving workloads, and proxying inference requests through a single API surface.

This repository is no longer just a learning sandbox. It already contains a working foundation for:

- provisioning infrastructure asynchronously
- storing cluster and deployment metadata in PostgreSQL
- caching model routing data in Redis
- deploying model workloads to Kubernetes
- proxying `/v1/chat/completions` requests to active model backends

At the same time, several production-hardening layers are still missing, especially around security, durable job tracking, API maturity, testing, and packaging.

---

## Current Status

**Overall progress:** ~45%

### Phase progress

- Phase 1 — Persistence Foundation: **95%**
- Phase 2 — Orchestrator Core: **70%**
- Phase 3 — Universal Proxy: **80%**
- Phase 4 — API Maturity & Reliability: **0%**
- Phase 5 — Observability: **5%**
- Phase 6 — Security Hardening: **0%**
- Phase 7 — Auth, RBAC & Multi-tenancy: **10%**
- Phase 8 — Testing & CI/CD: **0%**
- Phase 9 — HA & Operational Readiness: **0%**
- Phase 10 — Packaging & Distribution: **0%**

### Current reality

The core flow works:

1. `POST /api/provision`
2. background worker provisions infrastructure with Pulumi
3. worker fetches kubeconfig over SSH and stores cluster details
4. `POST /api/deploy`
5. background worker deploys model-serving workload to Kubernetes
6. deployment service URL is stored in Postgres
7. `POST /v1/chat/completions` resolves model target from Redis or Postgres and proxies traffic

This means the project is already useful for local/dev experimentation, but it is **not production-ready yet**.

---

## What Is Implemented

## ✅ Completed

### Persistence and data layer
- [x] PostgreSQL integration with `pgx/v5`
- [x] Redis integration with `go-redis/v9`
- [x] Store interface abstraction for DB access
- [x] Organization CRUD support
- [x] Cluster CRUD support
- [x] Deployment CRUD support
- [x] Active deployment target lookup for proxy routing
- [x] Initial SQL migration for:
  - [x] `organizations`
  - [x] `clusters`
  - [x] `deployments`

### Async orchestration
- [x] Asynq client integration
- [x] Asynq worker server integration
- [x] Separate `cmd/worker` process
- [x] Queue definitions:
  - [x] `critical`
  - [x] `infra-provision`
  - [x] `model-deploy`
  - [x] `cleanup`
- [x] Typed task payloads for:
  - [x] cluster provision
  - [x] model deploy
  - [x] cluster destroy
- [x] Deterministic task IDs
- [x] `POST /api/provision` returns `202 Accepted`
- [x] `POST /api/deploy` returns `202 Accepted`
- [x] `POST /api/destroy` returns `202 Accepted`
- [x] `GET /api/jobs/:id` returns Asynq task info
- [x] Worker graceful shutdown via signal-aware context

### Infra provisioning
- [x] Pulumi Automation API integration
- [x] Pulumi `up` flow for provisioning
- [x] Pulumi `destroy` flow for cleanup
- [x] Pulumi output extraction (`publicIp`)
- [x] SSH kubeconfig retrieval from provisioned VM
- [x] Retry loop for kubeconfig retrieval
- [x] Cluster DB status updates during provisioning

### Kubernetes deployment
- [x] Kubernetes client initialization from:
  - [x] local kubeconfig
  - [x] in-cluster config
  - [x] raw kubeconfig bytes from DB
- [x] Model deployment helper
- [x] Init-container based model download
- [x] `llama.cpp` server deployment
- [x] NodePort service creation
- [x] CPU and memory requests/limits on model container
- [x] Deployment DB status updates during deploy flow

### Proxy and routing
- [x] `/v1/chat/completions` handler
- [x] Request-body model parsing
- [x] Redis-first service discovery
- [x] Postgres fallback service discovery
- [x] Round-robin load balancing across targets
- [x] TCP health probing of targets
- [x] Reverse proxy reuse per target
- [x] Streaming-safe proxy configuration
- [x] Proxy cache invalidation after deploy/fail events

### Basic runtime
- [x] Echo-based HTTP server
- [x] Panic recovery middleware
- [x] Request logging middleware
- [x] Basic `/healthz` endpoint
- [x] Docker Compose for local Postgres + Redis

---

## ⚠️ Partially Implemented

These areas exist, but do **not** yet match the architecture plan fully.

- [ ] Job tracking is only partially implemented
  - current state is visible through Asynq inspection
  - durable DB-backed `jobs` table is still missing
- [ ] Deployment state transitions are only partial
  - current statuses include things like `installing`, `active`, `failed`
  - full state machine (`queued → building → deploying → running`) is missing
- [ ] Health checks are basic
  - `/healthz` exists
  - `/readyz` with DB/Redis readiness checks is missing
- [ ] Organization model exists
  - real multi-tenant enforcement does not
- [ ] Worker shutdown is implemented
  - API server graceful shutdown is not
- [ ] Proxy health checks exist
  - only TCP probing is implemented
  - richer HTTP/gRPC-aware health checks are missing

---

## ❌ Pending Work

## Phase 4 — API Maturity & Reliability
- [ ] Add API versioning under `/api/v1`
- [ ] Introduce structured error package
- [ ] Standardize all error responses
- [ ] Add custom global Echo error handler
- [ ] Add request validation on all payloads
- [ ] Add request ID middleware
- [ ] Add config validation with fail-fast startup
- [ ] Add `_FILE` config support for secret files
- [ ] Add automated DB migration runner
- [ ] Create durable `jobs` table
- [ ] Add DB-backed job history and status transitions
- [ ] Add audit logging table and write path
- [ ] Add pagination/list endpoints
- [ ] Introduce domain service layer
- [ ] Add OpenAPI/API documentation

## Phase 5 — Observability
- [ ] Add `/readyz`
- [ ] Add Prometheus `/metrics`
- [ ] Add structured request-scoped logging
- [ ] Add internal typed event system
- [ ] Add progress streaming for jobs/deployments
- [ ] Add distributed tracing

## Phase 6 — Security Hardening
- [ ] Encrypt kubeconfig at rest
- [ ] Replace insecure SSH host key handling
- [ ] Stop skipping Kubernetes TLS verification
- [ ] Add TLS for DB and Redis connections
- [ ] Add request rate limiting
- [ ] Add strict input validation
- [ ] Improve secrets handling

## Phase 7 — Auth, RBAC & Multi-tenancy
- [ ] Add JWT/OIDC auth middleware
- [ ] Add user model
- [ ] Add RBAC route guards
- [ ] Enforce `org_id` scoping in all queries
- [ ] Add session/token lifecycle handling

## Phase 8 — Testing & CI/CD
- [ ] Add unit tests
- [ ] Add handler tests
- [ ] Add worker tests
- [ ] Add store integration tests
- [ ] Introduce interfaces for queue and kube boundaries
- [ ] Add mocking strategy
- [ ] Add `Dockerfile`
- [ ] Add CI workflow
- [ ] Add linting config
- [ ] Add integration test setup

## Phase 9 — HA & Operational Readiness
- [ ] Add multi-replica API topology
- [ ] Add Redis HA/Sentinel or cluster mode
- [ ] Add PostgreSQL HA/replication
- [ ] Add backup/restore strategy
- [ ] Add leader election where needed

## Phase 10 — Packaging & Distribution
- [ ] Add Helm chart
- [ ] Add production values/config templates
- [ ] Add license enforcement
- [ ] Add feature-flag/tier gating
- [ ] Add release/versioning strategy
- [ ] Add installation and operations docs

---

## Main HTTP Endpoints

### Management API
- `POST /api/provision`
- `POST /api/deploy`
- `POST /api/destroy`
- `GET /api/jobs/:id`

### Inference API
- `POST /v1/chat/completions`

### Health
- `GET /healthz`

---

## Project Structure

```text
.
├── cmd/
│   ├── server/                  # API server entrypoint
│   └── worker/                  # Background worker entrypoint
├── internal/
│   ├── app/                     # Application wiring
│   ├── cache/                   # Redis client wrapper
│   ├── config/                  # Environment configuration
│   ├── http/                    # Echo router and handlers
│   ├── kube/                    # Kubernetes deployment logic
│   ├── models/                  # Domain models
│   ├── queue/                   # Queue names and task types
│   ├── store/                   # Store interface + Postgres implementation
│   └── worker/                  # Asynq client/server, Pulumi, SSH
├── infra/
│   └── azure/                   # Pulumi Azure infrastructure code
├── docker-compose.yml           # Local Postgres + Redis
├── docs/ARCHITECTURE_PLAN.md    # Target architecture and roadmap
└── go.mod
```

---

## Local Development

### Prerequisites
- Go
- Docker + Docker Compose
- PostgreSQL and Redis (or use Compose)
- Pulumi CLI
- Azure credentials/config if provisioning infra
- Kubernetes access if deploying workloads

### Start local dependencies
```bash
docker compose up -d
```

### Apply initial schema
At the moment migrations are still manual. Apply:

- `internal/store/migrations/001_init.sql`

### Run API server
```bash
go run ./cmd/server
```

### Run worker
```bash
go run ./cmd/worker
```

---

## Known Gaps / Important Notes

- The API server does **not** yet do graceful shutdown.
- Kubeconfig is currently stored in plaintext and must be encrypted before production use.
- SSH host key verification is currently insecure and must be fixed.
- Kubernetes TLS verification is currently loosened for remote cluster access.
- There are **no tests yet**.
- There is **no Dockerfile or CI pipeline yet**.
- The current README reflects the real implementation status better than the original architecture gap tracker, which is slightly outdated in a few places.

---

## Near-Term Priorities

### Immediate
- [ ] encrypt kubeconfig at rest
- [ ] add API server graceful shutdown
- [ ] create durable `jobs` table
- [ ] standardize error handling
- [ ] add request validation

### Next
- [ ] add request ID middleware
- [ ] add readiness and metrics endpoints
- [ ] add JWT auth
- [ ] add tests
- [ ] add Dockerfile and CI

---

## Status Summary

OwnLLM already has a **working control-plane skeleton** with real async infrastructure provisioning, real Kubernetes deployment flow, and a real model-aware inference proxy.

What remains is the set of features that turn a working prototype into a reliable platform:

- durability
- security
- observability
- validation
- auth
- tests
- packaging

Until those are added, treat this repository as a **functional prototype / dev-stage platform**, not a production-ready system.
# OwnLLM — Implementation Status Report

**Reference:** `ARCHITECTURE_PLAN.md`
**Scope:** All 23 Go source files read and verified
**Overall Completion:** ~45% (foundation is solid; production hardening not started)

---

## 1. Codebase Map (All Files Verified)

```
cmd/
  server/main.go           — API server entry point
  worker/main.go           — Worker process entry point (has graceful shutdown ✅)

internal/
  app/app.go               — Dependency injection wiring
  config/config.go         — Environment-based config (silent defaults ⚠️)
  cache/redis.go           — Redis client wrapper
  models/types.go          — Organization, Cluster, Deployment structs

  store/
    store.go               — Store interface (15 methods)
    postgres.go            — Full PostgreSQL implementation
    migrations/
      001_init.sql         — 3 tables: organizations, clusters, deployments

  queue/
    queues.go              — Queue names + task type constants

  worker/
    client.go              — Asynq client + Inspector (EnqueueProvision/Deploy/Destroy/GetTaskInfo)
    server.go              — Asynq server with queue weights, state transitions, cache invalidation
    provisioner.go         — Pulumi Up/Destroy (Azure, streams progress to stdout)
    ssh.go                 — SSH kubeconfig fetch with retry (30 attempts × 10s)
    tasks/tasks.go         — Typed task payloads + marshal/unmarshal helpers

  kube/
    client.go              — Kubernetes client (local kubeconfig + remote from bytes)
    deploy.go              — EnsureNginxDeployment (unused in prod path)
    vllm.go                — DeployModel: init-container download + llama.cpp server + NodePort svc

  http/
    router.go              — Echo server, flat route registration ⚠️
    handlers/
      provision.go         — HandleProvision / HandleDestroy (returns 202)
      deploy.go            — PostDeploy (returns 202 + deploymentId)
      jobs.go              — GetJobStatus (Asynq Inspector lookup)
      proxy.go             — Model-aware reverse proxy (Redis + DB service discovery)

infra/
  azure/main.go            — Pulumi Azure IaC (VM + k3s bootstrap)
```

---

## 2. Phase-by-Phase Status

### ✅ Phase 1: Persistence Foundation — 95% Complete

Every required component is implemented and wired correctly.

| Item | File | Status | Notes |
|------|------|--------|-------|
| PostgreSQL pool (pgx/v5) | `store/postgres.go` | ✅ | pgxpool, Ping on startup |
| Store interface | `store/store.go` | ✅ | 15 methods, clean abstraction |
| Organization CRUD | `store/postgres.go` | ✅ | Create, GetByID, GetByName |
| Cluster CRUD | `store/postgres.go` | ✅ | Create, Get, GetByName, List, UpdateStatus, UpdateDetails |
| Deployment CRUD | `store/postgres.go` | ✅ | Create, Get, UpdateStatus, UpdateServiceURL, ListActiveTargets |
| Redis client (go-redis/v9) | `cache/redis.go` | ✅ | Connects on startup, connection pool |
| Schema migrations | `migrations/001_init.sql` | ⚠️ | File exists; no automated runner — must apply manually |
| Config loading | `config/config.go` | ⚠️ | Reads env vars with silent fallbacks (no fail-fast, no `_FILE` support) |

**What's missing from Phase 1:**
- `jobs` table (required by architecture §2.2.1 and §3 schema) — **not created**
- `audit_logs` table (required by architecture §3 schema) — **not created**
- Automated migration runner (`golang-migrate` / `goose`) — **not integrated**
- Config validation on startup — **silent defaults used**

---

### ✅ Phase 2: Orchestrator Core — 70% Complete

The async job backbone is working end-to-end. The architecture's full durability requirements are not yet met.

| Item | File | Status | Notes |
|------|------|--------|-------|
| Asynq client | `worker/client.go` | ✅ | Enqueue with TaskID + MaxRetry(10) |
| Asynq Inspector | `worker/client.go` | ✅ | GetTaskInfo scans all 4 queues |
| Queue definitions | `queue/queues.go` | ✅ | critical, infra-provision, model-deploy, cleanup |
| Queue priority weights | `worker/server.go` | ✅ | critical:10, infra-provision:6, model-deploy:6, cleanup:4 |
| Task payload types | `worker/tasks/tasks.go` | ✅ | Provision, Deploy, Destroy — typed + JSON marshal |
| Worker process | `cmd/worker/main.go` | ✅ | Separate binary, configurable concurrency |
| Worker graceful shutdown | `cmd/worker/main.go` | ✅ | `signal.NotifyContext` + `server.Shutdown()` on SIGTERM |
| 202 Accepted on provision | `handlers/provision.go` | ✅ | Returns jobId, taskId, clusterId |
| 202 Accepted on deploy | `handlers/deploy.go` | ✅ | Returns jobId, taskId, deploymentId |
| Job status endpoint | `handlers/jobs.go` | ✅ | `GET /api/jobs/:id` via Inspector |
| Provision task handler | `worker/server.go` | ✅ | Sets status installing → active / failed |
| Deploy task handler | `worker/server.go` | ✅ | Sets status installing → active / failed, builds serviceURL |
| Destroy task handler | `worker/server.go` | ✅ | Sets status destroyed / failed |
| Pulumi integration | `worker/provisioner.go` | ✅ | `auto.UpsertStackLocalSource` → Up / Destroy |
| SSH kubeconfig fetch | `worker/ssh.go` | ✅ | SSH into VM, `cat /home/azureuser/client.config`, retry ×30 |
| Kubernetes model deploy | `kube/vllm.go` | ✅ | Init-container download + llama.cpp server + NodePort Service |
| Cache invalidation on deploy | `worker/server.go` | ✅ | Invalidates `proxy:model:targets:*` and `proxy:model:rr:*` |
| Deterministic task IDs | `handlers/deploy.go` | ✅ | `"deploy:" + deploymentID` — prevents duplicate enqueue |
| Full state machine (QUEUED→BUILDING→DEPLOYING→RUNNING) | — | ❌ | Only installing/active/failed exist |
| DB-backed jobs table | — | ❌ | All state lives in Redis (ephemeral) |
| Per-cluster concurrency limits | — | ❌ | Global worker concurrency only; unlimited deploys to one cluster |
| DLQ inspection endpoint | — | ❌ | Failed tasks visible in Asynq but no API to expose them |
| Progress streaming (SSE) | — | ❌ | No `GET /api/jobs/:id/stream` or SSE endpoint |
| API server graceful shutdown | `cmd/server/main.go` | ❌ | No signal handling — abrupt kill on SIGTERM |

---

### ✅ Phase 3: Universal Proxy — 80% Complete

The model-aware proxy is substantially implemented. The Architecture Plan's Gap Tracker marked this as ❌ (Static proxy only), but the actual code is far beyond that.

| Item | File | Status | Notes |
|------|------|--------|-------|
| Model field parsing | `handlers/proxy.go` | ✅ | Reads + restores body, parses `{"model": "..."}` |
| Redis-first discovery | `handlers/proxy.go` | ✅ | Key: `proxy:model:targets:<model>`, TTL 30s |
| DB fallback | `handlers/proxy.go` | ✅ | `ListActiveDeploymentTargets` on Redis miss |
| Round-robin selection | `handlers/proxy.go` | ✅ | Redis `INCR proxy:model:rr:<model>` → modulo index |
| TCP health probing | `handlers/proxy.go` | ✅ | 800ms dial timeout, filters unhealthy targets |
| Cache invalidation on unhealthy | `handlers/proxy.go` | ✅ | Removes stale targets, refetches from DB |
| Streaming-safe proxy | `handlers/proxy.go` | ✅ | `FlushInterval: -1` for SSE passthrough |
| Per-target proxy cache | `handlers/proxy.go` | ✅ | `proxyByTarget` map, lazy-init with RWMutex |
| Error JSON envelope | `handlers/proxy.go` | ✅ | `{"error":{"code":"...","message":"..."}}` |
| Empty model fallback | `handlers/proxy.go` | ✅ | Falls back to `defaultTarget` (VLLM_URL env var) |
| gRPC protocol support | — | ❌ | HTTP only; architecture specifies gRPC/HTTP dual |
| Request ID propagation to upstream | — | ❌ | No correlation header forwarded |
| HTTP health probe (not TCP) | — | ⚠️ | TCP dial only; no `/health` HTTP check |

---

### ❌ Phase 4: API Maturity & Reliability — 0% Complete

None of the Phase 4 items are implemented. These are the most critical production blockers.

| Item | Architecture Requirement | Current State |
|------|--------------------------|---------------|
| **Graceful shutdown (API server)** | SIGTERM + in-flight request draining | ❌ `cmd/server/main.go` has no signal handling |
| **Structured error types** | `internal/apierror` package, typed codes | ❌ Raw `map[string]string{"error": msg}` everywhere |
| **Custom Echo error handler** | Consistent JSON on panics/404/500 | ❌ Default Echo handler |
| **Route groups + middleware chains** | `/api/v1/` group with auth + rate-limit | ❌ All routes flat-registered on root |
| **API versioning** | All routes under `/api/v1/` | ❌ Current: `/api/deploy`, `/api/provision`, `/api/jobs/:id` |
| **Request validation** | `go-playground/validator` on all payloads | ❌ Manual empty-string checks only |
| **Request ID middleware** | UUID per request, propagated in logs | ❌ Not implemented |
| **DB migration runner** | Auto-run on startup (goose/golang-migrate) | ❌ Manual SQL execution required |
| **DB-backed job tracking** | `jobs` table as deployment source of truth | ❌ Table not created; state only in Redis |
| **Deployment state machine** | QUEUED→IN_PROGRESS→BUILDING→DEPLOYING→RUNNING/FAILED | ❌ Only `installing`/`active`/`failed` in deployments table |
| **Audit logging** | `audit_logs` table + write on every mutation | ❌ Table not created; no logging |
| **Pagination** | Cursor-based on all list endpoints | ❌ No list endpoints exist (no GET /clusters, GET /deployments) |
| **Config validation** | Fail-fast if required env vars missing | ❌ Silent defaults (e.g., falls back to `localhost:5432`) |
| **`_FILE` suffix support** | Read secret value from file path | ❌ Not implemented |
| **Domain service layer** | `provision.Service`, `deploy.Service` grouping | ❌ Handler-level wiring only in `app.go` |
| **OpenAPI specification** | Auto-generated or maintained API docs | ❌ Not started |

**Visible code smell examples from actual source:**

```go
// handlers/provision.go — inconsistent error format (string vs JSON)
return c.String(http.StatusBadRequest, "stackName is required")      // plain string
return c.JSON(http.StatusBadRequest, map[string]string{"error": ...}) // JSON map
```

```go
// config/config.go — silent fallback hides misconfiguration
databaseURL = "postgres://postgres:postgres@localhost:5432/ai_paas?sslmode=disable"
```

---

### ❌ Phase 5: Observability — 5% Complete

Only a stub `/healthz` exists. Nothing else.

| Item | Architecture Requirement | Current State |
|------|--------------------------|---------------|
| `/healthz` (liveness) | Returns OK if process is alive | ⚠️ Exists, always returns `"ok"` — no DB/Redis check |
| `/readyz` (readiness) | Returns OK only if DB + Redis are reachable | ❌ Not implemented |
| Prometheus metrics | `/metrics` endpoint: latency, queue depth, error rates | ❌ Not implemented |
| Event system | Typed internal events + channel-based pub/sub relay | ❌ Not implemented |
| Progress streaming | SSE endpoint for deployment log tailing | ❌ Not implemented |
| Distributed tracing | OpenTelemetry spans across handlers and workers | ❌ Not implemented |
| Structured logging with context | Request ID + trace ID in every log line | ❌ slog exists but no request context propagation |

---

### ❌ Phase 6: Security Hardening — 0% Complete

| Item | Architecture Requirement | Current State |
|------|--------------------------|---------------|
| **Kubeconfig encryption** | AES-256-GCM before writing to Postgres | ❌ Stored as plaintext in `clusters.kubeconfig` |
| **SSH host key verification** | Strict host key checking | ❌ `ssh.InsecureIgnoreHostKey()` used with a `// TODO` comment |
| **TLS: DB connection** | `sslmode=require` or `verify-full` | ❌ `sslmode=disable` in default config |
| **TLS: Redis connection** | TLS client config on go-redis | ❌ Plaintext connection |
| **Rate limiting** | Token bucket in Redis per org/user | ❌ Not implemented |
| **Input sanitization** | Strict schema validation on all payloads | ❌ Only minimal field presence checks |
| **Secrets not in env vars** | Support K8s secrets, vault, or `_FILE` suffix | ❌ All secrets via plain env vars |
| **RBAC foundations** | Role-based route guards | ❌ Not implemented |

**Most critical issue in the codebase today:**

```go
// kube/client.go — TLS verification disabled for all remote clusters
restConfig.TLSClientConfig.Insecure = true
restConfig.TLSClientConfig.CAData = nil

// worker/ssh.go — SSH host key checking disabled
HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Use strict checking in production
```

---

### ❌ Phase 7: Auth, RBAC & Multi-tenancy — 10% Complete

The data model has `org_id` foreign keys, but there is no enforcement layer.

| Item | Architecture Requirement | Current State |
|------|--------------------------|---------------|
| JWT/OIDC middleware | Token validation on all protected routes | ❌ Not implemented |
| RBAC route guards | Admin / operator / viewer roles | ❌ Not implemented |
| org_id query scoping | All DB queries filtered by caller's org | ❌ Default org used for all callers |
| User model | `users` table with org_id + roles | ❌ Table not created |
| Session management | JWT refresh / revocation | ❌ Not implemented |
| `org_id` FK on schema | Clusters + deployments reference org | ✅ FK exists in schema |
| Default org auto-creation | `ensureDefaultOrganization` in provision handler | ✅ Implemented as a workaround |

---

### ❌ Phase 8: Testing & CI/CD — 0% Complete

| Item | Architecture Requirement | Current State |
|------|--------------------------|---------------|
| Unit tests | >80% coverage on handlers, store, workers | ❌ Zero test files in repo |
| Interface mocking | Mock implementations of Store, Queue, Kube | ❌ Only `store.Store` interface exists; no mocks |
| Queue interface | `Queue` interface for mock injection | ❌ `*worker.Client` (concrete type) used directly |
| Kube interface | `Kube` interface for mock injection | ❌ `*kube.Client` (concrete type) used directly |
| Dockerfile | Multi-stage build for server + worker binaries | ❌ Not created |
| GitHub Actions / CI | Lint → test → build → push pipeline | ❌ `.github/workflows/` directory is empty |
| Integration tests | testcontainers for Postgres + Redis | ❌ Not started |
| golangci-lint config | `.golangci.yml` with rules | ❌ Not configured |

---

### ❌ Phase 9: HA & Operational Readiness — 0% Complete

| Item | Architecture Requirement | Current State |
|------|--------------------------|---------------|
| Multi-replica API | StatefulSet with N replicas behind LB | ❌ Single instance only |
| Redis Sentinel / Cluster | HA Redis with automatic failover | ❌ Single Redis instance |
| PostgreSQL HA | Streaming replication + Patroni | ❌ Single Postgres instance |
| Leader election | One active scheduler, N standby | ❌ Not implemented |
| DB backup / restore | Automated nightly backups, tested recovery | ❌ Not started |
| API server graceful shutdown | SIGTERM + request draining (30s timeout) | ❌ Abrupt kill |

Note: `docker-compose.yml` provides local dev infra (Postgres 18 + Redis 8) with proper healthchecks. Production HA topology is entirely absent.

---

### ❌ Phase 10: Packaging, Licensing & Distribution — 0% Complete

| Item | Architecture Requirement | Current State |
|------|--------------------------|---------------|
| Helm chart | `helm/ownllm/` with production values.yaml | ❌ Not created |
| License enforcement | BSL 1.1 + JWT tier claim validation | ❌ Not implemented |
| Feature flags | `features.IsEnabled(ctx, tier, feature)` | ❌ Not implemented |
| Tier gating | Community / Pro / Enterprise split | ❌ Not implemented |
| Semantic versioning | Git tags + changelog + GitHub releases | ❌ Not started |
| Installation documentation | Deploy guide, config reference, runbook | ❌ Only ARCHITECTURE_PLAN.md exists |

---

## 3. Architecture Gap Tracker (Updated from ARCHITECTURE_PLAN.md §7)

The original Gap Tracker in ARCHITECTURE_PLAN.md is partially outdated. The table below reflects the actual code state.

| Concern | Architecture Requirement | Code Reality | Phase |
|---------|--------------------------|--------------|-------|
| Postgres persistence | pgx/v5 pool, Store interface | ✅ Implemented | 1 |
| Redis caching | go-redis/v9 | ✅ Implemented | 1 |
| DB migrations | Automated runner | ⚠️ Manual SQL file only | 1 |
| Async job queue | Asynq client/server/queues | ✅ Implemented | 2 |
| Queue priority weights | Per-queue weights in asynq.Config | ✅ Implemented (critical:10, infra:6, deploy:6, cleanup:4) | 2 |
| Worker process | Separate cmd/worker | ✅ Implemented | 2 |
| Worker graceful shutdown | SIGTERM via signal.NotifyContext | ✅ Implemented | 2 |
| Non-blocking provision/deploy | 202 + job metadata | ✅ Implemented | 2 |
| Pulumi integration | Stack up/destroy with progress streaming | ✅ Implemented | 2 |
| SSH kubeconfig fetch | Retry-based SSH pull from VM | ✅ Implemented | 2 |
| Kubernetes model deploy | Init-container + llama.cpp + NodePort | ✅ Implemented | 2 |
| Worker state transitions | pending → installing → active/failed | ✅ Partial (not full QUEUED→RUNNING machine) | 2 |
| Job status endpoint | GET /api/jobs/:id | ✅ Implemented | 2 |
| Model cache invalidation | Worker invalidates Redis on deploy/fail | ✅ Implemented | 2 |
| Model-aware proxy routing | Parse model → resolve → proxy | ✅ Implemented | 3 |
| Service discovery | Redis-first + DB fallback | ✅ Implemented | 3 |
| Round-robin load balancing | Redis INCR counter across replicas | ✅ Implemented | 3 |
| TCP health probing | Filter unhealthy targets before routing | ✅ Implemented | 3 |
| Streaming-safe proxy | SSE passthrough (FlushInterval: -1) | ✅ Implemented | 3 |
| **DB-backed job tracking** | jobs table as durable source of truth | ❌ Redis-only (ephemeral) | 4 |
| **Full deployment state machine** | QUEUED→BUILDING→DEPLOYING→RUNNING/FAILED | ❌ Not implemented | 4 |
| **Per-cluster concurrency** | Limit concurrent ops per cluster | ❌ Unlimited parallelism | 4 |
| **DLQ visibility endpoint** | GET /api/jobs/failed | ❌ No dedicated endpoint | 4 |
| **API server graceful shutdown** | SIGTERM + request draining | ❌ Not implemented | 4 |
| **Structured error types** | internal/apierror package | ❌ Raw strings + maps | 4 |
| **Route groups + middleware** | Grouped routes with auth/rate-limit chains | ❌ Flat route registration | 4 |
| **API versioning** | /api/v1/ prefix on all routes | ❌ Unversioned paths | 4 |
| **Request validation** | go-playground/validator on all payloads | ❌ Minimal field checks | 4 |
| **Request ID middleware** | UUID correlation propagated in logs | ❌ Not implemented | 4 |
| **Config validation + _FILE** | Fail-fast startup + K8s secret file support | ❌ Silent empty defaults | 4 |
| **Domain service layer** | Group deps into domain services | ❌ Handler-level wiring only | 4 |
| **Audit trail** | audit_logs table + writes | ❌ Table missing, no logging | 4 |
| **Pagination** | Cursor-based list endpoints | ❌ No list endpoints exist | 4 |
| **Progress streaming** | SSE/polling for deploy progress | ❌ Not implemented | 4 |
| Prometheus metrics | /metrics endpoint | ❌ Not implemented | 5 |
| Health probes | /healthz + /readyz | ⚠️ /healthz exists (no DB/Redis check) | 5 |
| Event system | Typed internal events + relay | ❌ Not implemented | 5 |
| Distributed tracing | OpenTelemetry | ❌ Not implemented | 5 |
| **Kubeconfig encryption** | AES-256-GCM at rest | ❌ Stored as plaintext | 6 |
| **SSH host key checking** | Strict host verification | ❌ InsecureIgnoreHostKey() | 6 |
| TLS: DB + Redis | Encrypted internal connections | ❌ sslmode=disable, plaintext Redis | 6 |
| Rate limiting | Token bucket in Redis | ❌ Not implemented | 6 |
| Input validation | Schema validation layer | ❌ Minimal field checks only | 6 |
| Auth / OIDC | JWT/OIDC middleware | ❌ Not implemented | 7 |
| RBAC | Role-based route guards | ❌ Not implemented | 7 |
| Multi-tenant isolation | org_id scoped queries | ⚠️ FK exists; no enforcement | 7 |
| Unit tests | >80% coverage | ❌ Zero tests | 8 |
| Queue/Kube interfaces | Interfaces for mock injection | ❌ Only store.Store interface exists | 8 |
| Dockerfile | Multi-stage build | ❌ Not created | 8 |
| CI/CD pipeline | Lint/test/build/push | ❌ Not created | 8 |
| HA topology | Multi-replica, Sentinel/Cluster | ❌ Single instance | 9 |
| Helm chart | Production values.yaml | ❌ Not created | 10 |
| License enforcement | BSL 1.1 + JWT tier detection | ❌ Not implemented | 10 |
| Feature flag system | features.IsEnabled() interface | ❌ Not implemented | 10 |
| Tier gating | Community/Pro/Enterprise split | ❌ Not implemented | 10 |

**Legend:** ✅ Complete | ⚠️ Partial | ❌ Not Started

---

## 4. Critical Issues (Production Blockers)

### 🔴 BLOCKER 1 — Plaintext Kubeconfig in Database
**File:** `internal/store/postgres.go` + `001_init.sql`
The `clusters.kubeconfig` column stores raw kubeconfig YAML including cluster credentials and certificates. If the database is compromised, the attacker gains `cluster-admin` access to every provisioned cluster.

```sql
-- 001_init.sql
kubeconfig TEXT, -- encrypted in app layer or just text for now. TODO- Encrypt
```

The TODO has not been addressed. The architecture requires AES-256-GCM encryption before write and decryption after read.

---

### 🔴 BLOCKER 2 — No Durable Job State
**Files:** `worker/server.go`, `store/postgres.go`
All job state lives exclusively in Redis via Asynq. A Redis restart, eviction, or failover wipes all job history. There is no `jobs` table, so:
- You cannot answer "what deployments happened last week?"
- A worker crash mid-deploy loses the task; no idempotent resume
- The `/api/jobs/:id` endpoint fails once a completed task expires from Redis (Asynq default: 90 days, but configurable to less)

The `jobs` and `audit_logs` tables defined in the Architecture Plan's §3 schema have not been created.

---

### 🔴 BLOCKER 3 — API Server Has No Graceful Shutdown
**File:** `cmd/server/main.go`
The worker correctly uses `signal.NotifyContext` (lines 17–18 of `cmd/worker/main.go`). The API server does not:

```go
// cmd/server/main.go — current state
if err := application.Run(); err != nil { // blocks forever; SIGTERM kills immediately
    slog.Error(...)
    os.Exit(1)
}
```

On a Kubernetes pod eviction or rolling deployment, in-flight provision/deploy requests are dropped mid-execution. The expected fix:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
go func() { _ = application.Run() }()
<-ctx.Done()
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
_ = server.Shutdown(shutdownCtx)
```

---

### 🔴 BLOCKER 4 — Inconsistent and Untyped Error Responses
**Files:** `handlers/provision.go`, `handlers/deploy.go`, `handlers/jobs.go`
Error responses use at least three different formats across handlers:

```go
c.String(http.StatusBadRequest, "stackName is required")             // plain string
c.String(http.StatusInternalServerError, "Database error: ...")      // plain string
c.JSON(http.StatusBadRequest, map[string]string{"error": "..."})     // JSON map
c.JSON(http.StatusInternalServerError, map[string]string{"error": "..."}) // JSON map
```

There is no `internal/apierror` package. Clients cannot reliably parse errors. A custom Echo error handler is also absent, so panics and framework-level 404s use Echo's default format (inconsistent with handler formats).

---

### 🔴 BLOCKER 5 — Zero Test Coverage
There are no `*_test.go` files anywhere in the repository. This means:
- No safety net for any refactoring
- No regression detection
- `worker.Client` and `kube.Client` are concrete types passed directly into handlers, making them untestable without a real Redis instance and a real Kubernetes cluster

The `store.Store` interface is the only testable boundary that exists today.

---

### 🟠 HIGH — Disabled Security Checks with TODO Comments

```go
// worker/ssh.go
HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Use strict checking in production

// kube/client.go
restConfig.TLSClientConfig.Insecure = true   // no comment; silently skips TLS verification
restConfig.TLSClientConfig.CAData = nil
```

Both of these allow man-in-the-middle attacks on the SSH and Kubernetes API connections respectively.

---

### 🟠 HIGH — Config Silently Falls Back to Localhost Defaults

```go
// config/config.go
if databaseURL == "" {
    databaseURL = "postgres://postgres:postgres@localhost:5432/ai_paas?sslmode=disable"
}
```

In a misconfigured production environment, the application starts successfully but connects to a non-existent local database. The startup logs show the URL, but there is no fail-fast gate. The architecture requires fail-fast validation and `_FILE` suffix support for Kubernetes secrets.

---

## 5. Dependency Audit

### Currently in go.mod (confirmed)
| Package | Version | Usage |
|---------|---------|-------|
| `labstack/echo/v4` | v4.15.0 | HTTP framework |
| `jackc/pgx/v5` | v5.8.0 | PostgreSQL driver |
| `redis/go-redis/v9` | v9.17.3 | Redis client |
| `hibiken/asynq` | v0.25.1 | Job queue (listed as `// indirect` — should be direct) |
| `google/uuid` | v1.6.0 | UUID generation |
| `pulumi/pulumi/sdk/v3` | v3.218.0 | Infrastructure as Code |
| `k8s.io/client-go` | v0.35.0 | Kubernetes API |
| `k8s.io/api` | v0.35.0 | Kubernetes types |
| `golang.org/x/crypto` | v0.47.0 | SSH client |

**Note:** `hibiken/asynq` is marked `// indirect` in `go.mod` but is the primary job queue library. It should be promoted to a direct dependency:
```
go get github.com/hibiken/asynq@v0.25.1
```

### Required — Not Yet Added
| Package | Purpose | Priority |
|---------|---------|----------|
| `go-playground/validator/v10` | Request payload validation | P0 |
| `golang-jwt/jwt/v5` | JWT middleware | P0 |
| `pressly/goose/v3` OR `golang-migrate/migrate/v4` | DB migration runner | P0 |
| `prometheus/client_golang` | Metrics endpoint | P1 |
| `stretchr/testify` | Test assertions + mocking | P1 |
| `caarlos0/env/v10` | Struct-tag env parsing with validation | P1 |
| `opentelemetry-go` | Distributed tracing | P2 |
| `testcontainers/testcontainers-go` | Integration testing with real DB/Redis | P2 |

---

## 6. Missing Files (Must Create)

| File | Purpose | Priority |
|------|---------|----------|
| `internal/apierror/errors.go` | Typed error codes + APIError struct | P0 |
| `internal/apierror/handler.go` | Custom Echo HTTP error handler | P0 |
| `internal/crypto/encryption.go` | AES-256-GCM helpers for kubeconfig | P0 |
| `internal/store/migrations/002_jobs.sql` | jobs table with state machine columns | P0 |
| `internal/store/migrations/003_audit_logs.sql` | audit_logs table | P0 |
| `internal/models/job.go` | Job struct matching jobs table | P0 |
| `internal/middleware/request_id.go` | UUID generation + context injection | P1 |
| `internal/middleware/validator.go` | Echo middleware wrapping go-playground/validator | P1 |
| `internal/service/provision/service.go` | Domain service layer for provision logic | P1 |
| `internal/service/deploy/service.go` | Domain service layer for deploy logic | P1 |
| `internal/metrics/metrics.go` | Prometheus counter/histogram registration | P1 |
| `Dockerfile` | Multi-stage build for server + worker | P1 |
| `.github/workflows/ci.yml` | Lint → test → build → push | P1 |
| `helm/ownllm/Chart.yaml` | Helm chart definition | P2 |
| `helm/ownllm/values.yaml` | Production-ready defaults | P2 |
| `helm/ownllm/templates/deployment.yaml` | K8s Deployment manifests | P2 |

---

## 7. What Works End-to-End Today

A user can do all of the following with the current codebase against a running Postgres + Redis:

```
1. POST /api/provision  → Creates cluster record in DB → Enqueues Pulumi job → Returns 202
   Worker: Pulumi Up → SSH fetch kubeconfig → UpdateClusterDetails(active)

2. POST /api/deploy     → Creates deployment record → Enqueues k8s deploy job → Returns 202
   Worker: GetCluster → NewFromKubeConfig → DeployModel (init-container + service) → UpdateDeploymentServiceURL(active)

3. GET  /api/jobs/:id   → Returns Asynq task status (pending/active/completed/failed)

4. POST /v1/chat/completions  → Parses model field → Redis lookup → DB fallback
                               → TCP health check → Round-robin target → Reverse proxy (streaming-safe)

5. POST /api/destroy    → Enqueues Pulumi destroy job → Returns 202
   Worker: Pulumi Destroy → UpdateClusterStatus(destroyed)
```

---

## 8. Recommended Implementation Order

### Week 1–2: Fix Critical Blockers (Security + Reliability)

1. **Kubeconfig encryption** — `internal/crypto/encryption.go` + update `UpdateClusterDetails` and `GetCluster`
2. **jobs table migration** — `002_jobs.sql` + `Job` model + store CRUD methods
3. **Worker writes to jobs table** — Update `handleProvisionClusterTask` and `handleDeployModelTask`
4. **API server graceful shutdown** — Add `signal.NotifyContext` to `cmd/server/main.go`
5. **SSH host key verification** — Replace `InsecureIgnoreHostKey` with known-hosts check

### Week 3: API Maturity Foundations

6. **Structured errors** — `internal/apierror/errors.go` + custom Echo error handler
7. **API versioning** — Refactor `router.go` to use `e.Group("/api/v1", ...)`
8. **Request validation** — Add `go-playground/validator` + validation middleware
9. **Request ID middleware** — `internal/middleware/request_id.go`
10. **Config fail-fast** — Validate required env vars on startup; add `_FILE` suffix reading

### Week 4: Observability

11. **`/readyz` endpoint** — Check DB ping + Redis ping before returning 200
12. **Prometheus metrics** — Request latency, queue depth, deployment success/failure counters
13. **Event system** — Typed channel-based pub/sub for `deploy.completed`, `provision.failed`, etc.
14. **Structured log context** — Propagate request ID into handler + worker logs

### Week 5–6: Testing + CI/CD

15. **Queue and Kube interfaces** — Define `queue.Enqueuer` and `kube.Deployer` interfaces
16. **Mock implementations** — `store.MockStore`, `queue.MockEnqueuer`, `kube.MockDeployer`
17. **Unit tests** — Handlers (mocked deps), store (Postgres testcontainer), worker task handlers
18. **Dockerfile** — Multi-stage: `golang:1.25-alpine` build → `alpine` runtime, non-root user
19. **GitHub Actions** — `golangci-lint` → `go test ./...` → `docker build` → push on tag

### Week 7–8: Deployment + Auth

20. **Helm chart** — `helm/ownllm/` with ConfigMap + Secret injection, resource limits, HPA
21. **JWT middleware** — Validate Bearer token on all `/api/v1/` routes
22. **TLS for DB + Redis** — `sslmode=require` in DATABASE_URL, TLS config on go-redis
23. **Rate limiting** — Redis token bucket per `org_id`, enforced in middleware

---

## 9. Summary

| Phase | Name | % Done | Critical Missing Items |
|-------|------|--------|------------------------|
| 1 | Persistence Foundation | **95%** | Migration runner, jobs/audit tables |
| 2 | Orchestrator Core | **70%** | Durable jobs table, full state machine, API server shutdown, per-cluster concurrency |
| 3 | Universal Proxy | **80%** | gRPC, HTTP health probes, request ID propagation |
| 4 | API Maturity | **0%** | Structured errors, validation, versioning, graceful shutdown, audit logs |
| 5 | Observability | **5%** | /readyz, Prometheus, event system, tracing |
| 6 | Security Hardening | **0%** | Kubeconfig encryption, TLS, rate limiting, SSH host verification |
| 7 | Auth, RBAC, Multi-tenancy | **10%** | JWT, RBAC, org_id enforcement |
| 8 | Testing & CI/CD | **0%** | All tests, interfaces for mocking, Dockerfile, GitHub Actions |
| 9 | HA & Operational Readiness | **0%** | Multi-replica, Sentinel, PG replication, backups |
| 10 | Packaging & Distribution | **0%** | Helm chart, license enforcement, tier gating |
| **TOTAL** | | **~45%** | **Phases 4–10 need focused engineering** |

**Bottom line:** The core flow (provision cluster → deploy model → route inference) works end-to-end and is architecturally sound. The codebase needs production hardening — particularly kubeconfig encryption, durable job state, structured errors, tests, and graceful shutdown — before it can handle real workloads safely.
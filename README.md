<div align="center">

  <h1>Tark</h1>
  <p><b>A self-hosted platform for provisioning cloud infrastructure and deploying AI models — built with Go.</b></p>

  <p>
    <a href="https://golang.org/doc/go1.21"><img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go" alt="Go Version"></a>
    <a href="https://kubernetes.io/"><img src="https://img.shields.io/badge/Kubernetes-Compatible-326CE5?style=flat&logo=kubernetes" alt="Kubernetes"></a>
    <a href="https://www.pulumi.com/"><img src="https://img.shields.io/badge/IaC-Pulumi-8A3391?style=flat&logo=pulumi" alt="Pulumi"></a>
    <a href="https://github.com/hibiken/asynq"><img src="https://img.shields.io/badge/Queue-Asynq-red.svg" alt="Asynq"></a>
    <a href="#"><img src="https://img.shields.io/badge/Status-Beta-orange" alt="Status"></a>
    <a href="#"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License"></a>
  </p>

</div>

---

## What is Tark?

Tark is a Go-based backend system that does two main things:

1. **Provisions cloud infrastructure** — it creates Kubernetes clusters on Azure using Pulumi (Infrastructure as Code), entirely through an HTTP API call. No manual clicking in the Azure portal.

2. **Deploys AI models** — once a cluster is live, it deploys model-serving containers (like vLLM or llama.cpp) onto that cluster and exposes them behind a single unified API endpoint.

Think of it as a simplified, self-hosted version of what cloud providers like AWS SageMaker or Azure ML do — but one you own entirely and can learn from.

> **Why build this?** It's a practical way to learn distributed systems, Kubernetes, async job processing, reverse proxying, and cloud infrastructure — all in one real project. Yes, I have used AI tools for this project. I have used them for learning as well as building. Again, this is a personal project for my own learning.

---

## Table of Contents

- [How It Works](#how-it-works)
- [Key Concepts](#key-concepts)
- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Prerequisites](#prerequisites)
- [Getting Started (Local Dev)](#getting-started-local-dev)
- [API Reference](#api-reference)
- [Project Structure](#project-structure)
- [Common Makefile Commands](#common-makefile-commands)
- [Status & Roadmap](#status--roadmap)
- [Troubleshooting](#troubleshooting)

---

## How It Works

Here's the full lifecycle from zero to a running model:

```
You call POST /api/provision
        |
        v
API Server receives the request, creates a cluster record in PostgreSQL,
then drops a job onto the Redis queue (via Asynq).
        |
        v
The Background Worker picks up that job, runs Pulumi to create
the Azure VM + networking, then SSHes in to install Kubernetes (k3s).
        |
        v
Worker updates the cluster record in Postgres: status = "active",
stores the kubeconfig and public IP.
        |
        v
You call POST /api/deploy
        |
        v
API Server finds an active cluster, creates a deployment record,
enqueues another job.
        |
        v
Worker SSHes into the cluster, applies Kubernetes manifests
(Deployment + Service) for the model container.
        |
        v
You call POST /v1/chat/completions
        |
        v
API Server reverse-proxies your request to the live model endpoint,
discovered via Redis (fast) or PostgreSQL (fallback).
```

---

## Key Concepts

If some of these are new to you, here's a quick plain-English explanation:

| Concept | What it means in Tark |
|---|---|
| **Control Plane** | The API server + worker — the "brain" that orchestrates everything |
| **Inference Plane** | The Kubernetes cluster running the actual AI model |
| **Asynq** | A Redis-backed job queue — lets the API return instantly while heavy work (Pulumi, SSH) runs in the background |
| **Pulumi Automation API** | Lets you run infrastructure code (`pulumi up`) from inside a Go program, not just the CLI |
| **Reverse Proxy** | The API forwards `/v1/chat/completions` to whichever cluster is serving your model |
| **k3s** | A lightweight Kubernetes distribution — easier to install on a single VM than full K8s |
| **Foreign Key Constraint** | A database rule that prevents saving a deployment if its `cluster_id` doesn't exist — this is a safety guard, not a bug |

---

## Architecture

The system is split into two logical planes that never block each other:

```
+--------------------------------------------------------------+
|                        Control Plane                         |
|                                                              |
|   Client --> Echo HTTP API --> PostgreSQL (source of truth)  |
|                    |                                         |
|                    +-> Redis Queue (Asynq)                   |
|                               |                             |
|                               v                             |
|                    Background Worker                         |
|                    +-- ProvisionTask (Pulumi + SSH)          |
|                    +-- DeployTask (kubectl apply via SSH)    |
|                               |                             |
+-------------------------------|------------------------------+
                                |
                                v
+--------------------------------------------------------------+
|                       Inference Plane                        |
|                                                              |
|   Azure VM --> k3s Cluster --> Model Pod (vLLM / llama.cpp)  |
|                                                              |
+--------------------------------------------------------------+
```

**Why the separation?**
Provisioning a cloud VM takes 2–5 minutes. If the API waited for that, users would get timeouts. By queuing the work, the API returns a `jobId` immediately and the worker does the heavy lifting asynchronously.

---

## Tech Stack

| Layer | Technology | Why |
|---|---|---|
| **HTTP API** | [Echo v4](https://echo.labstack.com/) | Fast, simple Go web framework |
| **Database** | PostgreSQL (via [pgx v5](https://github.com/jackc/pgx)) | Durable source of truth for clusters, deployments |
| **Job Queue** | [Asynq](https://github.com/hibiken/asynq) + Redis | Async task processing with retries and deduplication |
| **IaC** | [Pulumi Automation API](https://www.pulumi.com/docs/using-pulumi/automation-api/) | Programmatic infrastructure provisioning |
| **Cloud** | Azure (via Pulumi Azure provider) | VM, networking, Public IPs |
| **Container Runtime** | k3s on Azure VM | Lightweight Kubernetes |
| **Service Discovery** | Redis (cache) + PostgreSQL (fallback) | Fast lookup of active model endpoints |
| **Language** | Go 1.21+ | Fast, simple, great for networked services |

---

## Prerequisites

Before running anything, make sure you have:

- **Go 1.21+** — [Install guide](https://golang.org/doc/install)
- **Docker & Docker Compose** — for local Postgres + Redis
- **Pulumi CLI** — [Install guide](https://www.pulumi.com/docs/install/)
- **Azure CLI** (`az`) — logged in with `az login`
- **Azure Subscription** — needed if you want to actually provision VMs

You do **not** need a live Azure account to run the server and worker locally. The provisioning tasks will just fail gracefully if Azure credentials aren't configured.

---

## Getting Started (Local Dev)

### Step 1 — Clone and install dependencies

```bash
git clone <your-repo-url>
cd Kubernetes
go mod download
```

### Step 2 — Start local backing services (Postgres + Redis)

```bash
make dev-up
```

This runs `docker compose up -d` and starts:
- PostgreSQL on `localhost:5432`
- Redis on `localhost:6379`

### Step 3 — Apply the database schema

```bash
make migrate-init
```

This runs the SQL in `internal/store/migrations/001_init.sql` which creates the `organizations`, `clusters`, and `deployments` tables.

> **Important:** If you wipe the database and re-run migrations, **all existing cluster and deployment records are deleted**. Any requests that reference old cluster UUIDs will fail with a foreign key error — this is expected and correct behavior.

### Step 4 — Build the binaries

```bash
make build
```

This compiles both `bin/server` and `bin/worker`.

### Step 5 — Run the API server and worker

Open two terminals:

```bash
# Terminal 1
make run-server-bin

# Terminal 2
make run-worker-bin
```

The server starts on `http://localhost:8080`.

### Step 6 — Verify it's working

```bash
curl http://localhost:8080/healthz
```

You should get a `200 OK`.

---

## API Reference

### Infrastructure Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/provision` | Provision a new Kubernetes cluster on Azure |
| `POST` | `/api/destroy` | Destroy a provisioned cluster |
| `GET` | `/api/clusters` | List all clusters |

**Example — Provision a cluster:**
```bash
curl -X POST http://localhost:8080/api/provision \
  -H "Content-Type: application/json" \
  -d '{"stackName": "my-cluster", "region": "southindia"}'
```

Response:
```json
{
  "jobId": "...",
  "taskId": "...",
  "status": "queued"
}
```

---

### Model Deployment

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/deploy` | Deploy a model to an active cluster |
| `GET` | `/api/deployments` | List all deployments |
| `DELETE` | `/api/deployments/:id` | Tear down a specific deployment |

**Example — Deploy a model (uses defaults if fields are omitted):**
```bash
curl -X POST http://localhost:8080/api/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "name": "tinyllama",
    "namespace": "default",
    "nodePort": 30000
  }'
```

> If you have multiple clusters, you can pass `"clusterId": "<uuid>"` to target a specific one. If omitted, Tark automatically picks the first `active` cluster for the default organization.

---

### Inference Proxy

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/chat/completions` | OpenAI-compatible inference — proxied to your model |

**Example:**
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "tinyllama",
    "messages": [{"role": "user", "content": "Explain Kubernetes in one sentence."}]
  }'
```

---

### Health

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/healthz` | Liveness check — returns `200 OK` if the server is up |

---

## Project Structure

```
.
├── cmd/
│   ├── server/          # API server entry point (starts Echo HTTP server)
│   └── worker/          # Background worker entry point (starts Asynq workers)
│
├── internal/
│   ├── app/             # Wires everything together (DI / setup)
│   ├── cache/           # Redis wrappers for fast service discovery
│   ├── config/          # Reads environment variables into a Config struct
│   ├── http/
│   │   └── handlers/    # HTTP handler functions (one file per domain)
│   ├── kube/            # Kubernetes client + manifest builders
│   ├── models/          # Shared Go structs (Cluster, Deployment, Organization)
│   ├── queue/           # Asynq queue names and task type constants
│   ├── store/
│   │   ├── postgres.go        # All PostgreSQL queries (no ORM)
│   │   ├── store.go           # Store interface (makes testing easier)
│   │   └── migrations/
│   │       └── 001_init.sql   # Initial schema — run this once after wiping the DB
│   └── worker/
│       └── tasks/       # Task handler implementations (Pulumi, SSH, Kubernetes)
│
├── infra/
│   └── azure/           # Pulumi program (Go) that defines Azure resources
│
├── docs/                # Architecture diagrams and planning notes
├── docker-compose.yml   # Local dev services (Postgres + Redis)
└── Makefile             # All common dev commands
```

---

## Common Makefile Commands

```bash
make help            # Show all available commands

# Development
make dev-up          # Start Postgres + Redis via Docker Compose
make dev-down        # Stop Docker Compose services
make dev-logs        # Tail Docker Compose logs
make migrate-init    # Apply initial DB schema

# Build & Run
make build           # Build both server and worker binaries
make run-server-bin  # Build + run the API server (port 8080)
make run-worker-bin  # Build + run the background worker

# Code Quality
make fmt             # Format all Go code (go fmt)
make vet             # Run go vet (catches common mistakes)
make test            # Run all tests
make tidy            # Sync go.mod and go.sum

# Quick API Tests (make sure server is running)
make api-provision   # Trigger cluster provisioning
make api-deploy      # Deploy the default tinyllama model
make api-chat        # Send a test chat completion request
make api-destroy     # Destroy the test cluster
```

---

## Status & Roadmap

The core flow (provision → deploy → proxy) works end-to-end. The codebase is approximately **45% complete** against the full architecture target. What's done is solid; what's missing is production hardening.

### Completed

- PostgreSQL store for organizations, clusters, deployments
- Redis + Asynq job queue with priority-weighted queues (critical / infra / deploy / cleanup)
- Pulumi Automation API integration (Azure VM + k3s provisioning)
- SSH-based kubeconfig fetch with retry
- Kubernetes manifest deployment via SSH (init-container model download + NodePort service)
- OpenAI-compatible inference reverse proxy with SSE streaming support
- Redis-first service discovery with PostgreSQL fallback
- Round-robin load balancing with TCP health probing
- Cache invalidation on deploy/fail
- Worker graceful shutdown (SIGTERM handled)
- Cluster validation on deploy — rejects requests with invalid or non-existent cluster IDs

### In Progress

- [ ] API server graceful shutdown (SIGTERM + in-flight request draining)
- [ ] Structured error types (`internal/apierror` package, consistent JSON across all handlers)
- [ ] Durable `jobs` table in PostgreSQL — currently all job state lives in Redis (ephemeral)
- [ ] `/readyz` readiness probe that checks DB + Redis before returning 200
- [ ] Full deployment state machine (QUEUED → BUILDING → DEPLOYING → RUNNING / FAILED)

### Planned

**Security (highest priority)**
- [ ] Kubeconfig encryption at rest (AES-256-GCM before writing to `clusters.kubeconfig`)
- [ ] SSH strict host key verification (currently uses `InsecureIgnoreHostKey`)
- [ ] TLS for database connections (`sslmode=require` instead of `sslmode=disable`)
- [ ] TLS for Redis connections
- [ ] Input validation on all request payloads (`go-playground/validator`)
- [ ] Rate limiting per organization (Redis token bucket)
- [ ] `_FILE` suffix support for secrets (read secret value from a file path, like Kubernetes does)
- [ ] Config fail-fast on startup — reject missing required env vars instead of silently falling back to localhost defaults

**API Maturity**
- [ ] API versioning — move all routes to `/api/v1/` prefix
- [ ] Route groups with middleware chains (auth, rate-limit, request ID)
- [ ] Request ID middleware — UUID per request, propagated through logs
- [ ] Custom Echo error handler — consistent JSON shape on panics, 404s, 500s
- [ ] Audit log table (`audit_logs`) — write an entry on every create/update/delete
- [ ] Pagination on list endpoints (cursor-based)
- [ ] Dead-letter queue (DLQ) visibility endpoint — expose failed Asynq tasks via API
- [ ] Progress streaming endpoint (SSE) for long-running deploy/provision jobs
- [ ] Domain service layer — group handler logic into `provision.Service` and `deploy.Service`
- [ ] OpenAPI specification

**Observability**
- [ ] Prometheus `/metrics` endpoint — request latency, queue depth, deployment success/failure counters
- [ ] OpenTelemetry distributed tracing across handlers and workers
- [ ] Structured log context — propagate request ID and trace ID into every log line
- [ ] Event system — typed internal channel-based pub/sub for events like `deploy.completed` and `provision.failed`

**Auth & Multi-tenancy**
- [ ] JWT / OIDC authentication middleware on all protected routes
- [ ] Role-Based Access Control (RBAC) — admin / operator / viewer roles
- [ ] `org_id` query scoping — all DB queries filtered by the caller's organization
- [ ] User model and session management

**Testing & CI/CD**
- [ ] Unit tests — zero test files currently exist in the repo
- [ ] Queue and Kube interfaces — needed so handlers can be tested without a real Redis or cluster
- [ ] Mock implementations for Store, Queue, and Kube
- [ ] Integration tests using testcontainers (real Postgres + Redis in CI)
- [ ] Dockerfile — multi-stage build for server and worker binaries
- [ ] GitHub Actions CI pipeline — lint → test → build → push on tag

**Operational Readiness**
- [ ] Automated DB migration runner (`goose` or `golang-migrate`) — currently requires manual SQL execution
- [ ] Helm chart for production Kubernetes deployment
- [ ] Multi-replica API server support
- [ ] Redis Sentinel / Cluster for high availability
- [ ] PostgreSQL streaming replication
- [ ] Automated database backups

---

## Troubleshooting

**`foreign key constraint "deployments_cluster_id_fkey"` error on POST /api/deploy**

> This means the `cluster_id` being used doesn't exist in the `clusters` table. Most commonly happens after wiping and re-migrating the database. Check that an active cluster exists:
> ```bash
> docker exec -i ai_paas_postgres psql -U postgres -d ai_paas \
>   -c "SELECT id, name, status FROM clusters;"
> ```
> If empty, you need to provision a cluster first via `POST /api/provision`.

---

**`column "namespace" does not exist` error**

> Your database has a stale `deployments` table from before the `namespace` column was added. Wipe and re-run migrations:
> ```bash
> make dev-down && make dev-up && make migrate-init
> ```

---

**Worker picks up a job but nothing happens on the cluster**

> Check that the cluster's `public_ip` and `kubeconfig` are populated in the database. If the Pulumi provisioning task failed, the worker may have recorded an error. Check `app.log` or the worker terminal output.

---

**Port 5432 or 6379 already in use**

> You likely have a system-level Postgres or Redis running. Run `make dev-down` which also attempts to kill those processes, or stop them manually:
> ```bash
> sudo systemctl stop postgresql redis
> ```

---

## License

MIT — see [LICENSE](./LICENSE) for details.
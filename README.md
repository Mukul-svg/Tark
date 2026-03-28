<div align="center">
  <h1>Tark</h1>
  <p><b>Enterprise Self-Hosted AI Platform & Control Plane</b></p>
  
  <p>
    <a href="https://golang.org/doc/go1.21"><img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go" alt="Go Version"></a>
    <a href="https://kubernetes.io/"><img src="https://img.shields.io/badge/Kubernetes-Compatible-326CE5?style=flat&logo=kubernetes" alt="Kubernetes"></a>
    <a href="https://github.com/hibiken/asynq"><img src="https://img.shields.io/badge/Queue-Asynq-red.svg" alt="Asynq"></a>
    <a href="#"><img src="https://img.shields.io/badge/License-BSL_1.1-blue.svg" alt="License"></a>
    <a href="#"><img src="https://img.shields.io/badge/Status-Beta-orange" alt="Status"></a>
  </p>
</div>

---

**Tark** is a distributed, Go-based control plane for provisioning Kubernetes clusters, deploying model-serving workloads, and proxying inference requests through a single unified API surface. It decouples the management APIs from the GPU-backed inference execution, providing a secure, observable, and multi-tenant foundation for private AI infrastructure.

## 📑 Table of Contents

- [Key Features](#-key-features)
- [Architecture Overview](#-architecture-overview)
- [Design Principles](#-design-principles)
- [Getting Started](#-getting-started)
- [API Reference](#-api-reference)
- [Project Structure](#-project-structure)
- [Status & Roadmap](#-status--roadmap)

---

## 🚀 Key Features

- **Automated Infrastructure Provisioning**: Asynchronously provision clusters via Pulumi Automation API (Azure backend support).
- **Model Deployment Orchestration**: Automatically deploy models (vLLM, llama.cpp) to Kubernetes.
- **Universal Inference Proxy**: `POST /v1/chat/completions` directly routes to the active model backend.
- **Inference-Aware Load Balancing**: Dynamic service discovery using Redis with PostgreSQL durable fallback.
- **Durable Async Execution**: Redis-backed queue (`asynq`) for infrastructure tasks with prioritized workers.
- **Multi-Tenant Ready**: Abstracted organizational boundaries, API routing, and deployment tracking.

---

## 🏛️ Architecture Overview

Tark operates on a divided architecture, ensuring that traffic scaling at the gateway does not overload inference clusters, and management operations don't block token generation.

```mermaid
flowchart TB
    %% Styling Definitions
    classDef client fill:#f5f5f5,stroke:#333,stroke-width:2px,color:#333;
    classDef gateway fill:#e1f5fe,stroke:#01579b,stroke-width:2px,color:#01579b;
    classDef api fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px,color:#2e7d32;
    classDef db fill:#fff3e0,stroke:#e65100,stroke-width:2px,color:#e65100;
    classDef worker fill:#f3e5f5,stroke:#4a148c,stroke-width:2px,color:#4a148c;
    classDef model fill:#ffebee,stroke:#c62828,stroke-width:2px,color:#c62828;
    classDef platform fill:#efebe9,stroke:#4e342e,stroke-width:2px,color:#4e342e;

    Client["Client Apps / SDKs / UI"]:::client --> Edge["Edge AI Gateway / API Gateway"]:::gateway
    Edge --> Auth["Auth / Rate Limit / Policy"]:::gateway
    Auth --> Router["Model Router / Control API"]:::api

    subgraph "Control Plane"
        direction TB
        Router --> API["Go API Server"]:::api
        API --> Redis[("Redis")]:::db
        API --> Postgres[("PostgreSQL")]:::db
        API --> Queue["Asynq Queue"]:::worker
        Queue --> Worker["Provision / Deploy Workers"]:::worker
        API --> Catalog["Model Catalog / Routing Registry"]:::api
        API --> Audit["Audit / Jobs / Usage Metadata"]:::api
    end

    subgraph "Inference Plane"
        direction TB
        Worker --> ClusterGW["Per-Cluster Inference Gateway"]:::gateway
        Router --> ClusterGW
        ClusterGW --> PoolA["Inference Pool A"]:::model
        ClusterGW --> PoolB["Inference Pool B"]:::model
        PoolA --> ModelA["vLLM / llama.cpp / model server"]:::model
        PoolB --> ModelB["vLLM / llama.cpp / model server"]:::model
    end

    subgraph "Platform Services"
        direction TB
        ModelStore["Model Registry / Object Storage"]:::platform --> Worker
        O11y["Metrics / Logs / Traces"]:::platform --> API
        O11y --> Worker
        O11y --> ClusterGW
        Security["Secrets / KMS / Policy Engine"]:::platform --> API
        Security --> Worker
        Security --> ClusterGW
    end
```

---

## 🧠 Design Principles

- **Control vs. Inference Plane**: Keep management APIs separated from GPU-backed inference.
- **Two-Tier Gateway Model**: Centralized edge gateway for auth/policy, and per-cluster gateways for model-aware balancing.
- **Inference-Aware Routing**: Route by model identity, upstream health, and cache locality.
- **Durable Orchestration**: Redis powers the queue, but PostgreSQL remains the absolute source of truth for deployments.
- **Security by Default**: Isolated tenant routing, encrypted kubeconfigs, and TLS boundaries.
- **Model Portability**: Compatible with diverse runtimes (vLLM, llama.cpp) behind a standard API contract.

---

## 🛠️ Getting Started

### Prerequisites

- **Go** (1.21+)
- **Docker & Docker Compose**
- **PostgreSQL & Redis**
- **Pulumi CLI** (with Azure credentials if provisioning infrastructure)
- **Kubernetes Access** (if deploying workloads)

### Local Development Setup

1. **Start local dependencies**
   ```bash
   docker compose up -d
   ```

2. **Apply migrations**
   At the moment migrations are manual. Apply the initial schema:
   `internal/store/migrations/001_init.sql`

3. **Run the API Server**
   ```bash
   go run ./cmd/server
   ```

4. **Run the Background Worker**
   ```bash
   go run ./cmd/worker
   ```

---

## 📡 API Reference

**Management API**
- `POST /api/provision` - Asynchronously provision infrastructure.
- `POST /api/deploy` - Deploy a model bundle.
- `POST /api/destroy` - Tear down cluster infrastructure.
- `GET /api/jobs/:id` - Fetch job status.

**Inference API**
- `POST /v1/chat/completions` - OpenAI-compatible completion proxy.

**Platform**
- `GET /healthz` - Liveness probe.

---

## 📁 Project Structure

```text
.
├── cmd/
│   ├── server/                  # API server entrypoint
│   └── worker/                  # Background worker entrypoint
├── internal/
│   ├── app/                     # Controller wiring & DI
│   ├── cache/                   # Fast-path Redis wrappers 
│   ├── config/                  # Environment Configuration
│   ├── http/                    # API Router & Handlers
│   ├── kube/                    # Kubernetes payload & client logic
│   ├── models/                  # Domain abstractions
│   ├── queue/                   # Asynq Queue names & typed tasks
│   ├── store/                   # PostgreSQL Durable Store
│   └── worker/                  # Task implementations (Pulumi, SSH)
├── infra/
│   └── azure/                   # Infrastructure-as-Code definitions
├── docs/                        # Architecture & Planning documents
└── docker-compose.yml           # Local dev backing services
```

---

## 📅 Status & Roadmap

The Tark core control plane is functionally operational for development and local testing. We are actively moving towards production readiness.

<details>
<summary><b>View Detailed Release Roadmap</b></summary><br>

### ✅ Core Foundation (Completed)
- PostgreSQL & Redis integration with dynamic target lookups.
- Asynq worker orchestration for Provision/Deploy/Destroy tasks.
- `pulumi up` workflows and automatic kubeconfig fetching.
- Active Reverse Proxy with TCP health probes and round-robin balancing.

### 🚧 Reliability & Observability (In Progress)
- [ ] Implement API Server graceful shutdown protocols.
- [ ] Centralized structured `apierror` definitions and middleware.
- [ ] Migrate async deployment records to a durable `jobs` DB table.
- [ ] Integrate OpenTelemetry distributed tracing and `prometheus/metrics`.
- [ ] Dedicated `/readyz` probes.

### 🔒 Security & Multi-Tenancy (Planned)
- [ ] JWT/OIDC Authentication boundaries.
- [ ] Role-Based Access Control (RBAC) and strict Org isolation.
- [ ] Encrypted payload and kubeconfig storage at rest.

</details>

---

## 📄 License

This project architecture is governed by the principles outlined in our documentation.
# Copilot Instructions for OwnLLM (simplek8)
## CODING STANDARDS
- Use Go 1.25+ features and syntax.
- Follow standard Go formatting (`gofmt`).
- Use descriptive variable and function names.
- Keep functions small and focused (ideally < 50 lines).
- Handle errors explicitly (no `panic` in production code).
- Use context for cancellation and timeouts in all long-running operations.
- Make sure to close any resources (DB connections, HTTP responses) properly.
- Follow all existing patterns in the codebase for consistency (e.g., how we structure handlers, workers, etc.).
- Always create/update knowlege document for any tech you are using with architecture diagram, flow, code snippets, and explanations. This is crucial for maintainability and onboarding new developers.
- Follow best coding practices for the specific libraries and frameworks we use (Echo, pgx, Pulumi, etc.). Refer to their official documentation for guidance.
- When in doubt, look at existing code in the repo for examples of how to implement a feature or structure a component. Consistency is key.
- For all tech, always refer to the official documentation and best practices. If you are using a library or framework, make sure to follow their recommended patterns and conventions. This ensures that our code is maintainable and leverages the full capabilities of the tools we are using.

## Project Overview
This is a sophisticated Go-based platform ("OwnLLM") that manages the lifecycle of private AI infrastructure. It combines a REST API, a Kubernetes controller-like logic, and an Infrastructure-as-Code runner (Pulumi Automation API).

## 1. Architecture & Boundaries

### Control Plane (Root Module: `simplek8`)
- **Role**: The management server exposing the API.
- **Entry Point**: `cmd/server/main.go`.
- **Core Components**:
  - `internal/http`: Echo web server handling API requests (`/api/provision`, `/v1/chat/completions`).
  - `internal/kube`: `client-go` wrapper for talking to K8s clusters. *Crucial*: This client is dynamic and can switch target clusters at runtime.
  - `internal/worker`: Wraps the Pulumi Automation API to execute infrastructure changes programmatically.
  - `internal/http/handlers/proxy.go`: Proxies OpenAI-compatible requests to the backend inference cluster.

### Infrastructure (Module: `ownllm`)
- **Location**: `infra/azure/`.
- **Role**: Defines the Azure resources (AKS, VMs) via Pulumi.
- **Usage**: This code is **executed** by the Control Plane via Pulumi SDK, not imported as a library.
- **Convention**: Treat this as a separate module. Edits here affect the infrastructure definition, edits in `internal/worker` affect *how* it is deployed.

## 2. Key Tech Stack & Conventions
- **Language**: Go 1.25+.
- **Web Framework**: Echo v4 (`github.com/labstack/echo/v4`).
- **Database**: Postgres (`pgx/v5`) - *Note*: Use migrations in `internal/store/migrations`.
- **Infrastructure**: Pulumi SDK v3 + Azure Native provider.
- **Logging**: Use `log/slog` (standard library).
- **Concurrency**: Operations like provisioning are long-running. The `internal/worker` package uses Pulumi's streaming output to capture progress.

## 3. Important Development Patterns

### Dynamic Kubernetes Client
The app does not just connect to one cluster. It handles provisioning a new cluster and then *switching* its internal client to point to it.
- **Pattern**: See `handlers.ProvisionHandler` and `kube.Client`.
- **Context**: When writing code that uses `h.appClient`, remember it might be pointing to a newly provisioned cluster, not necessarily the local one.

### Pulumi Automation
We do not use the `pulumi` CLI for app operations.
- **Pattern**: `worker.ProvisionCluster` uses `auto.UpsertStackLocalSource`.
- **Inputs**: Configuration is passed via maps (e.g., `vmSize`, `location`).
- **Outputs**: We capture generic outputs (like `publicIp`) to configure the Kube client.

### Inference Proxy
The request path for AI inference is:
`User -> POST /v1/chat/completions -> proxyHandler -> Reverse Proxy -> Backend Service (vLLM)`
- **File**: `internal/http/handlers/proxy.go`.
- **Config**: Relies on `VLLM_URL` env var or dynamic service discovery.

## 4. Workflows

### Build & Run
- **Control Plane**: `go run cmd/server/main.go`
- **Infrastructure**: To test infra code in isolation, cd to `infra/azure` and use `pulumi up`.

### Provisioning Flow
1. API receives `POST /api/provision`.
2. Handler invokes `worker.ProvisionCluster`.
3. Worker runs Pulumi up on `infra/azure`.
4. Resulting IP is used to reconfigure the K8s client.

## 5. File Structure Reference
- `internal/app/app.go`: Dependency injection and wiring.
- `internal/worker/provisioner.go`: logic for driving Pulumi.
- `internal/http/router.go`: Route definitions.
- `infra/azure/main.go`: The actual infrastructure definition.

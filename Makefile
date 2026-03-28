APP_NAME := tark
BINARY_DIR := bin
SERVER_BIN := $(BINARY_DIR)/server
WORKER_BIN := $(BINARY_DIR)/worker

PORT ?= 8080
NAMESPACE ?= default
REDIS_ADDR ?= localhost:6379
REDIS_PASSWORD ?=
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/ai_paas?sslmode=disable
WORKER_CONCURRENCY ?= 10

MIGRATION_INIT := internal/store/migrations/001_init.sql

GO ?= go
DOCKER_COMPOSE ?= docker compose

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make server         - Run API server"
	@echo "  make worker         - Run background worker"
	@echo "  make dev            - Run server and worker in separate terminals manually"
	@echo "  make dev-up         - Start local Postgres and Redis with Docker Compose"
	@echo "  make dev-down       - Stop local Docker Compose services"
	@echo "  make dev-logs       - Follow Docker Compose logs"
	@echo "  make migrate-init   - Apply initial SQL schema using psql"
	@echo "  make build          - Build server and worker binaries"
	@echo "  make build-server   - Build server binary"
	@echo "  make build-worker   - Build worker binary"
	@echo "  make run-server-bin - Run built server binary"
	@echo "  make run-worker-bin - Run built worker binary"
	@echo "  make fmt            - Format Go code"
	@echo "  make vet            - Run go vet"
	@echo "  make test           - Run tests"
	@echo "  make tidy           - Run go mod tidy"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make api-provision  - Trigger cluster provision via API"
	@echo "  make api-deploy     - Trigger model deploy via API"
	@echo "  make api-chat       - Send a chat test request to proxy"
	@echo "  make api-destroy    - Trigger cluster destroy via API"

.PHONY: server
server:
	PORT=$(PORT) \
	NAMESPACE=$(NAMESPACE) \
	DATABASE_URL=$(DATABASE_URL) \
	REDIS_ADDR=$(REDIS_ADDR) \
	REDIS_PASSWORD=$(REDIS_PASSWORD) \
	$(GO) run ./cmd/server

.PHONY: worker
worker:
	DATABASE_URL=$(DATABASE_URL) \
	REDIS_ADDR=$(REDIS_ADDR) \
	REDIS_PASSWORD=$(REDIS_PASSWORD) \
	WORKER_CONCURRENCY=$(WORKER_CONCURRENCY) \
	$(GO) run ./cmd/worker

.PHONY: dev
dev:
	@echo "Use 'make server' and 'make worker' in separate terminals."

.PHONY: dev-up
dev-up:
	$(DOCKER_COMPOSE) up -d

.PHONY: dev-down
dev-down:
	$(DOCKER_COMPOSE) down
	-sudo systemctl stop postgresql redis redis-server || true
	-sudo fuser -k 5432/tcp 6379/tcp || true

.PHONY: dev-logs
dev-logs:
	$(DOCKER_COMPOSE) logs -f

.PHONY: migrate-init
migrate-init:
	docker exec -i ai_paas_postgres psql -U postgres -d ai_paas < $(MIGRATION_INIT)

.PHONY: build
build: build-server build-worker

.PHONY: build-server
build-server:
	mkdir -p $(BINARY_DIR)
	$(GO) build -o $(SERVER_BIN) ./cmd/server

.PHONY: build-worker
build-worker:
	mkdir -p $(BINARY_DIR)
	$(GO) build -o $(WORKER_BIN) ./cmd/worker

.PHONY: run-server-bin
run-server-bin: build-server
	PORT=$(PORT) \
	NAMESPACE=$(NAMESPACE) \
	DATABASE_URL=$(DATABASE_URL) \
	REDIS_ADDR=$(REDIS_ADDR) \
	REDIS_PASSWORD=$(REDIS_PASSWORD) \
	$(SERVER_BIN)

.PHONY: run-worker-bin
run-worker-bin: build-worker
	DATABASE_URL=$(DATABASE_URL) \
	REDIS_ADDR=$(REDIS_ADDR) \
	REDIS_PASSWORD=$(REDIS_PASSWORD) \
	WORKER_CONCURRENCY=$(WORKER_CONCURRENCY) \
	$(WORKER_BIN)

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: test
test:
	$(GO) test ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: clean
clean:
	rm -rf $(BINARY_DIR)

.PHONY: api-provision
api-provision:
	curl -X POST http://localhost:$(PORT)/api/provision \
		-H "Content-Type: application/json" \
		-d '{"stackName": "tark-test-01", "region": "southindia"}'

.PHONY: api-deploy
api-deploy:
	curl -X POST http://localhost:$(PORT)/api/deploy \
		-H "Content-Type: application/json" \
		-d '{"name": "tinyllama"}'

.PHONY: api-chat
api-chat:
	curl -X POST http://localhost:$(PORT)/v1/chat/completions \
		-H "Content-Type: application/json" \
		-d '{"model": "tinyllama", "messages": [{"role": "user", "content": "Explain Kubernetes in one simple sentence."}]}'

.PHONY: api-destroy
api-destroy:
	curl -X POST http://localhost:$(PORT)/api/destroy \
		-H "Content-Type: application/json" \
		-d '{"stackName": "tark-test-01"}'

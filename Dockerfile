# Stage 1: Build binaries
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install dependencies and required tools
RUN apk add --no-cache git make

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build both binaries statically
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/worker ./cmd/worker

# Stage 2: Server Image
FROM alpine:3.19 AS server

WORKDIR /app

# Install CA certificates for external API calls
RUN apk add --no-cache ca-certificates

# Copy server binary from builder
COPY --from=builder /app/bin/server /app/server

EXPOSE 8080
CMD ["/app/server"]

# Stage 3: Worker Image
FROM alpine:3.19 AS worker

WORKDIR /app

# Install required tools (e.g. for ssh execution, pulumi if used as a binary locally)
# Pulumi might need manual fetching if you don't use their base image or binary downloads in code.
# The worker needs to run ssh to connect and deploy.
RUN apk add --no-cache ca-certificates openssh-client curl bash

# Install Pulumi CLI
RUN curl -fsSL https://get.pulumi.com | sh
ENV PATH="/root/.pulumi/bin:${PATH}"

# Copy worker binary
COPY --from=builder /app/bin/worker /app/worker
# Optionally, if Pulumi templates / Azure infra files are needed during execution, copy them:
COPY --from=builder /app/infra /app/infra

CMD ["/app/worker"]

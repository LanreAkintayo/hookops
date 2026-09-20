# Multi-stage Dockerfile for HookOps

# Stage 1: Build stage with full Go toolchain
FROM golang:alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /app

# Copy all source code (including vendor if present)
COPY . .

# Compile optimized static binary (uses vendor if present, otherwise downloads modules)
RUN if [ -d "vendor" ]; then \
        CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-w -s" -o /app/bin/api cmd/api/main.go; \
    else \
        CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/api cmd/api/main.go; \
    fi

# Stage 2: Minimal runtime image
FROM alpine:3.20

WORKDIR /app

# Install root CA certificates (required for outbound HTTPS webhook calls) and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Run as non-root user for security
RUN addgroup -S hookops && adduser -S hookops -G hookops

COPY --from=builder /app/bin/api /app/api

USER hookops

EXPOSE 8080

ENTRYPOINT ["/app/api"]

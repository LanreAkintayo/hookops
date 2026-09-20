.PHONY: db-up db-down migrate-up migrate-down build run test lint swagger test-integration test-coverage

db-up:
	docker compose up -d

db-down:
	docker compose down

db-clean:
	docker exec -i hookops-db psql -U postgres -d hookops -c "TRUNCATE TABLE delivery_attempts, events, subscriptions, event_types, endpoints, applications CASCADE;"


migrate-up:
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/001_create_applications_table.up.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/002_create_endpoints_table.up.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/003_create_event_types_table.up.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/004_create_subscriptions_table.up.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/005_create_events_table.up.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/006_create_delivery_attempts_table.up.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/007_add_rate_limit_to_endpoints.up.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/008_add_health_tracking_to_endpoints.up.sql

migrate-down:
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/008_add_health_tracking_to_endpoints.down.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/007_add_rate_limit_to_endpoints.down.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/006_create_delivery_attempts_table.down.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/005_create_events_table.down.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/004_create_subscriptions_table.down.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/003_create_event_types_table.down.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/002_create_endpoints_table.down.sql
	docker exec -i hookops-db psql -U postgres -d hookops < migrations/001_create_applications_table.down.sql

build:
	go build -o bin/api cmd/api/main.go

run:
	go run cmd/api/main.go

test:
	go test -v ./...

lint:
	@if [ -d /snap/go/11262 ]; then \
		GOROOT=/snap/go/11262 PATH=/snap/go/11262/bin:$$PATH golangci-lint run ./...; \
	else \
		golangci-lint run ./...; \
	fi

swagger:
	swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal

test-integration:
	go test -tags=integration -v -count=1 ./tests/integration/...

test-coverage:
	go test -coverpkg=./internal/config,./internal/engine,./internal/handler,./internal/middleware,./internal/response,./internal/router,./internal/service -coverprofile=coverage.out ./internal/config ./internal/engine ./internal/handler ./internal/middleware ./internal/response ./internal/router ./internal/service
	go tool cover -func=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage HTML report generated at coverage.html"


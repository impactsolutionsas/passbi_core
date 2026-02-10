.PHONY: help build run test clean docker migrate import

# Default target
help:
	@echo "PassBi Core - Makefile Commands"
	@echo "================================"
	@echo "build        - Build all binaries"
	@echo "run          - Run API server"
	@echo "test         - Run all tests"
	@echo "clean        - Remove build artifacts"
	@echo "docker       - Build and run with Docker Compose"
	@echo "migrate-up   - Run database migrations"
	@echo "migrate-down - Rollback database migrations"
	@echo "import       - Import GTFS data (requires GTFS= and AGENCY= vars)"

# Build targets
build:
	@echo "Building binaries..."
	@mkdir -p bin
	go build -o bin/passbi-api cmd/api/main.go
	go build -o bin/passbi-import cmd/importer/main.go
	@echo "✓ Build complete"

# Run API server
run:
	@echo "Starting API server..."
	go run cmd/api/main.go

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin
	@echo "✓ Clean complete"

# Docker targets
docker:
	@echo "Starting with Docker Compose..."
	docker-compose up --build

docker-down:
	@echo "Stopping Docker Compose..."
	docker-compose down

# Database migrations
migrate-up:
	@echo "Running migrations..."
	migrate -path migrations -database "postgres://passbi_user:passbi_password@localhost:5432/passbi?sslmode=disable" up
	@echo "✓ Migrations complete"

migrate-down:
	@echo "Rolling back migrations..."
	migrate -path migrations -database "postgres://passbi_user:passbi_password@localhost:5432/passbi?sslmode=disable" down
	@echo "✓ Rollback complete"

# Import GTFS
import:
	@if [ -z "$(GTFS)" ] || [ -z "$(AGENCY)" ]; then \
		echo "Error: GTFS and AGENCY variables required"; \
		echo "Usage: make import GTFS=./gtfs.zip AGENCY=agency_id"; \
		exit 1; \
	fi
	@echo "Importing GTFS for agency $(AGENCY)..."
	go run cmd/importer/main.go --agency-id=$(AGENCY) --gtfs=$(GTFS) --rebuild-graph
	@echo "✓ Import complete"

# Development helpers
dev-setup:
	@echo "Setting up development environment..."
	cp .env.example .env
	go mod download
	@echo "✓ Development setup complete"

fmt:
	@echo "Formatting code..."
	go fmt ./...
	@echo "✓ Format complete"

lint:
	@echo "Running linter..."
	golangci-lint run
	@echo "✓ Lint complete"

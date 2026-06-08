.PHONY: build run test t clean mock migrate-up migrate-down migrate-create

# Variables
BINARY_NAME=bookerbot
INSTANCE ?= local
MIGRATION_DIR=db/migrations
GO=$(shell which go)


# Build the application
build:
	go build -o bin/$(BINARY_NAME) cmd/bookerbotapi/main.go

# Run the application
run:
	INSTANCE=$(INSTANCE) go run cmd/bookerbotapi/main.go

# Test the application
test: t
	$(GO) test -v $(shell go list ./... | grep -v /vendor/)

t:
	$(GO) test -v ./... -count=2 -race -coverprofile=cover.out.tmp $(RUN_ARGS)

# Clean build artifacts
clean:
	go clean
	rm -rf bin/

# Generate mocks
mock:
	mockgen -source=./internal/repository/booking_repository.go > ./internal/repository/mock/mock_booking_repository.go
	mockgen -source=./internal/availability/manager.go > ./internal/availability/mocks/manager.go

# Create new migration
migrate-create:
	@read -p "Enter migration name: " name; \
	goose -dir $(MIGRATION_DIR) create $$name sql

# Run migrations up
migrate-up:
	goose -dir $(MIGRATION_DIR) postgres "$$DATABASE_URL" up

# Run migrations down
migrate-down:
	goose -dir $(MIGRATION_DIR) postgres "$$DATABASE_URL" down

# Docker build
docker-build:
	docker build -t tgbot:latest .

# Docker run
docker-run:
	docker run -e INSTANCE=docker -e DATABASE_URL="$$DATABASE_URL" tgbot:latest

# Install dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Run with hot reload (requires air)
dev:
	air


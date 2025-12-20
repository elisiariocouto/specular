.PHONY: help build run test lint clean fmt

BINARY_NAME=specular
GO=go
GOFLAGS=-v
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_LDFLAGS := -s -w -X github.com/elisiariocouto/specular/internal/version.Version=$(VERSION) -X github.com/elisiariocouto/specular/internal/version.Commit=$(COMMIT) -X github.com/elisiariocouto/specular/internal/version.BuildDate=$(BUILD_DATE)

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build the application
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) -ldflags "$(GO_LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/specular

run: build ## Build and run the application
	./bin/$(BINARY_NAME)

test: ## Run tests
	@echo "Running tests..."
	$(GO) test $(GOFLAGS) ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

lint: ## Run linters
	@echo "Running linters..."
	$(GO) fmt ./...
	staticcheck ./...
	$(GO) vet ./...
	@echo "Lint check passed"

fmt: ## Format code
	@echo "Formatting code..."
	$(GO) fmt ./...
	goimports -w .

clean: ## Remove build artifacts
	@echo "Cleaning..."
	$(GO) clean
	rm -f bin/$(BINARY_NAME)
	rm -f coverage.out coverage.html

deps: ## Download and tidy dependencies
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

docker-build: ## Build Docker image
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(BINARY_NAME):latest .

docker-build-alpine: ## Build Alpine Docker image
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-f Dockerfile.alpine \
		-t $(BINARY_NAME):latest-alpine .

docker-run: docker-build ## Build and run Docker container
	docker run -p 8080:8080 -v /tmp/specular-cache:/var/cache/specular $(BINARY_NAME):latest

docker-run-alpine: docker-build-alpine ## Build and run Alpine Docker container
	docker run -p 8080:8080 -v /tmp/specular-cache:/var/cache/specular $(BINARY_NAME):latest-alpine

.DEFAULT_GOAL := help

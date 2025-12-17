.PHONY: help build run test lint clean fmt

BINARY_NAME=speculum
GO=go
GOFLAGS=-v

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build the application
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) -o bin/$(BINARY_NAME) ./cmd/speculum

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
	docker build -t $(BINARY_NAME):latest .

docker-run: docker-build ## Build and run Docker container
	docker run -p 8080:8080 -v /tmp/speculum-cache:/var/cache/speculum $(BINARY_NAME):latest

.DEFAULT_GOAL := help

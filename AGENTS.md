# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Speculum is a caching proxy mirror for Terraform providers that implements the [Terraform Provider Network Mirror Protocol](https://developer.hashicorp.com/terraform/internals/provider-network-mirror-protocol). It intercepts provider requests, caches them locally, and serves subsequent requests from cache.

## Setup for Development

### Prerequisites
- Go 1.22 or later
- pre-commit installed (`pip install pre-commit` or `brew install pre-commit`)

### Initial Setup
1. Clone the repository
2. Run `make deps` to download dependencies
3. Run `pre-commit install` to set up pre-commit hooks
4. Configure environment variables (see "Running Locally" section below)

### Pre-commit Hooks
Pre-commit hooks will automatically run before each commit to:
- Format code with `go fmt` and `goimports`
- Run linters with `go vet` and `staticcheck`
- Run tests to ensure nothing is broken

If pre-commit fails, the commit is canceled. Fix the issues and try again.

To run pre-commit manually on all files:
```bash
pre-commit run --all-files
```

To skip pre-commit hooks (not recommended):
```bash
git commit --no-verify
```

## Commands

### Building and Running
- `make build` - Build the binary to `bin/speculum`
- `make run` - Build and run the application
- `make clean` - Remove build artifacts

### Testing
- `make test` - Run all tests
- `make test-coverage` - Run tests with coverage report (generates `coverage.html`)
- To run tests for a specific package: `go test -v ./internal/mirror`

### Code Quality
- `make fmt` - Format code with `go fmt` and `goimports`
- `make lint` - Run linters (`go vet`)
- `make deps` - Download and tidy dependencies

### Docker
- `make docker-build` - Build Docker image
- `make docker-run` - Build and run in Docker container

### Running Locally
Required environment variables for local development:
```bash
export SPECULUM_PORT=8080
export SPECULUM_HOST=0.0.0.0
export SPECULUM_CACHE_DIR=/tmp/speculum-cache
export SPECULUM_BASE_URL=http://localhost:8080
```

## Architecture

### Component Layers

The application follows a clean layered architecture:

1. **cmd/speculum/main.go** - Application entry point that wires up all components
   - Loads configuration from environment variables
   - Initializes storage backend (filesystem or memory)
   - Creates upstream client with retry logic
   - Assembles mirror service with storage and upstream
   - Starts HTTP server with graceful shutdown

2. **internal/server** - HTTP server and routing layer
   - Uses chi router with middleware chain (RequestID → Recovery → Logging → Metrics)
   - Routes are organized under `/terraform/providers` base path for future multi-registry support
   - Single unified route: `/:hostname/:namespace/:type/*` handles index.json, version.json, and .zip files
   - MetadataHandler dispatches to appropriate handlers based on file extension
   - Also serves `/health` and `/metrics` endpoints

3. **internal/mirror** - Core cache-or-fetch business logic
   - `GetIndex()` - Returns provider version list (index.json), cache-first
   - `GetVersion()` - Returns provider packages for specific version, rewrites URLs to point to mirror, stores upstream URL mappings
   - `GetArchive()` - Serves provider archive files, fetches from upstream using stored URL mapping if not cached, computes h1: hash on first fetch
   - URL rewriting: Converts upstream URLs to local mirror paths following terraform providers mirror structure (hostname/namespace/type/filename.zip)
   - Upstream URL mapping: Stored when version metadata is fetched to enable lazy archive fetching
   - h1: hash computation: extracts zip to temp directory and uses dirhash.HashDir (matches Terraform's approach, avoids HashZip bug with directory entries)

4. **internal/storage** - Storage abstraction layer
   - Interface with two implementations: `FilesystemStorage` and `MemoryStorage`
   - Methods: GetIndex, PutIndex, GetVersion, PutVersion, GetVersionsResponse, PutVersionsResponse, GetArchive, PutArchive, ExistsArchive
   - Filesystem layout matches `terraform providers mirror` structure:
     - Index files: `hostname/namespace/type/index.json`
     - Version metadata: `hostname/namespace/type/VERSION.json`
     - Versions API response cache: `.speculum-internal/hostname/namespace/type/versions.json`
     - Archives: stored with full domain preservation (e.g., `registry.terraform.io/hashicorp/aws/terraform-provider-aws_6.26.0_darwin_arm64.zip`)

5. **internal/mirror/upstream.go** - Upstream registry client
   - Handles fetching from registry.terraform.io or other registries
   - Built-in retry logic with exponential backoff
   - Timeout configuration per request

6. **internal/config** - Configuration management
   - All configuration from environment variables with `SPECULUM_` prefix
   - Validation in `Validate()` method
   - See internal/config/config.go:10-35 for complete Config struct

7. **internal/metrics** - Prometheus metrics collection
8. **internal/logger** - Structured logging with slog

### Key Design Patterns

- **Cache-first strategy**: All Get* methods in mirror service check storage before upstream
- **Non-blocking cache writes**: Cache PutIndex/PutVersion errors are ignored to not block requests
- **URL rewriting**: Archive URLs from upstream are rewritten to point to this mirror's `/archive-downloads/` endpoint
- **Archive path preservation**: Full domain preserved in paths (e.g., `releases.hashicorp.com/path/to/file.zip`) to support multiple upstream sources
- **h1: hash on-demand**: Computed when archive is first cached, stored separately for reuse

### Protocol Implementation

Implements Terraform Provider Network Mirror Protocol v1:
- `GET /:hostname/:namespace/:type/index.json` - List available versions
- `GET /:hostname/:namespace/:type/:version.json` - List packages for version
- Archive downloads are served from rewritten URLs pointing to this mirror

### Dependencies

- **chi/v5** - HTTP router with middleware support
- **prometheus/client_golang** - Metrics collection
- **golang.org/x/mod/sumdb/dirhash** - h1 hash computation for provider archives
- Go 1.25.5 standard library

## Configuration Notes

- All configuration is environment-based (no config files)
- `SPECULUM_BASE_URL` must match the public URL where the mirror is accessible
- Storage type can be switched between "filesystem" and "memory" via `SPECULUM_STORAGE_TYPE`
- Upstream registry is configurable for testing or alternate registries
- Filesystem structure matches `terraform providers mirror` for compatibility with existing tooling

## Code Quality Standards

- **Always fix lint errors**: Never use `nolint` directives or ignore linting rules
- If a lint rule flags code as problematic, the code should be refactored to resolve the issue properly
- The linting setup includes `go fmt`, `staticcheck`, and `go vet` to maintain code quality
- All code must pass `make lint` before being considered complete

## Testing Approach

- Unit tests should mock the Storage interface for mirror service tests
- Integration tests should use MemoryStorage for speed
- Use httptest for server/handler testing

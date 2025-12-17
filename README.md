# 🪞 Speculum

Speculum is an open-source Terraform network mirror that provides caching, control, and reproducibility for infrastructure dependencies. This might evolve into a generic proxy mirror for other packages/artifacts.

Speculum implements the [Terraform Provider Network Mirror Protocol](https://developer.hashicorp.com/terraform/internals/provider-network-mirror-protocol) to intercept provider requests, cache them locally, and serve subsequent requests from cache. This reduces dependency on upstream registries and improves deployment speeds.

## Features

- **Caching Proxy**: Cache Terraform providers locally to reduce upstream traffic
- **Simple Configuration**: Environment variable-based configuration
- **Observability**: Prometheus metrics and structured logging
- **Extensible Storage**: Filesystem storage with interface for future S3 support

## Requirements

- Go 1.21 or later

## Quick Start

### Installation

```bash
git clone https://github.com/elisiariocouto/speculum.git
cd speculum
make build
```

### Running Locally

```bash
# Set up cache directory
mkdir -p /tmp/speculum-cache

# Configure environment variables
export SPECULUM_PORT=8080
export SPECULUM_HOST=0.0.0.0
export SPECULUM_CACHE_DIR=/tmp/speculum-cache
export SPECULUM_BASE_URL=http://localhost:8080

# Run the server
make run
```

### Using with Terraform

Configure Terraform to use the mirror by adding to `~/.terraformrc`:

```hcl
provider_installation {
  network_mirror {
    url = "http://localhost:8080/terraform/providers/"
  }
}
```

Then run `terraform init` in any Terraform project and it will use your local mirror.

## Configuration

All configuration is via environment variables:

### Server Configuration
- `SPECULUM_PORT` (default: `8080`) - HTTP server port
- `SPECULUM_HOST` (default: `0.0.0.0`) - Bind address
- `SPECULUM_READ_TIMEOUT` (default: `30s`) - HTTP read timeout
- `SPECULUM_WRITE_TIMEOUT` (default: `30s`) - HTTP write timeout
- `SPECULUM_SHUTDOWN_TIMEOUT` (default: `30s`) - Graceful shutdown timeout

### Storage Configuration
- `SPECULUM_STORAGE_TYPE` (default: `filesystem`) - Storage backend
- `SPECULUM_CACHE_DIR` (default: `/var/cache/speculum`) - Cache directory

### Upstream Configuration
- `SPECULUM_UPSTREAM_TIMEOUT` (default: `60s`) - Upstream request timeout
- `SPECULUM_UPSTREAM_MAX_RETRIES` (default: `3`) - Max retry attempts

### Mirror Configuration
- `SPECULUM_BASE_URL` (default: `http://localhost:8080`) - Public base URL of mirror

### Observability Configuration
- `SPECULUM_LOG_LEVEL` (default: `info`) - Log level: debug, info, warn, error
- `SPECULUM_LOG_FORMAT` (default: `json`) - Log format: json, text
- `SPECULUM_METRICS_ENABLED` (default: `true`) - Enable Prometheus metrics

## API Endpoints

### List Versions
```
GET /terraform/providers/:hostname/:namespace/:type/index.json
```

Returns available versions of a provider.

### List Packages
```
GET /terraform/providers/:hostname/:namespace/:type/:version.json
```

Returns available installation packages for a specific version.

### Metrics
```
GET /metrics
```

Prometheus metrics endpoint.

### Health
```
GET /health
```

Health check endpoint.

## Development

### Running Tests

```bash
make test
```

### Running Tests with Coverage

```bash
make test-coverage
```

### Formatting Code

```bash
make fmt
```

### Linting

```bash
make lint
```

## Architecture

The mirror consists of several layers:

- **HTTP Server** - Handles requests and routing
- **Mirror Service** - Core cache-or-fetch business logic
- **Storage Layer** - Abstract interface with filesystem implementation
- **Upstream Client** - Fetches from registry.terraform.io
- **Observability** - Prometheus metrics and structured logging

## Future Enhancements

- S3 storage backend
- Cache invalidation API
- Pre-warming cache
- Authentication and authorization
- Rate limiting
- Support for other ecosystems (Docker, npm, PyPI)

# 🪞 Speculum

Speculum is an open-source Terraform provider network mirror. This might evolve into a generic proxy mirror for other packages/artifacts.

Speculum implements the [Terraform Provider Network Mirror Protocol](https://developer.hashicorp.com/terraform/internals/provider-network-mirror-protocol) to intercept provider requests, cache them locally, and serve subsequent requests from cache. This reduces dependency on upstream registries and improves deployment speeds.

## Features

- **Caching Proxy**: Cache Terraform providers locally to reduce upstream traffic
- **Simple Configuration**: Environment variable-based configuration
- **Observability**: Prometheus metrics and structured logging
- **Extensible Storage**: Filesystem storage with interface for future S3 support

## Quick Start

> **⚠️ Important**: Terraform requires network mirrors to be served over HTTPS with a valid certificate. Running Speculum on plain HTTP (localhost development excluded) will not work with Terraform. Use a reverse proxy like [Caddy](https://caddyserver.com/), [Traefik](https://traefik.io/), or [nginx](https://nginx.org/) to handle TLS termination.

### Installation

Download the latest release from the [releases page](https://github.com/elisiariocouto/speculum/releases).

Alternatively, run Speculum using Docker:

```bash
# Docker Hub
docker run -p 8080:8080 \
  -e SPECULUM_BASE_URL=https://speculum.example.com/ \
  elisiariocouto/speculum:latest

# GitHub Container Registry
docker run -p 8080:8080 \
  -e SPECULUM_BASE_URL=https://speculum.example.com/ \
  ghcr.io/elisiariocouto/speculum:latest
```

### Using with Terraform

Configure Terraform to use the mirror by adding to `~/.terraformrc`:

```hcl
provider_installation {
  network_mirror {
    url = "https://speculum.example.com/"
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
- `SPECULUM_BASE_URL` (default: `https://speculum.example.com`) - Public base URL of mirror, supports paths

### Observability Configuration
- `SPECULUM_LOG_LEVEL` (default: `info`) - Log level: debug, info, warn, error
- `SPECULUM_LOG_FORMAT` (default: `json`) - Log format: json, text
- `SPECULUM_METRICS_ENABLED` (default: `true`) - Enable Prometheus metrics

## API Endpoints

### List Versions
```
GET $SPECULUM_BASE_URL/:hostname/:namespace/:type/index.json
```

Returns available versions of a provider.

### List Packages
```
GET $SPECULUM_BASE_URL/:hostname/:namespace/:type/:version.json
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

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, running locally, and release procedures.

## Architecture

The mirror consists of several layers:

- **HTTP Server** - Handles requests and routing
- **Mirror Service** - Core cache-or-fetch business logic
- **Storage Layer** - Abstract interface with filesystem implementation
- **Upstream Client** - Fetches from provider registries, uses Terraform's [Remote Service Discovery Protocol](https://developer.hashicorp.com/terraform/internals/remote-service-discovery)
- **Observability** - Prometheus metrics and structured logging

## Future Enhancements

- S3 storage backend
- Cache invalidation API
- Pre-warming cache (already supported since the filesystem structure is the same as `terraform providers mirror`)
- Authentication and authorization
- Rate limiting
- Support for other ecosystems (Docker, npm, PyPI, nuget, maven)

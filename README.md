# 🪞 Specular

Specular is an open-source Terraform provider network mirror. This might evolve into a generic proxy mirror for other packages/artifacts.

Specular implements the [Terraform Provider Network Mirror Protocol](https://developer.hashicorp.com/terraform/internals/provider-network-mirror-protocol) to intercept provider requests, cache them locally, and serve subsequent requests from cache. This reduces dependency on upstream registries and improves deployment speeds.

## Features

- **Caching Proxy**: Cache Terraform providers locally to reduce upstream traffic
- **Simple Configuration**: Environment variable-based configuration
- **Observability**: Prometheus metrics and structured logging
- **Extensible Storage**: Filesystem storage with interface for future S3 support

## Quick Start

> **⚠️ Important**: Terraform requires network mirrors to be served over HTTPS with a valid certificate. Running Specular on plain HTTP (localhost development excluded) will not work with Terraform. Use a reverse proxy like [Caddy](https://caddyserver.com/), [Traefik](https://traefik.io/), or [nginx](https://nginx.org/) to handle TLS termination.

### Installation

Download the latest release from the [releases page](https://github.com/elisiariocouto/specular/releases).

Alternatively, run Specular using Docker:

```bash
# Docker Hub
docker run -p 8080:8080 \
  -e SPECULAR_BASE_URL=https://specular.example.com \
  elisiariocouto/specular:latest

# GitHub Container Registry
docker run -p 8080:8080 \
  -e SPECULAR_BASE_URL=https://specular.example.com \
  ghcr.io/elisiariocouto/specular:latest
```

The default images are based on distroless. Alpine-based images are available with the `-alpine` suffix (e.g., `latest-alpine`, `2025.12.3-alpine`) for a lightweight footprint. These are suitable for resource-constrained environments.

### Using with Terraform

Configure Terraform to use the mirror by adding to `~/.terraformrc`:

```hcl
provider_installation {
  network_mirror {
    url = "https://specular.example.com/terraform/providers/"
  }
}
```

Then run `terraform init` in any Terraform project and it will use your local mirror.

> **Note**: The URL must end with `/terraform/providers/` to match Specular's routing structure.

## Configuration

All configuration is via environment variables:

### Server Configuration
- `SPECULAR_BASE_URL` (default: `https://specular.example.com`) - Public base URL of mirror (without trailing slash). Supports hosting at subpaths (e.g., `https://example.com/mirror`). Note that Terraform provider endpoints are served under `/terraform/providers`, and observability endpoints (`/health`, `/metrics`) are served at the root of this base URL.
- `SPECULAR_PORT` (default: `8080`) - HTTP server port
- `SPECULAR_HOST` (default: `0.0.0.0`) - Bind address
- `SPECULAR_READ_TIMEOUT` (default: `30s`) - HTTP read timeout
- `SPECULAR_WRITE_TIMEOUT` (default: `30s`) - HTTP write timeout
- `SPECULAR_SHUTDOWN_TIMEOUT` (default: `30s`) - Graceful shutdown timeout

### Storage Configuration
- `SPECULAR_STORAGE_TYPE` (default: `filesystem`) - Storage backend
- `SPECULAR_CACHE_DIR` (default: `/var/cache/specular`) - Cache directory

### Upstream Configuration
- `SPECULAR_UPSTREAM_TIMEOUT` (default: `60s`) - Upstream request timeout
- `SPECULAR_UPSTREAM_MAX_RETRIES` (default: `3`) - Max retry attempts

### Observability Configuration
- `SPECULAR_LOG_LEVEL` (default: `info`) - Log level: debug, info, warn, error
- `SPECULAR_LOG_FORMAT` (default: `json`) - Log format: json, text
- `SPECULAR_METRICS_ENABLED` (default: `true`) - Enable Prometheus metrics

## API Endpoints

> **Note**: All Terraform provider endpoints are served under the `/terraform/providers` path prefix. This structure allows Specular to support other package registries in the future (e.g., `/docker/registries`, `/npm`, `/pypi`, `/maven`) as a multi-ecosystem pull-through cache.

### Terraform Provider Endpoints

#### List Versions
```
GET $SPECULAR_BASE_URL/terraform/providers/:hostname/:namespace/:type/index.json
```

Returns available versions of a provider.

**Example:**
```
https://specular.example.com/terraform/providers/registry.terraform.io/hashicorp/aws/index.json
```

#### List Packages
```
GET $SPECULAR_BASE_URL/terraform/providers/:hostname/:namespace/:type/:version.json
```

Returns available installation packages for a specific version.

**Example:**
```
https://specular.example.com/terraform/providers/registry.terraform.io/hashicorp/aws/5.70.0.json
```

### Observability Endpoints

#### Health
```
GET $SPECULAR_BASE_URL/health
```

Health check endpoint. Returns `{"status":"ok"}` when the service is running.

#### Metrics
```
GET $SPECULAR_BASE_URL/metrics
```

Prometheus metrics endpoint (returns 404 if metrics are disabled via `SPECULAR_METRICS_ENABLED=false`).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, running locally, and release procedures.

## Architecture

The mirror consists of the following layers:

- **HTTP Server** - Handles requests and routing
- **Mirror Service** - Core cache-or-fetch business logic
- **Storage Layer** - Abstract interface with filesystem implementation
- **Upstream Client** - Fetches from provider registries, uses Terraform's [Remote Service Discovery Protocol](https://developer.hashicorp.com/terraform/internals/remote-service-discovery)
- **Observability** - Prometheus metrics and structured logging

## Future Enhancements

- S3 storage backend
- Cache invalidation API
- Pre-warming cache (*technically* already supported since the filesystem structure is the same as `terraform providers mirror`)
- Authentication and authorization
- Rate limiting
- Support for other ecosystems (Docker, npm, PyPI, nuget, maven)

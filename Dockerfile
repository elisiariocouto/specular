# Build stage
FROM golang:1.25 AS builder
WORKDIR /src

ENV CGO_ENABLED=0
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG COMMIT
ARG BUILD_DATE

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN set -e; \
    VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}; \
    COMMIT=${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}; \
    BUILD_DATE=${BUILD_DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}; \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w -X github.com/elisiariocouto/speculum/internal/version.Version=$VERSION -X github.com/elisiariocouto/speculum/internal/version.Commit=$COMMIT -X github.com/elisiariocouto/speculum/internal/version.BuildDate=$BUILD_DATE" -o /out/speculum ./cmd/speculum

# Runtime stage
FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /

ENV SPECULUM_PORT=8080 \
    SPECULUM_HOST=0.0.0.0 \
    SPECULUM_CACHE_DIR=/tmp/speculum-cache \
    SPECULUM_BASE_URL=http://localhost:8080 \
    SPECULUM_STORAGE_TYPE=filesystem

COPY --from=builder /out/speculum /speculum

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/speculum"]

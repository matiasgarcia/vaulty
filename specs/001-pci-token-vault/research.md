# Research: PCI-Compliant Token Vault & Payment Proxy

**Branch**: `001-pci-token-vault` | **Date**: 2026-03-28

## 1. Language & Runtime

- **Decision**: Go 1.22+
- **Rationale**: Go's stdlib provides audited cryptographic primitives
  (crypto/aes, crypto/hmac, crypto/rand) essential for PCI-compliant
  encryption. Goroutines handle 1,000+ concurrent requests efficiently
  with minimal memory overhead. Static binaries enable scratch/distroless
  Docker images with minimal attack surface.
- **Alternatives considered**:
  - Python 3.12 + FastAPI: Good payment provider SDKs but GIL limits
    CPU-bound crypto operations; higher memory and slower cold starts.
  - Node.js 22 + Fastify: Single-threaded limits crypto throughput;
    less predictable memory management under load.
  - Rust + Axum: Maximum performance but 2-3x slower development;
    limited payment provider SDK ecosystem.

## 2. HTTP Framework

- **Decision**: chi v5 (lightweight HTTP router)
- **Rationale**: chi is a composable, idiomatic Go HTTP router built on
  net/http stdlib. Supports middleware chaining (auth, logging, recovery),
  graceful shutdown, and request context propagation. Minimal dependency
  footprint aligns with PCI attack surface requirements.
- **Alternatives considered**:
  - Gin: More opinionated, larger dependency tree, custom context
    instead of stdlib context.
  - Echo: Similar to Gin — heavier than needed for this use case.
  - stdlib net/http only: Viable but chi adds routing ergonomics
    without meaningful overhead.

## 3. Primary Storage (Vault/PAN)

- **Decision**: PostgreSQL 16+
- **Rationale**: ACID transactions ensure data integrity for tokenization
  operations. Row-level security (RLS) can enforce per-service access
  policies at the database level. Streaming replication supports the
  99.999% uptime target with automatic failover (Patroni or cloud-managed).
- **Alternatives considered**:
  - MySQL 8: Lacks row-level security; less mature JSON support for
    metadata; weaker extension ecosystem.
  - DynamoDB: AWS lock-in; eventually consistent by default; harder
    to enforce ACID across token + vault entry operations.

## 4. Ephemeral Storage (CVV)

- **Decision**: Redis 7+
- **Rationale**: Native EXPIRE/TTL with automatic key eviction is exactly
  what CVV ephemeral storage requires. GETDEL command provides atomic
  get-and-delete for single-use semantics. Redis Sentinel or Cluster mode
  supports high availability. Encryption at rest available via Redis
  Enterprise or cloud-managed Redis (ElastiCache, Cloud Memorystore).
- **Alternatives considered**:
  - Memcached: No atomic get-and-delete; no encryption at rest; no
    replication for HA — fails PCI requirements.
  - In-memory (process): Data lost on restart; no HA; only viable
    for single-instance which contradicts 99.999% uptime.

## 5. Testing Framework

- **Decision**: go test (stdlib) + testify + testcontainers-go
- **Rationale**: go test is the standard testing tool with built-in
  benchmarking and race detection. testify adds assertion helpers and
  mock generation. testcontainers-go enables integration tests against
  real PostgreSQL and Redis instances in Docker, avoiding mock/prod
  divergence for encryption and TTL behaviors.
- **Alternatives considered**:
  - go test only: Viable but verbose assertions slow development.
  - Ginkgo/Gomega: BDD-style adds complexity without clear benefit
    for this domain.

## 6. Containerization

- **Decision**: Multi-stage Docker builds with distroless base images
- **Rationale**: Go produces static binaries that run on
  `gcr.io/distroless/static-debian12` — no shell, no package manager,
  minimal CVE surface. This directly satisfies PCI DSS hardening
  requirements (no unnecessary software). Multi-stage builds keep
  build tools out of the final image.
- **Alternatives considered**:
  - Alpine: Includes shell and apk — larger attack surface.
  - Scratch: No CA certificates bundle; distroless includes it.

## 7. Payment Provider Integration

- **Decision**: Direct HTTP client with provider REST APIs
- **Rationale**: Go lacks official Stripe/Adyen SDKs with the same
  maturity as Python/Node equivalents. However, both Stripe and Adyen
  expose well-documented REST APIs. Using Go's net/http client with
  a thin adapter layer per provider keeps dependencies minimal and
  allows adding new providers without SDK availability constraints.
  Stripe does have a community Go SDK (stripe-go) that can be used.
- **Alternatives considered**:
  - stripe-go community SDK: Viable for Stripe specifically; use it
    if Stripe is the primary provider. But maintain adapter pattern
    for provider-agnostic design.

## 8. Key Management

- **Decision**: External KMS (AWS KMS / GCP Cloud KMS / Azure Key Vault)
- **Rationale**: Envelope encryption requires a KEK managed by a
  hardware-backed KMS. All major clouds offer FIPS 140-2 Level 3
  validated HSMs behind their KMS APIs. Go SDKs for AWS/GCP/Azure
  are mature and well-maintained.
- **Alternatives considered**:
  - HashiCorp Vault: Additional operational complexity; cloud KMS
    provides equivalent security with less overhead for this use case.
  - Self-managed HSM: Prohibitive cost and operational burden for
    initial deployment.

## 9. Observability

- **Decision**: Structured JSON logging (slog) + OpenTelemetry
- **Rationale**: Go 1.22+ includes slog in stdlib for structured
  logging — JSON output is SIEM-compatible. OpenTelemetry provides
  distributed tracing with correlation IDs across microservices,
  satisfying FR-011/FR-012 audit requirements.
- **Alternatives considered**:
  - Zap/Zerolog: High performance but slog is now stdlib and
    sufficient; fewer dependencies.
  - Jaeger directly: OpenTelemetry is the vendor-neutral standard
    that can export to Jaeger, Datadog, etc.

## 10. Database Driver & Migrations

- **Decision**: pgx v5 + golang-migrate
- **Rationale**: pgx is the most performant pure-Go PostgreSQL driver
  with native support for COPY, LISTEN/NOTIFY, and connection pooling
  (pgxpool). golang-migrate provides version-controlled schema
  migrations with CLI and library interfaces.
- **Alternatives considered**:
  - database/sql + lib/pq: lib/pq is in maintenance mode; pgx is
    the actively maintained successor.
  - GORM: ORM adds abstraction that obscures encryption operations
    and complicates row-level security integration.

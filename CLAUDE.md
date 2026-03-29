# vault Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-03-29

## Active Technologies
- Go 1.22+ (existing codebase) + chi v5, pgx v5, go-redis v9, AWS SDK v2 (KMS) — no new dependencies (002-multitenant-tokenization)
- PostgreSQL 16+ (add tenant_id to existing tables, new tenants table), Redis 7+ (CVV keys prefixed with tenant) (002-multitenant-tokenization)
- PostgreSQL 16+ (existing tokens/vault_entries tables unchanged), Redis 7+ (new CVV token keys alongside existing CVV keys) (003-independent-cvv-tokens)

- Go 1.22+ + chi v5 (HTTP router), pgx v5 (PostgreSQL driver), go-redis v9, AWS/GCP KMS SDK, slog (stdlib logging), OpenTelemetry (001-pci-token-vault)

## Project Structure

```text
src/
tests/
```

## Commands

# Add commands for Go 1.22+

## Code Style

Go 1.22+: Follow standard conventions

## Recent Changes
- 003-independent-cvv-tokens: Added Go 1.22+ (existing codebase) + chi v5, pgx v5, go-redis v9, AWS SDK v2 (KMS) — no new dependencies
- 002-multitenant-tokenization: Added Go 1.22+ (existing codebase) + chi v5, pgx v5, go-redis v9, AWS SDK v2 (KMS) — no new dependencies

- 001-pci-token-vault: Added Go 1.22+ + chi v5 (HTTP router), pgx v5 (PostgreSQL driver), go-redis v9, AWS/GCP KMS SDK, slog (stdlib logging), OpenTelemetry

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->

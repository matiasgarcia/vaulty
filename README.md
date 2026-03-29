# PCI Token Vault & Reveal-Forward Proxy

PCI DSS-compliant tokenization vault and stateless reveal-forward proxy. Securely tokenizes payment card data (PAN + CVV), stores PAN encrypted with envelope encryption (AES-256-GCM + KMS), holds CVV ephemerally in Redis (TTL, single-use), and exposes a proxy that reveals tokens and forwards real card data to any 3rd party provider.

## Architecture

```
Backend Client
     │
     ├──POST /vault/tokenize──▶ Tokenizer Service
     │                              ├── PostgreSQL (encrypted PAN)
     │                              ├── Redis (ephemeral CVV)
     │                              └── KMS (envelope encryption)
     │
     └──POST /proxy/forward───▶ Proxy Service (stateless, no DB)
                                    │
                                    ├──mTLS──▶ Tokenizer /internal/detokenize
                                    │          (reveal PAN + CVV)
                                    │
                                    └──HTTPS──▶ 3rd Party Provider
                                               (receives real card data)
```

- **Tokenizer**: Owns PostgreSQL and Redis. Tokenizes, detokenizes, manages lifecycle.
- **Proxy**: Stateless pipe. Scans JSON payloads for `tok_...` patterns, reveals them via Tokenizer, forwards to any destination. Stores nothing.

## Prerequisites

- Go 1.22+
- Docker & Docker Compose

## Quick Start

### 1. Start infrastructure

```bash
docker compose up -d
```

This starts:
- PostgreSQL 16 (port 5432)
- Redis 7 (port 6379)
- LocalStack with KMS (port 4566)
- Mock Provider / Express (port 9090)

### 2. Create KMS key

```bash
aws --endpoint-url=http://localhost:4566 --region us-east-1 \
  kms create-key --query 'KeyMetadata.Arn' --output text
```

Copy the ARN output into `.env` as `KMS_KEY_ARN`.

### 3. Configure environment

```bash
cp .env.example .env
```

Edit `.env` and set:
- `KMS_KEY_ARN` — the ARN from step 2
- `HMAC_KEY` — generate with `openssl rand -base64 32`

The rest of the defaults work for local development.

### 4. Run database migrations

```bash
set -a && source .env && set +a
go run cmd/migrate/main.go up
```

### 5. Start services

```bash
# Terminal 1 — Tokenizer (port 8080)
set -a && source .env && set +a && go run cmd/tokenizer/main.go

# Terminal 2 — Proxy (port 8081)
set -a && source .env && set +a && go run cmd/proxy/main.go
```

> **Note**: Use `set -a` before `source .env` to export all variables to the shell. Plain `source .env` does not export them.

### 6. Run smoke tests

```bash
set -a && source .env && set +a && ./test/smoke.sh
```

### 7. Check mock provider received real card data

```bash
docker compose logs mock-provider
```

## API Endpoints

### Tokenizer Service (port 8080)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/vault/tokenize` | Tokenize PAN + CVV, return token |
| GET | `/vault/tokens/{token_id}` | Get token status |
| DELETE | `/vault/tokens/{token_id}` | Deactivate token (soft-delete) |
| GET | `/vault/tokens/{token_id}/audit` | Get audit trail |
| POST | `/internal/detokenize` | Reveal PAN + CVV (mTLS, Proxy only) |
| GET | `/health` | Health check (no auth) |

### Proxy Service (port 8081)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/proxy/forward` | Reveal tokens in payload, forward to destination |
| GET | `/health` | Health check (no auth) |

## Usage Examples

### Create a tenant (required first)

```bash
curl -X POST http://localhost:8080/admin/tenants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin:secret" \
  -d '{"tenant_id": "merchant-a", "name": "Merchant A"}'
```

### Tokenize a card

```bash
curl -X POST http://localhost:8080/vault/tokenize \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-a" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "123"
  }'
```

### Forward with token reveal

```bash
curl -X POST http://localhost:8081/proxy/forward \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer client:secret" \
  -H "X-Tenant-ID: merchant-a" \
  -d '{
    "destination": "http://localhost:9090/receive",
    "payload": {
      "card": "tok_XXXXXXXXXX",
      "amount": 5000,
      "currency": "USD"
    }
  }'
```

The proxy scans `payload` for any `tok_...` values, reveals them (replaces with real PAN/expiry/CVV), and forwards the entire payload to the destination.

## Multitenancy

The system is fully multitenant. Every data operation is scoped by a mandatory `X-Tenant-ID` header.

- **Same PAN + different tenant = different token** (tenant-scoped blind index)
- **Same PAN + same tenant = same token** (deterministic within tenant)
- **Per-tenant KMS key** — each tenant gets a dedicated encryption key provisioned at creation
- **Zero cross-tenant data leakage** — all queries filtered by tenant

### Tenant provisioning

```bash
curl -X POST http://localhost:8080/admin/tenants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin:secret" \
  -d '{"tenant_id": "merchant-a", "name": "Merchant A"}'
```

All subsequent requests must include `X-Tenant-ID: merchant-a`.

### Tenant management

| Method | Path | Description |
|--------|------|-------------|
| POST | `/admin/tenants` | Create tenant (provisions KMS key) |
| GET | `/admin/tenants` | List tenants |
| GET | `/admin/tenants/{tenant_id}` | Get tenant details |
| DELETE | `/admin/tenants/{tenant_id}` | Deactivate tenant |

## Authentication

All endpoints (except `/health`) require a Bearer token:

```
Authorization: Bearer <service_name>:<secret>
```

Service roles:
- `client` — can tokenize and forward
- `admin` — can tokenize, manage tokens, and manage tenants
- `proxy` — can forward and detokenize (internal)

## Project Structure

```
cmd/
├── tokenizer/main.go       # Tokenization Service
├── proxy/main.go            # Reveal-Forward Proxy
└── migrate/main.go          # Database migrations

internal/
├── auth/                    # Bearer auth + RBAC + mTLS middleware
├── crypto/                  # AES-256-GCM, HMAC blind index, envelope encryption
├── handler/                 # HTTP handlers
├── kms/                     # AWS KMS client (LocalStack in dev)
├── model/                   # Domain entities
├── proxy/                   # Token revealer + HTTP forwarder
├── repository/              # PostgreSQL repositories
├── redis/                   # Redis CVV store
├── server/                  # Router, middleware, graceful shutdown
└── audit/                   # Structured audit logging

migrations/                  # SQL migration files
test/
├── mock-provider/           # Express app simulating a 3rd party provider
└── smoke.sh                 # End-to-end smoke test
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | required |
| `REDIS_URL` | Redis connection string | `redis://localhost:6379/0` |
| `KMS_KEY_ARN` | AWS KMS key ARN for envelope encryption | required |
| `KMS_ENDPOINT` | KMS endpoint (LocalStack override) | `http://localhost:4566` |
| `AWS_REGION` | AWS region | `us-east-1` |
| `AWS_ACCESS_KEY_ID` | AWS credentials (use `test` for LocalStack) | required |
| `AWS_SECRET_ACCESS_KEY` | AWS credentials (use `test` for LocalStack) | required |
| `HMAC_KEY` | Base64-encoded 32-byte key for PAN blind index | required |
| `CVV_TTL` | CVV time-to-live | `1h` |
| `PORT_TOKENIZER` | Tokenizer service port | `8080` |
| `PORT_PROXY` | Proxy service port | `8081` |
| `TOKENIZER_BASE_URL` | Proxy → Tokenizer URL | `http://localhost:8080` |
| `LOG_LEVEL` | Log level | `info` |
| `LOG_FORMAT` | Log format (json/text) | `json` |

## Tests

Integration tests run against real PostgreSQL, Redis, and LocalStack containers via [testcontainers-go](https://golang.testcontainers.org/). Docker must be running.

```bash
# Run all integration tests
go test -tags=integration -v -timeout=600s ./tests/integration/...

# Run a specific test
go test -tags=integration -v -timeout=120s -run TestTokenize ./tests/integration/...

# Run with race detection
go test -tags=integration -race -timeout=600s ./tests/integration/...
```

No manual setup needed — containers are created and destroyed automatically per test.

**Coverage areas**: tokenization, detokenization, forward/reveal, token lifecycle, authentication/RBAC, audit logging, tenant provisioning, multitenant isolation.

## Docker

Build service images:

```bash
docker build -f Dockerfile.tokenizer -t vault-tokenizer .
docker build -f Dockerfile.proxy -t vault-proxy .
```

Both use `gcr.io/distroless/static-debian12` as runtime base (no shell, minimal attack surface).

## Security

- PAN encrypted at rest with AES-256-GCM + envelope encryption (DEK/KEK via KMS)
- CVV stored ephemerally in Redis with TTL (default 1h), auto-deleted after single use (GETDEL)
- PAN masked in all logs (`411111******1111`), CVV never logged
- Proxy is stateless — no database, no persistent state, wipes card data from memory after forwarding
- mTLS enforced for internal detokenize endpoint
- RBAC on all endpoints

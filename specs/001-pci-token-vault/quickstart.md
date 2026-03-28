# Quickstart: PCI Token Vault & Payment Proxy

## Prerequisites

- Go 1.22+
- Docker & Docker Compose
- PostgreSQL 16+ (or use Docker)
- Redis 7+ (or use Docker)
- LocalStack (free Hobby plan) for local KMS — included in docker-compose

## Local Development Setup

### 1. Clone and configure

```bash
git clone <repo-url> && cd vault
cp .env.example .env
# Edit .env — defaults work for local dev with LocalStack
```

### 2. Start dependencies (PostgreSQL + Redis + LocalStack + mock PSP)

```bash
docker compose up -d
```

This starts:
- **PostgreSQL 16** on port 5432
- **Redis 7** on port 6379
- **LocalStack** on port 4566 (KMS for envelope encryption)
- **Mock PSP** (Express) on port 9090

### 3. Create KMS key in LocalStack

```bash
aws --endpoint-url=http://localhost:4566 kms create-key \
  --description "Vault KEK" \
  --query 'KeyMetadata.KeyId' --output text
# Copy the KeyId to .env as KMS_KEY_ARN
```

### 4. Run migrations

```bash
go run cmd/migrate/main.go up
```

### 5. Start services

```bash
# Tokenization Service (port 8080)
go run cmd/tokenizer/main.go

# Payment Proxy (port 8081)
go run cmd/proxy/main.go
```

### 6. Tokenize a card

```bash
curl -X POST http://localhost:8080/vault/tokenize \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <dev-token>" \
  -d '{
    "pan": "4111111111111111",
    "expiry_month": 12,
    "expiry_year": 2027,
    "cvv": "123"
  }'
```

Response:
```json
{
  "token": "tok_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
  "cvv_stored": true,
  "is_existing": false
}
```

### 7. Process a payment

```bash
curl -X POST http://localhost:8081/proxy/charge \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <dev-token>" \
  -d '{
    "token": "tok_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
    "amount": 5000,
    "currency": "USD",
    "idempotency_key": "order-12345"
  }'
```

### 8. Run tests

```bash
# Unit tests
go test ./...

# Integration tests (requires Docker dependencies running)
go test -tags=integration ./...

# Race detection
go test -race ./...
```

## Project Layout

```
cmd/
├── tokenizer/main.go    # Tokenization Service entrypoint
├── proxy/main.go        # Payment Proxy entrypoint
└── migrate/main.go      # Database migration tool

internal/
├── auth/                # Authentication & RBAC middleware
├── crypto/              # AES-256-GCM encryption, HMAC blind index
├── handler/             # HTTP handlers (tokenize, charge, manage)
├── kms/                 # KMS client abstraction (envelope encryption)
├── model/               # Domain entities (Token, VaultEntry, etc.)
├── provider/            # Reveal & forward: adapters for any 3rd party PSP
├── repository/          # PostgreSQL data access
├── redis/               # Redis CVV store client
├── server/              # HTTP server setup, middleware, routing
└── audit/               # Structured audit logging

migrations/              # SQL migration files
test/
└── mock-provider/       # Express app simulating a 3rd party PSP (port 9090)
config/                  # Configuration loading
docker-compose.yaml      # Local dev: PostgreSQL + Redis + mock-provider
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT_TOKENIZER` | Tokenization service port | `8080` |
| `PORT_PROXY` | Payment proxy port | `8081` |
| `DATABASE_URL` | PostgreSQL connection string | required |
| `REDIS_URL` | Redis connection string | required |
| `KMS_KEY_ARN` | KMS key for KEK operations | required |
| `KMS_ENDPOINT` | KMS endpoint override (LocalStack) | `http://localhost:4566` |
| `AWS_REGION` | AWS region (LocalStack uses any) | `us-east-1` |
| `HMAC_KEY` | Key for PAN blind index (base64) | required |
| `CVV_TTL` | CVV time-to-live | `1h` |
| `LOG_LEVEL` | Logging level | `info` |
| `LOG_FORMAT` | Log format (json/text) | `json` |
| `PROVIDER_BASE_URL` | 3rd party PSP endpoint | `http://localhost:9090` |

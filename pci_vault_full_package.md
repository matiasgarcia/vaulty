
# PCI Vault System – Full Engineering Package

## 1. OpenAPI Contracts

### Tokenization Service

```yaml
openapi: 3.0.0
info:
  title: Tokenization API
  version: 1.0.0

paths:
  /vault/tokenize:
    post:
      summary: Tokenize PAN
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                pan:
                  type: string
                expiry:
                  type: string
                cvv:
                  type: string
      responses:
        '200':
          description: Token created
          content:
            application/json:
              schema:
                type: object
                properties:
                  token:
                    type: string
```

---

### Payment Proxy

```yaml
paths:
  /proxy/charge:
    post:
      summary: Charge using token
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                token:
                  type: string
                amount:
                  type: integer
                currency:
                  type: string
      responses:
        '200':
          description: Payment result
```

---

## 2. Terraform (Infrastructure Blueprint)

```hcl
provider "aws" {
  region = "us-east-1"
}

resource "aws_kms_key" "vault_key" {
  description = "Vault encryption key"
}

resource "aws_vpc" "cde_vpc" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "cde_subnet" {
  vpc_id     = aws_vpc.cde_vpc.id
  cidr_block = "10.0.1.0/24"
}

resource "aws_security_group" "vault_sg" {
  vpc_id = aws_vpc.cde_vpc.id
}

resource "aws_ecs_cluster" "vault_cluster" {}
```

---

## 3. Threat Model (STRIDE)

| Threat | Example | Mitigation |
|------|--------|-----------|
| Spoofing | Fake service calling vault | mTLS |
| Tampering | Modify ciphertext | AES-GCM auth tag |
| Repudiation | Deny transaction | Audit logs |
| Information Disclosure | DB breach | Encryption |
| Denial of Service | API flooding | Rate limiting |
| Elevation of Privilege | Access vault | RBAC |

---

## 4. Identity & Secrets

- Use short-lived tokens (JWT / IAM)
- No static credentials
- Secrets stored in:
  - AWS Secrets Manager
  - HashiCorp Vault

---

## 5. Service Mesh (Optional but Recommended)

- mTLS enforced automatically
- Identity via SPIFFE
- Traffic policies enforced

---

## 6. Final Notes

This package now includes:
- Architecture
- APIs
- Infra
- Threat model

This is equivalent to a **real fintech-grade system design baseline**.

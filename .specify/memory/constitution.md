<!--
  Sync Impact Report
  ==================
  Version change: 0.0.0 → 1.0.0 (MAJOR - initial constitution ratification)

  Modified principles: N/A (initial version)

  Added sections:
    - Core Principles (5 principles)
    - Security Requirements
    - Development Workflow
    - Governance

  Removed sections: N/A

  Templates requiring updates:
    - .specify/templates/plan-template.md ✅ compatible (Constitution Check section exists)
    - .specify/templates/spec-template.md ✅ compatible (no constitution-specific gates)
    - .specify/templates/tasks-template.md ✅ compatible (phase structure aligns)

  Follow-up TODOs: None
-->

# Vault Constitution

## Core Principles

### I. PCI DSS Compliance First (NON-NEGOTIABLE)

All design and implementation decisions MUST prioritize PCI DSS compliance
above convenience, performance, or developer experience.

- PCI DSS Requirements 3, 4, 7, 8, 10, and 11 are mandatory gates for
  every feature that touches cardholder data.
- No feature MUST ship without a compliance review against applicable
  PCI DSS requirements.
- Non-compliance is a blocker — not a warning, not a TODO.

**Rationale**: The Vault exists to isolate and protect cardholder data.
Compliance is the product, not a side constraint.

### II. Zero Trust Security

No service, user, or network segment is trusted by default.

- All inter-service communication MUST use mTLS.
- Role-Based Access Control (RBAC) MUST be enforced on every endpoint.
- Least privilege access: services receive only the permissions their
  function requires.
- Network segmentation MUST isolate the Cardholder Data Environment (CDE)
  from public-facing components via Kubernetes namespace policies and
  private subnets.

**Rationale**: A payment vault is a high-value target. Defense in depth
ensures that a single compromised component cannot cascade into a
full breach.

### III. Encryption by Default

All sensitive data MUST be encrypted at rest and in transit with no
opt-out mechanism.

- At rest: AES-256-GCM with envelope encryption (DEK encrypted by KEK
  managed via external KMS/HSM).
- In transit: TLS 1.2+ minimum for all connections.
- Encryption keys MUST NOT be stored alongside encrypted data.
- Key rotation policies MUST be defined and enforced.

**Rationale**: Encryption is the last line of defense if access controls
fail. Defaulting to encrypted removes the risk of accidental plaintext
storage.

### IV. Ephemeral Sensitive Data

Data that is needed temporarily MUST have enforced time-to-live (TTL)
and single-use semantics.

- CVV MUST have a maximum TTL of 1 hour.
- CVV MUST be auto-deleted after first use OR expiration, whichever
  comes first.
- PAN MUST never exist in plaintext outside of in-memory decryption
  within the Payment Proxy service.
- Data retention MUST follow the principle of minimal necessary
  duration.

**Rationale**: Reducing the window and surface of sensitive data
exposure directly reduces breach impact and PCI DSS scope.

### V. Observability & Auditability

Every operation on cardholder data MUST produce an auditable trace.

- All tokenization, detokenization, and payment proxy operations MUST
  be logged with correlation IDs.
- PAN MUST be masked in all logs using the format `411111******1111`.
- CVV MUST NEVER appear in any log, metric, trace, or error message.
- Centralized logging with SIEM integration is required for production
  deployments.
- Structured logging format MUST be used across all services.

**Rationale**: PCI DSS Requirement 10 mandates comprehensive audit
trails. Observability also enables incident response and operational
debugging without exposing sensitive data.

## Security Requirements

- **Cryptographic stack**: AES-256-GCM (data at rest), TLS 1.2+
  (data in transit), external KMS/HSM for key management.
- **Authentication**: All API consumers MUST authenticate. Service-to-
  service authentication uses mTLS certificates.
- **Authorization**: RBAC enforced at the API gateway and at each
  service boundary. Only the Payment Proxy service is authorized to
  detokenize.
- **Token generation**: Tokens MUST be non-reversible and
  deterministic — the same PAN MUST always return the same token.
  The system MUST use a blind index (HMAC of PAN) for dedup lookup.
  Tokens MUST NOT be derivable from the PAN itself.
- **Input validation**: PAN MUST pass Luhn validation before
  tokenization. All inputs MUST be validated at system boundaries.
- **Deployment**: Kubernetes with network policies enforcing CDE
  isolation (namespaces: `cde-vault`, `cde-proxy`, `public-api`).

## Development Workflow

- **Specification first**: Every feature MUST begin with a spec
  (`/speckit.specify`) before implementation starts.
- **Constitution check**: Implementation plans MUST include a
  Constitution Check gate verifying alignment with all five principles.
- **Code review**: All changes touching cardholder data paths MUST
  receive security-focused review before merge.
- **Testing gates**: Integration tests MUST cover tokenization,
  encryption, TTL enforcement, and access control boundaries.
- **Idempotency**: Payment processing endpoints MUST be idempotent
  to prevent duplicate charges.
- **Error handling**: Error responses MUST use structured formats and
  MUST NOT leak internal state, stack traces, or sensitive data.

## Governance

This constitution is the authoritative governance document for the
Vault project. It supersedes conflicting guidance in any other
document.

- **Amendments**: Any change to this constitution MUST be documented
  with a version bump, rationale, and migration plan for affected
  components.
- **Versioning**: Semantic versioning applies — MAJOR for principle
  removals or redefinitions, MINOR for new principles or expanded
  guidance, PATCH for clarifications and wording fixes.
- **Compliance review**: Every implementation plan MUST pass the
  Constitution Check gate in `plan-template.md` before proceeding to
  Phase 0 research.
- **Enforcement**: All PRs and code reviews MUST verify compliance
  with these principles. Non-compliance MUST be resolved before merge.
- **Complexity justification**: Any architectural complexity beyond
  the minimum required MUST be justified in the plan's Complexity
  Tracking table.

**Version**: 1.1.0 | **Ratified**: 2026-03-28 | **Last Amended**: 2026-03-28

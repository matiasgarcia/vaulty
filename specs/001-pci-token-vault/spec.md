# Feature Specification: PCI-Compliant Token Vault & Payment Proxy

**Feature Branch**: `001-pci-token-vault`
**Created**: 2026-03-28
**Status**: Draft
**Input**: User description: "Read specs from pci_vault_consolidated_full.md"

## Clarifications

### Session 2026-03-28

- Q: When CVV has expired, should the system attempt payment without CVV or reject? → A: Always attempt payment without CVV; the provider decides acceptance.
- Q: Should the same PAN always return the same token, or produce a new token each time? → A: Same PAN MUST return the same token (deterministic 1:1 mapping). System MUST use a blind index (e.g., HMAC of PAN) to detect existing entries and return the existing token, updating CVV/expiry if new values provided.
- Q: What is the availability SLA target? → A: 99.999% uptime (~5.3 min downtime/year).
- Q: Can tokens be permanently deleted (hard-delete)? → A: No. Only soft-delete (logical deactivation). Token and vault data MUST be retained for audit trail purposes.
- Q: Should the system validate currency codes? → A: No validation. Currency is passed through to the payment provider as-is.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Tokenize Payment Card Data (Priority: P1)

A merchant integration sends a customer's card number (PAN), expiration
date, and CVV to the system. The system validates the card number,
generates a unique token, securely stores the PAN, temporarily holds
the CVV, and returns the token to the merchant. From this point on,
the merchant uses only the token to reference the card — never the
raw card data.

**Why this priority**: Tokenization is the foundational capability.
Without it, no other feature (payment, retrieval, audit) can function.
This is the MVP.

**Independent Test**: Submit a valid PAN + CVV via the tokenization
endpoint and verify a token is returned, the PAN is stored encrypted,
and the CVV is held temporarily.

**Acceptance Scenarios**:

1. **Given** a valid PAN (passes Luhn), expiration date, and CVV,
   **When** the client sends a tokenization request,
   **Then** the system returns a unique token and stores the PAN
   encrypted and the CVV with a time-limited lifespan.

2. **Given** an invalid PAN (fails Luhn validation),
   **When** the client sends a tokenization request,
   **Then** the system rejects the request with a structured error
   indicating invalid card number.

3. **Given** the same PAN is tokenized twice,
   **When** two separate tokenization requests are made,
   **Then** the same token is returned both times (deterministic 1:1
   mapping), and the CVV/expiry are updated if new values are provided.

4. **Given** a CVV is stored during tokenization,
   **When** 1 hour elapses without the CVV being used,
   **Then** the CVV is automatically deleted.

---

### User Story 2 - Reveal and Forward via Proxy (Priority: P2)

A backend client sends a JSON payload containing one or more tokens
along with a destination URL. The Proxy scans the payload for token
patterns (`tok_...`), reveals each token (replaces with real PAN +
expiry, retrieves CVV if available), and forwards the complete
payload — with tokens replaced by real values — to the destination.
The destination's response is returned verbatim. The Proxy stores
NOTHING and wipes sensitive data from memory after forwarding.

**Why this priority**: The reveal-and-forward capability is the
primary business value. It enables backend systems to interact with
3rd party providers (payment processors, identity verifiers, etc.)
without ever handling raw card data.

**Independent Test**: Tokenize a card, then submit a forward request
with a payload containing the token and a mock destination. Verify
the mock destination receives the real PAN/CVV and the response is
returned verbatim.

**Acceptance Scenarios**:

1. **Given** a JSON payload containing a valid active token,
   **When** the client sends a forward request with a destination URL,
   **Then** the Proxy reveals the token (replaces with real PAN +
   expiry + CVV if available), forwards the modified payload to the
   destination, and returns the destination's response verbatim.

2. **Given** a payload with a token whose CVV has expired,
   **When** the client sends a forward request,
   **Then** the Proxy reveals the PAN and expiry but omits CVV,
   forwards the payload, and returns the destination's response.

3. **Given** a payload containing an invalid or inactive token,
   **When** the client sends a forward request,
   **Then** the Proxy rejects the request with a structured error
   without forwarding anything to the destination.

4. **Given** a payload with no token patterns,
   **When** the client sends a forward request,
   **Then** the Proxy forwards the payload unchanged to the
   destination.

---

### User Story 3 - Manage Token Lifecycle (Priority: P3)

An administrator or system process needs to validate token status,
deactivate compromised tokens, and review token usage history for
audit and compliance purposes.

**Why this priority**: Token lifecycle management is essential for
operational control and PCI DSS compliance but is not required for
the core tokenize-and-pay flow.

**Independent Test**: Create a token, deactivate it, then verify it
cannot be used for payments. Review the usage history to confirm all
operations are logged.

**Acceptance Scenarios**:

1. **Given** an active token,
   **When** an authorized user requests token validation,
   **Then** the system confirms the token exists and is active.

2. **Given** an active token,
   **When** an authorized user deactivates the token,
   **Then** the token status changes to inactive and subsequent
   payment attempts are rejected.

3. **Given** a token with usage history,
   **When** an authorized user requests the audit trail,
   **Then** the system returns a chronological log of all operations
   performed with that token.

---

### User Story 4 - Authenticate and Authorize Access (Priority: P4)

All clients and services interacting with the system MUST be
authenticated and authorized. Each operation (tokenize, detokenize,
pay, manage) is gated by role-based access control. Only the payment
proxy service is authorized to detokenize.

**Why this priority**: Security is non-negotiable, but the auth
framework is infrastructure that supports the other stories rather
than being a standalone user-facing journey.

**Independent Test**: Attempt to call the detokenization endpoint
from an unauthorized service and verify access is denied. Then call
from the authorized payment proxy and verify access is granted.

**Acceptance Scenarios**:

1. **Given** an unauthenticated request,
   **When** any endpoint is called,
   **Then** the system rejects the request with an authentication
   error.

2. **Given** an authenticated service without the "detokenize" role,
   **When** the service attempts to retrieve a PAN,
   **Then** the system rejects the request with an authorization
   error.

3. **Given** the payment proxy service with the "detokenize" role,
   **When** the service requests PAN retrieval for a valid token,
   **Then** the system decrypts and returns the PAN.

---

### Edge Cases

- What happens when the CVV store is temporarily unavailable during
  tokenization? The system MUST still tokenize the PAN and return
  a token, but MUST indicate that CVV storage failed.
- What happens when the payment provider is unreachable? The system
  MUST return a structured error and MUST NOT mark the payment as
  successful.
- What happens when the KMS/HSM is unavailable? The system MUST
  reject tokenization and payment requests rather than falling back
  to insecure storage.
- How does the system handle concurrent payment requests for the
  same token with the same CVV? Only the first request MUST consume
  the CVV; subsequent requests proceed without it.
- What happens when a token is deactivated while a payment is
  in-flight? The in-flight payment MUST complete; subsequent
  requests MUST be rejected.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accept PAN, expiration date, and CVV and
  return a unique, non-reversible token.
- **FR-002**: System MUST validate PAN format using the Luhn
  algorithm before tokenization.
- **FR-003**: System MUST generate deterministic tokens — the same
  PAN tokenized multiple times MUST return the same token. The system
  MUST use a blind index (e.g., HMAC of PAN) to detect existing
  entries. If the PAN already exists, the system MUST return the
  existing token and update CVV/expiry if new values are provided.
- **FR-004**: System MUST store PAN only in encrypted form, with
  metadata (creation time, last usage, status).
- **FR-005**: System MUST store CVV in ephemeral storage with a
  configurable TTL (default: 1 hour) and auto-delete after first
  use or expiration.
- **FR-006**: System MUST restrict PAN retrieval (detokenization)
  exclusively to the Payment Proxy service.
- **FR-007**: System MUST accept a JSON payload containing tokens
  and a destination URL, scan the payload for token patterns,
  reveal each token (replace with real PAN + expiry + CVV if
  available), forward the modified payload to the destination, and
  return the destination's response verbatim. The Proxy MUST NOT
  store any part of the payload or the destination's response.
- **FR-008**: System MUST return structured error responses for
  invalid tokens, expired CVVs, provider errors, and internal
  failures.
- **FR-009**: System MUST validate token existence and status
  (active/inactive) before any operation.
- **FR-010**: System MUST support logical deactivation (soft-delete)
  of tokens and prevent usage of inactive tokens. Hard-delete of
  tokens is NOT permitted; all token and vault data MUST be retained
  for audit trail purposes.
- **FR-011**: System MUST log all tokenization requests,
  detokenization attempts, payment events, and access attempts.
- **FR-012**: System MUST maintain per-token usage history with
  timestamps for audit traceability.
- **FR-013**: System MUST authenticate all clients and services.
- **FR-014**: System MUST enforce role-based access control (RBAC)
  on all operations.
- **FR-015**: System MUST support concurrent requests and maintain
  99.999% uptime (~5.3 minutes downtime per year).
- **FR-016**: [REMOVED — idempotency is the calling client's
  responsibility, not the vault/proxy system.]
- **FR-017**: System MUST allow configuration of CVV TTL with a
  default of 1 hour.
- **FR-018**: System MUST support integration with one or more
  payment providers with configurable endpoints and credentials.

### Key Entities

- **Token**: Unique non-reversible alias for a PAN with a 1:1
  deterministic mapping. Attributes: token ID, PAN blind index
  (HMAC for dedup lookup), status (active/inactive), creation time,
  last usage timestamp.
- **Vault Entry**: Encrypted PAN record. Attributes: token
  reference, encrypted PAN (ciphertext), initialization vector,
  authentication tag, encrypted DEK, creation timestamp.
- **CVV Record**: Ephemeral CVV storage. Attributes: token
  reference, CVV value, TTL, creation timestamp, usage status.
- **Audit Log Entry**: Immutable record of an operation. Attributes:
  operation type, token reference (masked), actor, timestamp,
  result, correlation ID.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A valid card can be tokenized and a token returned in
  under 2 seconds end-to-end.
- **SC-002**: System handles at least 1,000 concurrent tokenization
  requests without degradation.
- **SC-002a**: System maintains 99.999% uptime measured monthly.
- **SC-003**: 100% of CVV records are deleted within 5 seconds after
  TTL expiration or first use.
- **SC-004**: 100% of PAN values are stored encrypted — zero
  plaintext PAN instances exist at rest.
- **SC-005**: Reveal-and-forward completes (including destination
  round-trip) in under 5 seconds for 95% of requests.
- **SC-006**: All operations produce audit log entries with
  correlation IDs — zero untracked operations.
- **SC-007**: Unauthorized detokenization attempts are rejected
  100% of the time.
- **SC-008**: [REMOVED — idempotency is the calling client's
  responsibility.]
- **SC-009**: System passes PCI DSS compliance review for
  Requirements 3, 4, 7, 8, 10, and 11.

## Assumptions

- The system operates in a cloud environment with access to a
  managed key management service (KMS/HSM).
- A container orchestration platform (Kubernetes or equivalent) is
  available for deployment with namespace isolation.
- At least one payment provider (e.g., Stripe, Adyen) is available
  for integration during initial development.
- Service-to-service communication occurs within a private network
  where mTLS can be enforced.
- The initial release targets server-to-server integrations only —
  no direct browser or mobile SDK is in scope.
- Horizontal scaling of individual services is supported by the
  deployment platform.
- Centralized logging infrastructure (SIEM-compatible) is available
  or will be provisioned as part of deployment.

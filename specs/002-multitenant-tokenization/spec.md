# Feature Specification: Multitenant Tokenization

**Feature Branch**: `002-multitenant-tokenization`
**Created**: 2026-03-28
**Status**: Draft
**Input**: User description: "The application must be multitenant now. That means that for the same PAN, but different TENANT, the token returned must differ. However, if the PAN was already tokenized in a given tenant and the same PAN is provided, the same TOKEN must be returned."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Tenant-Scoped Tokenization (Priority: P1)

A backend client belonging to Tenant A tokenizes a customer's card.
The system returns a token scoped to Tenant A. When Tenant B
tokenizes the exact same card number, a different token is returned.
Each tenant's token namespace is completely isolated — tokens from
one tenant cannot be used, resolved, or referenced by another tenant.

**Why this priority**: This is the core multitenancy requirement.
Without tenant-scoped tokens, the system cannot serve multiple
merchants/organizations securely on the same infrastructure.

**Independent Test**: Tokenize the same PAN from two different tenants.
Verify each receives a different token. Verify each token resolves
only within its own tenant context.

**Acceptance Scenarios**:

1. **Given** Tenant A tokenizes PAN `4111111111111111`,
   **When** the system processes the request,
   **Then** a token scoped to Tenant A is returned (e.g., `tok_aaa...`).

2. **Given** Tenant B tokenizes the same PAN `4111111111111111`,
   **When** the system processes the request,
   **Then** a different token scoped to Tenant B is returned (e.g., `tok_bbb...`).

3. **Given** Tenant A tokenizes the same PAN `4111111111111111` again,
   **When** the system processes the request,
   **Then** the original Tenant A token (`tok_aaa...`) is returned
   (deterministic within tenant).

4. **Given** Tenant A's token `tok_aaa...`,
   **When** Tenant B attempts to use it (detokenize, forward, status),
   **Then** the system rejects the request as if the token does not exist.

---

### User Story 2 - Tenant Identification on All Operations (Priority: P2)

Every API request MUST include a tenant identifier. All existing
operations (tokenize, detokenize, forward, token management, audit)
MUST be scoped to the requesting tenant. No cross-tenant data
leakage is permitted.

**Why this priority**: Tenant scoping must apply across the entire
system, not just tokenization. Without this, existing operations
(forward, lifecycle, audit) would break tenant isolation.

**Independent Test**: Perform a full flow (tokenize → forward →
deactivate → audit) for two separate tenants on the same PAN.
Verify complete isolation at every step.

**Acceptance Scenarios**:

1. **Given** a request without a tenant identifier,
   **When** any endpoint is called,
   **Then** the system rejects the request with a clear error
   indicating the tenant is required.

2. **Given** Tenant A has tokenized a card,
   **When** Tenant A requests the audit trail for their token,
   **Then** only Tenant A's operations are returned — no Tenant B
   operations are visible.

3. **Given** Tenant A deactivates their token,
   **When** Tenant B queries the same PAN (which they also tokenized),
   **Then** Tenant B's token remains active and unaffected.

---

### User Story 3 - Tenant Provisioning (Priority: P3)

New tenants can be onboarded to the system. Each tenant receives a
unique identifier and credentials for API access. Tenant metadata
(name, status) is stored for operational purposes.

**Why this priority**: Without a way to create and manage tenants,
the system cannot onboard new customers. However, for an initial
release, tenants can be provisioned manually or via configuration.

**Independent Test**: Create a new tenant, tokenize a card under
that tenant, verify the token is scoped correctly.

**Acceptance Scenarios**:

1. **Given** a new tenant is provisioned with a unique identifier,
   **When** the tenant makes their first tokenization request,
   **Then** the system accepts it and returns a tenant-scoped token.

2. **Given** a tenant is deactivated,
   **When** the tenant attempts any operation,
   **Then** the system rejects the request indicating the tenant
   is inactive.

---

### Edge Cases

- What happens when a tenant is deactivated while tokens exist?
  Existing tokens MUST be preserved for audit purposes but all
  operations (tokenize, detokenize, forward) MUST be rejected.
- What happens if the tenant identifier format is invalid? The
  system MUST reject the request with a clear validation error.
- Can a token from a deactivated tenant be re-activated if the
  tenant is re-activated? Yes — token status is independent of
  tenant status; only new operations are blocked on inactive tenants.
- What is the maximum number of tenants? The system MUST support
  at least 10,000 tenants without performance degradation.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every API request (except health check) MUST include
  a tenant identifier. Requests without a tenant MUST be rejected.
- **FR-002**: The blind index used for PAN deduplication MUST be
  scoped per tenant. The same PAN under different tenants MUST
  produce different blind index values.
- **FR-003**: Token generation MUST be deterministic within a
  tenant — same PAN + same tenant always returns the same token.
  Same PAN + different tenant MUST return different tokens.
- **FR-004**: All data access (tokens, vault entries, audit logs)
  MUST be filtered by tenant. No query MUST ever return data
  belonging to another tenant.
- **FR-005**: The reveal-and-forward proxy MUST validate that the
  tokens in the payload belong to the requesting tenant before
  revealing them.
- **FR-006**: Token lifecycle operations (status, deactivation,
  audit trail) MUST be scoped to the requesting tenant.
- **FR-007**: The system MUST support provisioning new tenants with
  a unique identifier, name, and active/inactive status.
- **FR-008**: Inactive tenants MUST be blocked from all operations
  except health checks. Their existing data MUST be preserved.
- **FR-009**: Audit log entries MUST include the tenant identifier
  for every operation.
- **FR-010**: The system MUST support at least 10,000 tenants
  without degradation in tokenization or forwarding performance.
- **FR-011**: Each tenant MUST have a dedicated KMS key (KEK) for
  envelope encryption. The key MUST be created during tenant
  provisioning and its ARN stored in the tenant record. A
  compromised tenant key MUST NOT affect other tenants' data.

### Key Entities

- **Tenant**: An organization or merchant using the vault. Attributes:
  tenant ID (unique identifier), name, status (active/inactive),
  KMS key ARN (dedicated per-tenant encryption key), created timestamp.
- **Token** (modified): Now includes a tenant reference. The
  uniqueness constraint on blind index becomes per-tenant.
- **Audit Log Entry** (modified): Now includes a tenant identifier
  for every entry.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Same PAN tokenized by two different tenants always
  produces two different tokens — 100% isolation.
- **SC-002**: Same PAN tokenized twice by the same tenant always
  returns the identical token — 100% deterministic.
- **SC-003**: Cross-tenant data access attempts are rejected 100%
  of the time — zero data leakage.
- **SC-004**: System supports 10,000+ tenants with no measurable
  performance degradation compared to single-tenant baseline.
- **SC-005**: All existing functionality (tokenize, forward,
  lifecycle, audit) continues to work correctly with tenant
  scoping — zero regression.
- **SC-006**: Tenant provisioning and deactivation complete in
  under 1 second.

## Assumptions

- Tenant identity is provided via a request header or extracted
  from the authenticated service's credentials (Bearer token).
- Tenant provisioning for the initial release can be done via
  direct API call or configuration — a full admin UI is out of scope.
- Each tenant has its own dedicated KMS key (KEK). A new KMS key
  is created when a tenant is provisioned. The tenant's KMS key ARN
  is stored as part of the tenant record.
- The blind index is made tenant-specific by incorporating the
  tenant ID into the HMAC computation (e.g., HMAC(tenant_id + PAN)).
- Existing single-tenant data (if any) will need a migration to
  assign a default tenant. Migration strategy is part of the plan.

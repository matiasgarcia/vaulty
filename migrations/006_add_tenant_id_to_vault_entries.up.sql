ALTER TABLE vault_entries ADD COLUMN tenant_id VARCHAR(64) NOT NULL;

CREATE INDEX idx_vault_entries_tenant_id ON vault_entries (tenant_id);

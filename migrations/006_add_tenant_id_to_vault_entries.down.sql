DROP INDEX IF EXISTS idx_vault_entries_tenant_id;
ALTER TABLE vault_entries DROP COLUMN IF EXISTS tenant_id;

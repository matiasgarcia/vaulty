DROP INDEX IF EXISTS idx_audit_log_tenant_id;
ALTER TABLE audit_log DROP COLUMN IF EXISTS tenant_id;

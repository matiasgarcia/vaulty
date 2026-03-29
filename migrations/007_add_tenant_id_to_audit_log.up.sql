ALTER TABLE audit_log ADD COLUMN tenant_id VARCHAR(64) NOT NULL;

CREATE INDEX idx_audit_log_tenant_id ON audit_log (tenant_id);

ALTER TABLE tokens ADD COLUMN tenant_id VARCHAR(64) NOT NULL;

DROP INDEX IF EXISTS idx_tokens_pan_blind_index;

CREATE UNIQUE INDEX idx_tokens_tenant_blind_index ON tokens (tenant_id, pan_blind_index);
CREATE INDEX idx_tokens_tenant_id ON tokens (tenant_id);

ALTER TABLE tokens ADD CONSTRAINT fk_tokens_tenant
    FOREIGN KEY (tenant_id) REFERENCES tenants(tenant_id);

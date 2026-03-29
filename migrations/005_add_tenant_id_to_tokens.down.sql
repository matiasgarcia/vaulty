ALTER TABLE tokens DROP CONSTRAINT IF EXISTS fk_tokens_tenant;
DROP INDEX IF EXISTS idx_tokens_tenant_id;
DROP INDEX IF EXISTS idx_tokens_tenant_blind_index;
CREATE UNIQUE INDEX idx_tokens_pan_blind_index ON tokens (pan_blind_index);
ALTER TABLE tokens DROP COLUMN IF EXISTS tenant_id;

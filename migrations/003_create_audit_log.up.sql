CREATE TABLE IF NOT EXISTS audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    correlation_id VARCHAR(64) NOT NULL,
    operation VARCHAR(32) NOT NULL,
    token_id_masked VARCHAR(64),
    actor VARCHAR(128) NOT NULL,
    result VARCHAR(16) NOT NULL,
    detail JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_log_correlation_id ON audit_log (correlation_id);
CREATE INDEX idx_audit_log_token_id_masked ON audit_log (token_id_masked);
CREATE INDEX idx_audit_log_created_at ON audit_log (created_at);

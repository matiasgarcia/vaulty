CREATE TABLE IF NOT EXISTS tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_id VARCHAR(64) NOT NULL,
    pan_blind_index VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    expiry_month INTEGER NOT NULL,
    expiry_year INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_tokens_token_id ON tokens (token_id);
CREATE UNIQUE INDEX idx_tokens_pan_blind_index ON tokens (pan_blind_index);
CREATE INDEX idx_tokens_status ON tokens (status);

CREATE TABLE IF NOT EXISTS vault_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_id VARCHAR(64) NOT NULL REFERENCES tokens(token_id),
    pan_ciphertext BYTEA NOT NULL,
    iv BYTEA NOT NULL,
    auth_tag BYTEA NOT NULL,
    dek_encrypted BYTEA NOT NULL,
    kms_key_id VARCHAR(256) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vault_entries_token_id UNIQUE (token_id)
);

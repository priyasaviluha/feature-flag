CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    key_prefix VARCHAR(20) NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    environment_id UUID NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    status VARCHAR(10) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'REVOKED')),
    created_by UUID NOT NULL REFERENCES admin(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    CONSTRAINT api_keys_revocation_consistency CHECK ((status = 'REVOKED') = (revoked_at IS NOT NULL))
);

CREATE INDEX idx_api_keys_environment_created ON api_keys(environment_id, created_at DESC);

CREATE TABLE api_key_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id UUID NOT NULL REFERENCES admin(id) ON DELETE RESTRICT,
    api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
    environment_id UUID NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    action VARCHAR(30) NOT NULL CHECK (action IN ('API_KEY_REVOKED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_api_key_audit_logs_environment_created
    ON api_key_audit_logs(environment_id, created_at DESC);

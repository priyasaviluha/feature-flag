
CREATE TABLE admin (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    singleton_id SMALLINT NOT NULL DEFAULT 1,

    email VARCHAR(255) NOT NULL,

    password_hash TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT admin_singleton_check
        CHECK (singleton_id = 1),

    CONSTRAINT admin_singleton_unique
        UNIQUE (singleton_id)
);

CREATE UNIQUE INDEX admin_email_unique_idx
ON admin (LOWER(email));


CREATE TABLE environments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    name VARCHAR(50) NOT NULL UNIQUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    archived_at TIMESTAMPTZ
);

CREATE TABLE feature_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(150) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(30) NOT NULL
        CHECK (type IN ('BOOLEAN', 'NUMBER', 'THRESHOLD', 'PERCENTAGE')),
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at TIMESTAMPTZ
);

CREATE TABLE environment_feature_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    environment_id UUID NOT NULL,
    feature_id UUID NOT NULL,

    config JSONB NOT NULL,

    version BIGINT NOT NULL DEFAULT 1,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_environment
        FOREIGN KEY (environment_id)
        REFERENCES environments(id),

    CONSTRAINT fk_feature
        FOREIGN KEY (feature_id)
        REFERENCES feature_definitions(id),

    CONSTRAINT unique_environment_feature
        UNIQUE (environment_id, feature_id),

    CONSTRAINT config_version_positive
        CHECK (version >= 1)
);

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    environment_id UUID NOT NULL,
    feature_id UUID NOT NULL,

    action VARCHAR(20) NOT NULL,

    old_value JSONB,
    new_value JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_audit_environment
        FOREIGN KEY (environment_id)
        REFERENCES environments(id),

    CONSTRAINT fk_audit_feature
        FOREIGN KEY (feature_id)
        REFERENCES feature_definitions(id)
);


-- Fetch configurations by feature
CREATE INDEX idx_environment_feature_configs_feature
    ON environment_feature_configs(feature_id);

-- Fetch audit history for a feature in an environment
CREATE INDEX idx_audit_logs_environment_feature_created
    ON audit_logs(environment_id, feature_id, created_at DESC);

-- Fetch recent audit activity for an environment
CREATE INDEX idx_audit_logs_environment_created
    ON audit_logs(environment_id, created_at DESC);

CREATE OR REPLACE FUNCTION validate_feature_config()
RETURNS TRIGGER AS $$
DECLARE
    feature_type VARCHAR(30);
BEGIN
    SELECT type
    INTO feature_type
    FROM feature_definitions
    WHERE id = NEW.feature_id;

    IF feature_type = 'BOOLEAN' THEN
        IF jsonb_typeof(NEW.config) IS DISTINCT FROM 'object'
           OR jsonb_typeof(NEW.config -> 'enabled') IS DISTINCT FROM 'boolean'
           OR (NEW.config - 'enabled') IS DISTINCT FROM '{}'::jsonb THEN
            RAISE EXCEPTION 'Invalid BOOLEAN configuration';
        END IF;

    ELSIF feature_type = 'NUMBER' THEN
        IF jsonb_typeof(NEW.config) IS DISTINCT FROM 'object'
           OR jsonb_typeof(NEW.config -> 'value') IS DISTINCT FROM 'number'
           OR (NEW.config - 'value') IS DISTINCT FROM '{}'::jsonb THEN
            RAISE EXCEPTION 'Invalid NUMBER configuration';
        END IF;

    ELSIF feature_type = 'THRESHOLD' THEN
        IF jsonb_typeof(NEW.config) IS DISTINCT FROM 'object'
           OR jsonb_typeof(NEW.config -> 'operator') IS DISTINCT FROM 'string'
           OR NEW.config ->> 'operator' NOT IN ('>', '>=', '<', '<=', '=', '!=')
           OR jsonb_typeof(NEW.config -> 'threshold') IS DISTINCT FROM 'number'
           OR jsonb_typeof(NEW.config -> 'context_key') IS DISTINCT FROM 'string'
           OR (NEW.config - ARRAY['operator', 'threshold', 'context_key']) IS DISTINCT FROM '{}'::jsonb THEN
            RAISE EXCEPTION 'Invalid THRESHOLD configuration';
        END IF;

    ELSIF feature_type = 'PERCENTAGE' THEN
        IF jsonb_typeof(NEW.config) IS DISTINCT FROM 'object'
           OR jsonb_typeof(NEW.config -> 'enabled') IS DISTINCT FROM 'boolean'
           OR jsonb_typeof(NEW.config -> 'percentage') IS DISTINCT FROM 'number'
           OR (NEW.config - ARRAY['enabled', 'percentage']) IS DISTINCT FROM '{}'::jsonb
           OR (NEW.config ->> 'percentage')::numeric NOT BETWEEN 0 AND 100 THEN
            RAISE EXCEPTION 'Invalid PERCENTAGE configuration';
        END IF;

    ELSE
        RAISE EXCEPTION 'Unknown feature type: %', feature_type;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_validate_feature_config
BEFORE INSERT OR UPDATE OF feature_id, config
ON environment_feature_configs
FOR EACH ROW
EXECUTE FUNCTION validate_feature_config();

CREATE OR REPLACE FUNCTION prevent_audit_log_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Audit logs are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_prevent_audit_log_update
BEFORE UPDATE OR DELETE
ON audit_logs
FOR EACH ROW
EXECUTE FUNCTION prevent_audit_log_modification();

CREATE OR REPLACE FUNCTION prevent_configured_feature_type_change()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.type IS DISTINCT FROM OLD.type
       AND EXISTS (
           SELECT 1
           FROM environment_feature_configs
           WHERE feature_id = OLD.id
       ) THEN
        RAISE EXCEPTION
            'Cannot change feature type after configurations exist';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_prevent_feature_type_change
BEFORE UPDATE OF type
ON feature_definitions
FOR EACH ROW
EXECUTE FUNCTION prevent_configured_feature_type_change();

CREATE TRIGGER trg_prevent_audit_log_truncate
BEFORE TRUNCATE
ON audit_logs
FOR EACH STATEMENT
EXECUTE FUNCTION prevent_audit_log_modification();
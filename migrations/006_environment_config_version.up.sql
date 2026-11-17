ALTER TABLE environments
    ADD COLUMN config_version BIGINT NOT NULL DEFAULT 0
    CHECK (config_version >= 0);

-- Existing configurations form the initial snapshot revision.
UPDATE environments e
SET config_version = 1
WHERE EXISTS (
    SELECT 1 FROM environment_feature_configs c WHERE c.environment_id = e.id
);

CREATE OR REPLACE FUNCTION bump_environment_config_version()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        UPDATE environments SET config_version = config_version + 1 WHERE id = OLD.environment_id;
        RETURN OLD;
    END IF;

    UPDATE environments SET config_version = config_version + 1 WHERE id = NEW.environment_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_bump_environment_config_version
AFTER INSERT OR UPDATE OR DELETE ON environment_feature_configs
FOR EACH ROW EXECUTE FUNCTION bump_environment_config_version();

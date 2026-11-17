DROP TRIGGER IF EXISTS trg_bump_environment_config_version ON environment_feature_configs;
DROP FUNCTION IF EXISTS bump_environment_config_version();
ALTER TABLE environments DROP COLUMN IF EXISTS config_version;

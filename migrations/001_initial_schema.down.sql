DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS environment_feature_configs;
DROP TABLE IF EXISTS feature_definitions;
DROP TABLE IF EXISTS environments;
DROP TABLE IF EXISTS admin;

DROP FUNCTION IF EXISTS prevent_audit_log_modification();
DROP FUNCTION IF EXISTS validate_feature_config();
DROP FUNCTION IF EXISTS prevent_configured_feature_type_change();
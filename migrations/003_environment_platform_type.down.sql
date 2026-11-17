-- Restore the legacy labels only when they still match the labels written by
-- this migration. Remove only Android/iOS rows created by migration 003, and
-- only when no configuration, audit, or group rule refers to them.
UPDATE environments
SET name = 'Development'
WHERE platform = 'web' AND type = 'development' AND name = 'Web Development'
  AND NOT EXISTS (SELECT 1 FROM environments WHERE name = 'Development');

UPDATE environments
SET name = 'Staging'
WHERE platform = 'web' AND type = 'staging' AND name = 'Web Staging'
  AND NOT EXISTS (SELECT 1 FROM environments WHERE name = 'Staging');

UPDATE environments
SET name = 'Production'
WHERE platform = 'web' AND type = 'production' AND name = 'Web Production'
  AND NOT EXISTS (SELECT 1 FROM environments WHERE name = 'Production');

DELETE FROM environments AS environment
WHERE environment.platform IN ('android', 'ios')
  AND environment.name IN (
      'Android Development', 'Android Staging', 'Android Production',
      'iOS Development', 'iOS Staging', 'iOS Production'
  )
  AND NOT EXISTS (
      SELECT 1 FROM environment_feature_configs AS config
      WHERE config.environment_id = environment.id
  )
  AND NOT EXISTS (
      SELECT 1 FROM audit_logs AS audit
      WHERE audit.environment_id = environment.id
  )
  AND NOT EXISTS (
      SELECT 1 FROM feature_group_rules AS rule
      WHERE rule.environment_id = environment.id
  );

ALTER TABLE environments
    DROP CONSTRAINT environments_platform_type_unique,
    DROP CONSTRAINT environments_platform_check,
    DROP CONSTRAINT environments_type_check,
    DROP COLUMN platform,
    DROP COLUMN type;

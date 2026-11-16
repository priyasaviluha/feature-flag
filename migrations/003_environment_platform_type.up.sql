-- Keep the existing environment rows and their IDs so dependent configurations
-- and audit records continue to reference the same environments.
ALTER TABLE environments
    ADD COLUMN platform VARCHAR(20),
    ADD COLUMN type VARCHAR(20);

UPDATE environments
SET name = 'Web Development', platform = 'web', type = 'development'
WHERE name = 'Development';

UPDATE environments
SET name = 'Web Staging', platform = 'web', type = 'staging'
WHERE name = 'Staging';

UPDATE environments
SET name = 'Web Production', platform = 'web', type = 'production'
WHERE name = 'Production';

ALTER TABLE environments
    ALTER COLUMN platform SET NOT NULL,
    ALTER COLUMN type SET NOT NULL;

ALTER TABLE environments
    ADD CONSTRAINT environments_platform_check
        CHECK (platform IN ('web', 'android', 'ios')),
    ADD CONSTRAINT environments_type_check
        CHECK (type IN ('development', 'staging', 'production')),
    ADD CONSTRAINT environments_platform_type_unique
        UNIQUE (platform, type);

-- Retain environments_name_key from migration 001: the required display names
-- are distinct, while platform/type now provide the environment identity.

INSERT INTO environments (name, platform, type)
VALUES
    ('Android Development', 'android', 'development'),
    ('Android Staging', 'android', 'staging'),
    ('Android Production', 'android', 'production'),
    ('iOS Development', 'ios', 'development'),
    ('iOS Staging', 'ios', 'staging'),
    ('iOS Production', 'ios', 'production');

CREATE TABLE user_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at TIMESTAMPTZ
);

CREATE TABLE feature_group_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    environment_id UUID NOT NULL
        REFERENCES environments(id) ON DELETE RESTRICT,

    feature_id UUID NOT NULL
        REFERENCES feature_definitions(id) ON DELETE RESTRICT,

    group_id UUID NOT NULL
        REFERENCES user_groups(id) ON DELETE RESTRICT,

    config JSONB NOT NULL,

    version BIGINT NOT NULL DEFAULT 1,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (environment_id, feature_id, group_id)
);

CREATE INDEX idx_feature_group_rules_feature
ON feature_group_rules(feature_id);

CREATE INDEX idx_feature_group_rules_group
ON feature_group_rules(group_id);

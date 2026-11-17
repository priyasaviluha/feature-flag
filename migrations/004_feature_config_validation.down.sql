DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM environment_feature_configs AS config
        JOIN feature_definitions AS feature
            ON feature.id = config.feature_id
        WHERE feature.type = 'THRESHOLD'
          AND config.config ? 'enabled'
    ) THEN
        RAISE EXCEPTION 'Cannot rollback feature config validation: canonical THRESHOLD configurations exist';
    END IF;
END;
$$;

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

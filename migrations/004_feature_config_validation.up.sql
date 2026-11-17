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
        IF jsonb_typeof(NEW.config) IS DISTINCT FROM 'object' THEN
            RAISE EXCEPTION 'Invalid THRESHOLD configuration';
        END IF;

        -- Keep accepting the legacy shape exactly as before. Existing rows are
        -- not rewritten, and future updates to these rows remain valid.
        IF NOT (NEW.config ? 'enabled') THEN
            IF jsonb_typeof(NEW.config -> 'operator') IS DISTINCT FROM 'string'
               OR NEW.config ->> 'operator' NOT IN ('>', '>=', '<', '<=', '=', '!=')
               OR jsonb_typeof(NEW.config -> 'threshold') IS DISTINCT FROM 'number'
               OR jsonb_typeof(NEW.config -> 'context_key') IS DISTINCT FROM 'string'
               OR (NEW.config - ARRAY['operator', 'threshold', 'context_key']) IS DISTINCT FROM '{}'::jsonb THEN
                RAISE EXCEPTION 'Invalid THRESHOLD configuration';
            END IF;
        ELSE
            IF jsonb_typeof(NEW.config -> 'enabled') IS DISTINCT FROM 'boolean'
               OR jsonb_typeof(NEW.config -> 'operator') IS DISTINCT FROM 'string'
               OR NEW.config ->> 'operator' NOT IN ('>=', '<=', '=')
               OR jsonb_typeof(NEW.config -> 'threshold') IS DISTINCT FROM 'number'
               OR NOT (NEW.config ? 'context_key')
               OR (NEW.config - ARRAY['enabled', 'operator', 'threshold', 'context_key']) IS DISTINCT FROM '{}'::jsonb
               OR (
                    NEW.config -> 'enabled' = 'true'::jsonb
                    AND (
                        jsonb_typeof(NEW.config -> 'context_key') IS DISTINCT FROM 'string'
                        OR NEW.config ->> 'context_key' = ''
                    )
               )
               OR (
                    NEW.config -> 'enabled' = 'false'::jsonb
                    AND jsonb_typeof(NEW.config -> 'context_key') NOT IN ('null', 'string')
               ) THEN
                RAISE EXCEPTION 'Invalid THRESHOLD configuration';
            END IF;
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

package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEnvironmentNotFound = errors.New("environment not found")
	ErrFeatureNotFound     = errors.New("feature not found")
	ErrConfigNotFound      = errors.New("environment configuration not found")
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) EnvironmentExists(ctx context.Context, id string) error {
	var exists bool
	if err := r.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM environments WHERE id = $1 AND archived_at IS NULL)
	`, id).Scan(&exists); err != nil {
		return fmt.Errorf("check environment: %w", err)
	}
	if !exists {
		return ErrEnvironmentNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, environmentID string, page, limit int64) ([]Configuration, int64, error) {
	var total int64
	const filter = `FROM environment_feature_configs c
		JOIN feature_definitions f ON f.id = c.feature_id
		WHERE c.environment_id = $1 AND f.archived_at IS NULL`
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) `+filter, environmentID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count environment configurations: %w", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT f.key, f.type, c.config, c.version, c.updated_at
		`+filter+`
		ORDER BY f.key
		LIMIT $2 OFFSET $3
	`, environmentID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list environment configurations: %w", err)
	}
	defer rows.Close()
	items := make([]Configuration, 0)
	for rows.Next() {
		item, err := scanConfiguration(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan environment configuration: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate environment configurations: %w", err)
	}
	return items, total, nil
}

func (r *Repository) Find(ctx context.Context, environmentID, featureKey string) (Configuration, error) {
	var item Configuration
	var version int64
	err := r.db.QueryRow(ctx, `
		SELECT f.key, f.type, COALESCE(c.config, 'null'::jsonb), COALESCE(c.version, 0), COALESCE(c.updated_at, 'epoch'::timestamptz)
		FROM feature_definitions f
		LEFT JOIN environment_feature_configs c ON c.feature_id = f.id AND c.environment_id = $1
		WHERE f.key = $2 AND f.archived_at IS NULL
	`, environmentID, featureKey).Scan(&item.FeatureKey, &item.Type, &item.Config, &version, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Configuration{}, ErrFeatureNotFound
	}
	if err != nil {
		return Configuration{}, fmt.Errorf("find feature configuration: %w", err)
	}
	if version == 0 {
		return Configuration{}, ErrConfigNotFound
	}
	item.Version = version
	return item, nil
}

func (r *Repository) Begin(ctx context.Context) (UpdateTx, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin configuration update: %w", err)
	}
	return &configurationTx{tx: tx}, nil
}

type UpdateTx interface {
	EnvironmentExists(context.Context, string) error
	FindFeature(context.Context, string) (string, string, error)
	LockConfiguration(context.Context, string, string) (json.RawMessage, error)
	UpdateConfiguration(context.Context, string, string, json.RawMessage) (int64, time.Time, error)
	InsertAudit(context.Context, string, string, json.RawMessage, json.RawMessage) error
	Commit(context.Context) error
	Rollback(context.Context) error
}

type configurationTx struct{ tx pgx.Tx }

func (t *configurationTx) EnvironmentExists(ctx context.Context, id string) error {
	var found string
	err := t.tx.QueryRow(ctx, `SELECT id FROM environments WHERE id = $1 AND archived_at IS NULL FOR SHARE`, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEnvironmentNotFound
	}
	if err != nil {
		return fmt.Errorf("find active environment: %w", err)
	}
	return nil
}

func (t *configurationTx) FindFeature(ctx context.Context, key string) (string, string, error) {
	var id, featureType string
	err := t.tx.QueryRow(ctx, `
		SELECT id, type FROM feature_definitions
		WHERE key = $1 AND archived_at IS NULL
		FOR SHARE
	`, key).Scan(&id, &featureType)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrFeatureNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("find active feature: %w", err)
	}
	return id, featureType, nil
}

func (t *configurationTx) LockConfiguration(ctx context.Context, environmentID, featureID string) (json.RawMessage, error) {
	var oldConfig json.RawMessage
	err := t.tx.QueryRow(ctx, `
		SELECT config FROM environment_feature_configs
		WHERE environment_id = $1 AND feature_id = $2
		FOR UPDATE
	`, environmentID, featureID).Scan(&oldConfig)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrConfigNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock environment configuration: %w", err)
	}
	return oldConfig, nil
}

func (t *configurationTx) UpdateConfiguration(ctx context.Context, environmentID, featureID string, config json.RawMessage) (int64, time.Time, error) {
	var version int64
	var updatedAt time.Time
	err := t.tx.QueryRow(ctx, `
		UPDATE environment_feature_configs
		SET config = $3::jsonb, version = version + 1, updated_at = CURRENT_TIMESTAMP
		WHERE environment_id = $1 AND feature_id = $2
		RETURNING version, updated_at
	`, environmentID, featureID, string(config)).Scan(&version, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, time.Time{}, ErrConfigNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && ((pgErr.Code == "P0001" && isConfigValidationError(pgErr.Message)) || pgErr.Code == "22P02" || pgErr.Code == "22003") {
			return 0, time.Time{}, ErrInvalidConfig
		}
		return 0, time.Time{}, fmt.Errorf("update environment configuration: %w", err)
	}
	return version, updatedAt, nil
}

func isConfigValidationError(message string) bool {
	switch message {
	case "Invalid BOOLEAN configuration", "Invalid NUMBER configuration", "Invalid THRESHOLD configuration", "Invalid PERCENTAGE configuration":
		return true
	default:
		return false
	}
}

func (t *configurationTx) InsertAudit(ctx context.Context, environmentID, featureID string, oldValue, newValue json.RawMessage) error {
	_, err := t.tx.Exec(ctx, `
		INSERT INTO audit_logs (environment_id, feature_id, action, old_value, new_value)
		VALUES ($1, $2, 'CONFIG_UPDATE', $3::jsonb, $4::jsonb)
	`, environmentID, featureID, string(oldValue), string(newValue))
	if err != nil {
		return fmt.Errorf("insert configuration audit log: %w", err)
	}
	return nil
}

func (t *configurationTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *configurationTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

func scanConfiguration(row pgx.Row) (Configuration, error) {
	var item Configuration
	err := row.Scan(&item.FeatureKey, &item.Type, &item.Config, &item.Version, &item.UpdatedAt)
	return item, err
}

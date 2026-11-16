package feature

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound          = errors.New("feature not found")
	ErrDuplicateKey      = errors.New("feature key already exists")
	ErrEnvironmentsState = errors.New("all nine expected environments must exist and be active")
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Begin(ctx context.Context) (FeatureTx, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin feature transaction: %w", err)
	}
	return &featureTx{tx: tx}, nil
}

func (r *Repository) List(ctx context.Context, page, limit int64, featureType string) ([]Feature, int64, error) {
	var total int64
	if err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM feature_definitions
		WHERE archived_at IS NULL AND ($1 = '' OR type = $1)
	`, featureType).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count features: %w", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, key, name, type, description, created_at, archived_at
		FROM feature_definitions
		WHERE archived_at IS NULL AND ($1 = '' OR type = $1)
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, featureType, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list features: %w", err)
	}
	defer rows.Close()
	items := make([]Feature, 0)
	for rows.Next() {
		item, err := scanFeature(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan feature: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate features: %w", err)
	}
	return items, total, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (Feature, error) {
	item, err := scanFeature(r.db.QueryRow(ctx, `
		SELECT id, key, name, type, description, created_at, archived_at
		FROM feature_definitions WHERE id = $1 AND archived_at IS NULL
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Feature{}, ErrNotFound
	}
	if err != nil {
		return Feature{}, fmt.Errorf("find feature: %w", err)
	}
	return item, nil
}

func (r *Repository) Update(ctx context.Context, id string, name string, nameSet bool, description *string, descriptionSet bool) (Feature, error) {
	item, err := scanFeature(r.db.QueryRow(ctx, `
		UPDATE feature_definitions
		SET name = CASE WHEN $2 THEN $3 ELSE name END,
		    description = CASE WHEN $4 THEN $5::text ELSE description END
		WHERE id = $1 AND archived_at IS NULL
		RETURNING id, key, name, type, description, created_at, archived_at
	`, id, nameSet, name, descriptionSet, description))
	if errors.Is(err, pgx.ErrNoRows) {
		return Feature{}, ErrNotFound
	}
	if err != nil {
		return Feature{}, fmt.Errorf("update feature: %w", err)
	}
	return item, nil
}

func (r *Repository) Archive(ctx context.Context, id string) error {
	var archivedID string
	err := r.db.QueryRow(ctx, `
		UPDATE feature_definitions SET archived_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND archived_at IS NULL
		RETURNING id
	`, id).Scan(&archivedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("archive feature: %w", err)
	}
	return nil
}

func scanFeature(row pgx.Row) (Feature, error) {
	var item Feature
	err := row.Scan(&item.ID, &item.Key, &item.Name, &item.Type, &item.Description, &item.CreatedAt, &item.ArchivedAt)
	return item, err
}

type featureTx struct{ tx pgx.Tx }

func (t *featureTx) ActiveEnvironments(ctx context.Context) ([]EnvironmentRef, error) {
	rows, err := t.tx.Query(ctx, `
		SELECT id, platform, type
		FROM environments
		WHERE archived_at IS NULL
		  AND (platform, type) IN (
			('web', 'development'), ('web', 'staging'), ('web', 'production'),
			('android', 'development'), ('android', 'staging'), ('android', 'production'),
			('ios', 'development'), ('ios', 'staging'), ('ios', 'production')
		  )
		ORDER BY platform, CASE type WHEN 'development' THEN 1 WHEN 'staging' THEN 2 WHEN 'production' THEN 3 END
		FOR SHARE
	`)
	if err != nil {
		return nil, fmt.Errorf("list active environments: %w", err)
	}
	defer rows.Close()
	items := make([]EnvironmentRef, 0, 9)
	for rows.Next() {
		var item EnvironmentRef
		if err := rows.Scan(&item.ID, &item.Platform, &item.Type); err != nil {
			return nil, fmt.Errorf("scan active environment: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active environments: %w", err)
	}
	return items, nil
}

func (t *featureTx) InsertFeature(ctx context.Context, key, name, featureType string, description *string) (Feature, error) {
	item, err := scanFeature(t.tx.QueryRow(ctx, `
		INSERT INTO feature_definitions (key, name, type, description)
		VALUES ($1, $2, $3, $4)
		RETURNING id, key, name, type, description, created_at, archived_at
	`, key, name, featureType, description))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "feature_definitions_key_key" {
			return Feature{}, ErrDuplicateKey
		}
		return Feature{}, fmt.Errorf("insert feature: %w", err)
	}
	return item, nil
}

func (t *featureTx) InsertDefaultConfigurations(ctx context.Context, featureID string, config []byte, environments []EnvironmentRef) error {
	for _, environment := range environments {
		_, err := t.tx.Exec(ctx, `
			INSERT INTO environment_feature_configs (environment_id, feature_id, config)
			VALUES ($1, $2, $3::jsonb)
		`, environment.ID, featureID, string(config))
		if err != nil {
			return fmt.Errorf("insert default configuration for environment %s: %w", environment.ID, err)
		}
	}
	return nil
}

func (t *featureTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *featureTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

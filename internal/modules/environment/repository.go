package environment

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("environment not found")
var ErrDuplicateName = errors.New("environment name already exists")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context) ([]Environment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, platform, type, created_at, archived_at
		FROM environments
		WHERE archived_at IS NULL
		ORDER BY platform, CASE type
			WHEN 'development' THEN 1
			WHEN 'staging' THEN 2
			WHEN 'production' THEN 3
		END
	`)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	defer rows.Close()

	environments := make([]Environment, 0)
	for rows.Next() {
		var environment Environment
		if err := rows.Scan(
			&environment.ID,
			&environment.Name,
			&environment.Platform,
			&environment.Type,
			&environment.CreatedAt,
			&environment.ArchivedAt,
		); err != nil {
			return nil, fmt.Errorf("scan environment: %w", err)
		}
		environments = append(environments, environment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate environments: %w", err)
	}
	return environments, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (Environment, error) {
	var environment Environment
	err := r.db.QueryRow(ctx, `
		SELECT id, name, platform, type, created_at, archived_at
		FROM environments
		WHERE id = $1 AND archived_at IS NULL
	`, id).Scan(
		&environment.ID,
		&environment.Name,
		&environment.Platform,
		&environment.Type,
		&environment.CreatedAt,
		&environment.ArchivedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Environment{}, ErrNotFound
	}
	if err != nil {
		return Environment{}, fmt.Errorf("find environment by ID: %w", err)
	}
	return environment, nil
}

func (r *Repository) UpdateName(ctx context.Context, id, name string) (Environment, error) {
	var environment Environment
	err := r.db.QueryRow(ctx, `
		UPDATE environments SET name = $2
		WHERE id = $1 AND archived_at IS NULL
		RETURNING id, name, platform, type, created_at, archived_at
	`, id, name).Scan(&environment.ID, &environment.Name, &environment.Platform, &environment.Type, &environment.CreatedAt, &environment.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Environment{}, ErrNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Environment{}, ErrDuplicateName
		}
		return Environment{}, fmt.Errorf("update environment name: %w", err)
	}
	return environment, nil
}

func (r *Repository) Archive(ctx context.Context, id string) error {
	// The first UPDATE preserves an existing archive timestamp. A follow-up
	// existence check distinguishes an already archived row from an unknown ID.
	_, err := r.db.Exec(ctx, `UPDATE environments SET archived_at = CURRENT_TIMESTAMP WHERE id = $1 AND archived_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("archive environment: %w", err)
	}
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM environments WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("check environment for archive: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

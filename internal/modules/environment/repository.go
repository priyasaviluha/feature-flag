package environment

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("environment not found")

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
		WHERE id = $1
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

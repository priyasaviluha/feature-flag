package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAdminNotFound = errors.New("admin not found")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) FindByEmail(ctx context.Context, email string) (Admin, error) {
	var admin Admin
	err := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, created_at
		FROM admin
		WHERE LOWER(email) = LOWER($1)
	`, email).Scan(&admin.ID, &admin.Email, &admin.PasswordHash, &admin.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrAdminNotFound
	}
	if err != nil {
		return Admin{}, fmt.Errorf("find admin by email: %w", err)
	}
	return admin, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (Admin, error) {
	var admin Admin
	err := r.db.QueryRow(ctx, `
		SELECT id, email, created_at
		FROM admin
		WHERE id = $1
	`, id).Scan(&admin.ID, &admin.Email, &admin.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrAdminNotFound
	}
	if err != nil {
		return Admin{}, fmt.Errorf("find admin by ID: %w", err)
	}
	return admin, nil
}

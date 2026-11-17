package apikey

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

type AuthenticatedKey struct {
	ID            string
	EnvironmentID string
	Environment   string
}

func (r *Repository) Authenticate(ctx context.Context, hash string) (AuthenticatedKey, string, error) {
	var key AuthenticatedKey
	var status string
	var expiresAt *time.Time
	var archivedAt *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT k.id, k.environment_id, e.name, k.status, k.expires_at, e.archived_at
		FROM api_keys k JOIN environments e ON e.id = k.environment_id
		WHERE k.key_hash = $1
	`, hash).Scan(&key.ID, &key.EnvironmentID, &key.Environment, &status, &expiresAt, &archivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthenticatedKey{}, "", ErrAPIKeyNotFound
	}
	if err != nil {
		return AuthenticatedKey{}, "", fmt.Errorf("authenticate API key: %w", err)
	}
	if status != "ACTIVE" || (expiresAt != nil && !expiresAt.After(time.Now())) || archivedAt != nil {
		return AuthenticatedKey{}, status, ErrAPIKeyInactive
	}
	if _, err := r.db.Exec(ctx, `UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = $1`, key.ID); err != nil {
		return AuthenticatedKey{}, "", fmt.Errorf("update API key usage: %w", err)
	}
	return key, status, nil
}

func (r *Repository) Create(ctx context.Context, name, environmentID, prefix, hash, adminID, raw string) (CreatedAPIKey, error) {
	var result CreatedAPIKey
	err := r.db.QueryRow(ctx, `
		INSERT INTO api_keys (name, key_prefix, key_hash, environment_id, created_by)
		SELECT $1, $2, $3, e.id, $5 FROM environments e
		WHERE e.id = $4 AND e.archived_at IS NULL
		RETURNING id, name, environment_id, (SELECT name FROM environments WHERE id = environment_id), status, created_at
	`, name, prefix, hash, environmentID, adminID).Scan(&result.ID, &result.Name, &result.EnvironmentID, &result.Environment, &result.Status, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreatedAPIKey{}, ErrEnvironmentNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return CreatedAPIKey{}, ErrHashCollision
		}
		return CreatedAPIKey{}, fmt.Errorf("create API key: %w", err)
	}
	result.APIKey = raw
	return result, nil
}

func (r *Repository) List(ctx context.Context, environmentID string, page, limit int64) ([]APIKey, int64, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM environments WHERE id = $1 AND archived_at IS NULL)`, environmentID).Scan(&exists); err != nil {
		return nil, 0, fmt.Errorf("check environment: %w", err)
	}
	if !exists {
		return nil, 0, ErrEnvironmentNotFound
	}
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM api_keys WHERE environment_id = $1`, environmentID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count API keys: %w", err)
	}
	rows, err := r.db.Query(ctx, `
		SELECT k.id, k.name, k.key_prefix, k.environment_id, e.name, k.status,
			k.created_at, k.revoked_at, k.last_used_at, k.expires_at
		FROM api_keys k JOIN environments e ON e.id = k.environment_id
		WHERE k.environment_id = $1 ORDER BY k.created_at DESC, k.id
		LIMIT $2 OFFSET $3
	`, environmentID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list API keys: %w", err)
	}
	defer rows.Close()
	items := make([]APIKey, 0)
	for rows.Next() {
		var item APIKey
		if err := rows.Scan(&item.ID, &item.Name, &item.KeyPrefix, &item.EnvironmentID, &item.Environment, &item.Status, &item.CreatedAt, &item.RevokedAt, &item.LastUsedAt, &item.ExpiresAt); err != nil {
			return nil, 0, fmt.Errorf("scan API key: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate API keys: %w", err)
	}
	return items, total, nil
}

func (r *Repository) Revoke(ctx context.Context, id, adminID string) (returnErr error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin API key revocation: %w", err)
	}
	defer func() {
		if returnErr != nil {
			_ = tx.Rollback(context.Background())
		}
	}()
	var environmentID, status string
	err = tx.QueryRow(ctx, `SELECT environment_id, status FROM api_keys WHERE id = $1 FOR UPDATE`, id).Scan(&environmentID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAPIKeyNotFound
	}
	if err != nil {
		return fmt.Errorf("find API key: %w", err)
	}
	if status == "ACTIVE" {
		if _, err := tx.Exec(ctx, `UPDATE api_keys SET status = 'REVOKED', revoked_at = CURRENT_TIMESTAMP WHERE id = $1`, id); err != nil {
			return fmt.Errorf("revoke API key: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO api_key_audit_logs (admin_id, api_key_id, environment_id, action) VALUES ($1, $2, $3, 'API_KEY_REVOKED')`, adminID, id, environmentID); err != nil {
			return fmt.Errorf("audit API key revocation: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit API key revocation: %w", err)
	}
	return nil
}

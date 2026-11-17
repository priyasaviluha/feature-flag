package audit

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("audit log not found")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, environmentID, featureID *string, page, limit int64) ([]AuditLog, int64, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, 0, fmt.Errorf("begin configuration audit list: %w", err)
	}
	defer tx.Rollback(ctx)
	var total int64
	const filter = `FROM audit_logs
		WHERE ($1::uuid IS NULL OR environment_id = $1::uuid)
		  AND ($2::uuid IS NULL OR feature_id = $2::uuid)`
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) `+filter, environmentID, featureID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count configuration audit logs: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT id, environment_id, feature_id, action, old_value, new_value, created_at
		`+filter+`
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4
	`, environmentID, featureID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list configuration audit logs: %w", err)
	}
	defer rows.Close()
	items := make([]AuditLog, 0)
	for rows.Next() {
		item, err := scanAuditLog(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan configuration audit log: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate configuration audit logs: %w", err)
	}
	return items, total, nil
}

func (r *Repository) Find(ctx context.Context, id string) (AuditLog, error) {
	item, err := scanAuditLog(r.db.QueryRow(ctx, `
		SELECT id, environment_id, feature_id, action, old_value, new_value, created_at
		FROM audit_logs WHERE id = $1
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return AuditLog{}, ErrNotFound
	}
	if err != nil {
		return AuditLog{}, fmt.Errorf("find configuration audit log: %w", err)
	}
	return item, nil
}

type rowScanner interface{ Scan(...any) error }

func scanAuditLog(row rowScanner) (AuditLog, error) {
	var item AuditLog
	err := row.Scan(&item.ID, &item.EnvironmentID, &item.FeatureID, &item.Action, &item.OldValue, &item.NewValue, &item.CreatedAt)
	return item, err
}

package usergroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"flag_backend/internal/modules/configuration"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrGroupNotFound                = errors.New("user group not found")
	ErrDuplicateName                = errors.New("user group name already exists")
	ErrGroupArchived                = errors.New("user group is archived")
	ErrAlreadyArchived              = errors.New("user group is already archived")
	ErrEnvironmentNotFound          = errors.New("environment not found")
	ErrFeatureNotFound              = errors.New("feature not found")
	ErrDefaultConfigurationNotFound = errors.New("environment configuration not found")
	ErrRuleNotFound                 = errors.New("feature group rollout rule not found")
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, name string, description *string) (Group, error) {
	group, err := scanGroup(r.db.QueryRow(ctx, `
		INSERT INTO user_groups (name, description)
		VALUES ($1, $2)
		RETURNING id, name, description, created_at, archived_at
	`, name, description))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "user_groups_name_key" {
			return Group{}, ErrDuplicateName
		}
		return Group{}, fmt.Errorf("create user group: %w", err)
	}
	return group, nil
}

func (r *Repository) List(ctx context.Context) ([]GroupSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, description
		FROM user_groups
		WHERE archived_at IS NULL
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list user groups: %w", err)
	}
	defer rows.Close()
	items := make([]GroupSummary, 0)
	for rows.Next() {
		var item GroupSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.Description); err != nil {
			return nil, fmt.Errorf("scan user group: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user groups: %w", err)
	}
	return items, nil
}

func (r *Repository) Find(ctx context.Context, id string) (Group, error) {
	group, err := scanGroup(r.db.QueryRow(ctx, `
		SELECT id, name, description, created_at, archived_at
		FROM user_groups WHERE id = $1
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrGroupNotFound
	}
	if err != nil {
		return Group{}, fmt.Errorf("find user group: %w", err)
	}
	return group, nil
}

func (r *Repository) Patch(ctx context.Context, id string, patch GroupPatch) (Group, error) {
	group, err := scanGroup(r.db.QueryRow(ctx, `
		UPDATE user_groups
		SET name = CASE WHEN $2 THEN $3 ELSE name END,
		    description = CASE WHEN $4 THEN $5::text ELSE description END
		WHERE id = $1 AND archived_at IS NULL
		RETURNING id, name, description, created_at, archived_at
	`, id, patch.NameSet, patch.Name, patch.DescriptionSet, patch.Description))
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, r.groupState(ctx, id)
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "user_groups_name_key" {
			return Group{}, ErrDuplicateName
		}
		return Group{}, fmt.Errorf("update user group: %w", err)
	}
	return group, nil
}

func (r *Repository) Archive(ctx context.Context, id string) error {
	var archivedID string
	err := r.db.QueryRow(ctx, `
		UPDATE user_groups SET archived_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND archived_at IS NULL
		RETURNING id
	`, id).Scan(&archivedID)
	if errors.Is(err, pgx.ErrNoRows) {
		stateErr := r.groupState(ctx, id)
		if errors.Is(stateErr, ErrGroupArchived) {
			return ErrAlreadyArchived
		}
		return stateErr
	}
	if err != nil {
		return fmt.Errorf("archive user group: %w", err)
	}
	return nil
}

func (r *Repository) groupState(ctx context.Context, id string) error {
	var archived bool
	err := r.db.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM user_groups WHERE id = $1`, id).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrGroupNotFound
	}
	if err != nil {
		return fmt.Errorf("check user group state: %w", err)
	}
	if archived {
		return ErrGroupArchived
	}
	return ErrGroupNotFound
}

func scanGroup(row pgx.Row) (Group, error) {
	var group Group
	err := row.Scan(&group.ID, &group.Name, &group.Description, &group.CreatedAt, &group.ArchivedAt)
	return group, err
}

func (r *Repository) Effective(ctx context.Context, environmentID, featureID string) (EnvironmentFeatureGroups, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return EnvironmentFeatureGroups{}, fmt.Errorf("begin effective group configuration snapshot: %w", err)
	}
	defer tx.Rollback(ctx)

	var environmentName, featureKey *string
	var defaultConfig []byte
	var defaultVersion *int64
	err = tx.QueryRow(ctx, `
		SELECT
			(SELECT name FROM environments WHERE id = $1 AND archived_at IS NULL),
			(SELECT key FROM feature_definitions WHERE id = $2 AND archived_at IS NULL),
			c.config,
			c.version
		FROM (SELECT 1) AS singleton
		LEFT JOIN environment_feature_configs c
			ON c.environment_id = $1 AND c.feature_id = $2
	`, environmentID, featureID).Scan(&environmentName, &featureKey, &defaultConfig, &defaultVersion)
	if err != nil {
		return EnvironmentFeatureGroups{}, fmt.Errorf("load group rollout context: %w", err)
	}
	if environmentName == nil {
		return EnvironmentFeatureGroups{}, ErrEnvironmentNotFound
	}
	if featureKey == nil {
		return EnvironmentFeatureGroups{}, ErrFeatureNotFound
	}
	if defaultConfig == nil || defaultVersion == nil {
		return EnvironmentFeatureGroups{}, ErrDefaultConfigurationNotFound
	}

	rows, err := tx.Query(ctx, `
		SELECT g.id, g.name,
		       COALESCE(rule.config, $3::jsonb),
		       COALESCE(rule.version, $4::bigint),
		       rule.id IS NOT NULL
		FROM user_groups g
		LEFT JOIN feature_group_rules rule
		  ON rule.environment_id = $1 AND rule.feature_id = $2 AND rule.group_id = g.id
		WHERE g.archived_at IS NULL
		ORDER BY g.created_at DESC, g.id DESC
	`, environmentID, featureID, string(defaultConfig), *defaultVersion)
	if err != nil {
		return EnvironmentFeatureGroups{}, fmt.Errorf("list effective group configurations: %w", err)
	}
	defer rows.Close()
	result := EnvironmentFeatureGroups{FeatureKey: *featureKey, Environment: *environmentName, Groups: make([]EffectiveConfig, 0)}
	for rows.Next() {
		var item EffectiveConfig
		if err := rows.Scan(&item.GroupID, &item.GroupName, &item.Config, &item.Version, &item.IsOverride); err != nil {
			return EnvironmentFeatureGroups{}, fmt.Errorf("scan effective group configuration: %w", err)
		}
		result.Groups = append(result.Groups, item)
	}
	if err := rows.Err(); err != nil {
		return EnvironmentFeatureGroups{}, fmt.Errorf("iterate effective group configurations: %w", err)
	}
	return result, nil
}

func (r *Repository) BeginRollout(ctx context.Context) (RolloutTx, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin feature group rollout: %w", err)
	}
	return &rolloutTx{tx: tx}, nil
}

type RolloutTx interface {
	FindEnvironment(context.Context, string) (EnvironmentIdentity, error)
	FindFeature(context.Context, string) (FeatureIdentity, error)
	FindActiveGroup(context.Context, string) (GroupIdentity, error)
	FindGroup(context.Context, string) (GroupIdentity, error)
	UpsertRule(context.Context, string, string, string, json.RawMessage) (RuleVersion, error)
	DeleteRule(context.Context, string, string, string) error
	Commit(context.Context) error
	Rollback(context.Context) error
}

type rolloutTx struct{ tx pgx.Tx }

func (t *rolloutTx) FindEnvironment(ctx context.Context, id string) (EnvironmentIdentity, error) {
	var item EnvironmentIdentity
	err := t.tx.QueryRow(ctx, `
		SELECT id, name FROM environments WHERE id = $1 AND archived_at IS NULL FOR SHARE
	`, id).Scan(&item.ID, &item.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return EnvironmentIdentity{}, ErrEnvironmentNotFound
	}
	if err != nil {
		return EnvironmentIdentity{}, fmt.Errorf("find active environment: %w", err)
	}
	return item, nil
}

func (t *rolloutTx) FindFeature(ctx context.Context, id string) (FeatureIdentity, error) {
	var item FeatureIdentity
	err := t.tx.QueryRow(ctx, `
		SELECT id, key, type FROM feature_definitions WHERE id = $1 AND archived_at IS NULL FOR SHARE
	`, id).Scan(&item.ID, &item.Key, &item.Type)
	if errors.Is(err, pgx.ErrNoRows) {
		return FeatureIdentity{}, ErrFeatureNotFound
	}
	if err != nil {
		return FeatureIdentity{}, fmt.Errorf("find active feature: %w", err)
	}
	return item, nil
}

func (t *rolloutTx) FindActiveGroup(ctx context.Context, id string) (GroupIdentity, error) {
	var item GroupIdentity
	err := t.tx.QueryRow(ctx, `
		SELECT id, name FROM user_groups WHERE id = $1 AND archived_at IS NULL FOR SHARE
	`, id).Scan(&item.ID, &item.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return GroupIdentity{}, t.groupState(ctx, id)
	}
	if err != nil {
		return GroupIdentity{}, fmt.Errorf("find active user group: %w", err)
	}
	return item, nil
}

func (t *rolloutTx) FindGroup(ctx context.Context, id string) (GroupIdentity, error) {
	var item GroupIdentity
	err := t.tx.QueryRow(ctx, `SELECT id, name FROM user_groups WHERE id = $1 FOR SHARE`, id).Scan(&item.ID, &item.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return GroupIdentity{}, ErrGroupNotFound
	}
	if err != nil {
		return GroupIdentity{}, fmt.Errorf("find user group: %w", err)
	}
	return item, nil
}

func (t *rolloutTx) groupState(ctx context.Context, id string) error {
	var archived bool
	err := t.tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM user_groups WHERE id = $1`, id).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrGroupNotFound
	}
	if err != nil {
		return fmt.Errorf("check user group state: %w", err)
	}
	if archived {
		return ErrGroupArchived
	}
	return ErrGroupNotFound
}

func (t *rolloutTx) UpsertRule(ctx context.Context, environmentID, featureID, groupID string, config json.RawMessage) (RuleVersion, error) {
	var result RuleVersion
	err := t.tx.QueryRow(ctx, `
		INSERT INTO feature_group_rules (environment_id, feature_id, group_id, config, version)
		VALUES ($1, $2, $3, $4::jsonb, 1)
		ON CONFLICT (environment_id, feature_id, group_id) DO UPDATE
		SET config = EXCLUDED.config,
		    version = feature_group_rules.version + 1,
		    updated_at = CURRENT_TIMESTAMP
		RETURNING version, updated_at
	`, environmentID, featureID, groupID, string(config)).Scan(&result.Version, &result.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "P0001" || pgErr.Code == "22P02" || pgErr.Code == "22003") {
			return RuleVersion{}, configuration.ErrInvalidConfig
		}
		return RuleVersion{}, fmt.Errorf("upsert feature group rollout rule: %w", err)
	}
	return result, nil
}

func (t *rolloutTx) DeleteRule(ctx context.Context, environmentID, featureID, groupID string) error {
	result, err := t.tx.Exec(ctx, `
		DELETE FROM feature_group_rules
		WHERE environment_id = $1 AND feature_id = $2 AND group_id = $3
	`, environmentID, featureID, groupID)
	if err != nil {
		return fmt.Errorf("delete feature group rollout rule: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrRuleNotFound
	}
	return nil
}

func (t *rolloutTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *rolloutTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

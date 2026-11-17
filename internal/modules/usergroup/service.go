package usergroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"flag_backend/internal/modules/configuration"
)

var (
	ErrInvalidName = errors.New("group name must not be empty and must be at most 100 characters")
	ErrEmptyPatch  = errors.New("patch must include name or description")
)

type Store interface {
	Create(context.Context, string, *string) (Group, error)
	List(context.Context) ([]GroupSummary, error)
	Find(context.Context, string) (Group, error)
	Patch(context.Context, string, GroupPatch) (Group, error)
	Archive(context.Context, string) error
	Effective(context.Context, string, string) (EnvironmentFeatureGroups, error)
	BeginRollout(context.Context) (RolloutTx, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Create(ctx context.Context, name string, description *string) (Group, error) {
	name = strings.TrimSpace(name)
	if !validName(name) {
		return Group{}, ErrInvalidName
	}
	return s.store.Create(ctx, name, description)
}

func (s *Service) List(ctx context.Context) ([]GroupSummary, error) { return s.store.List(ctx) }

func (s *Service) Find(ctx context.Context, id string) (Group, error) { return s.store.Find(ctx, id) }

func (s *Service) Patch(ctx context.Context, id string, patch GroupPatch) (Group, error) {
	if !patch.NameSet && !patch.DescriptionSet {
		return Group{}, ErrEmptyPatch
	}
	if patch.NameSet {
		patch.Name = strings.TrimSpace(patch.Name)
		if !validName(patch.Name) {
			return Group{}, ErrInvalidName
		}
	}
	return s.store.Patch(ctx, id, patch)
}

func (s *Service) Archive(ctx context.Context, id string) error { return s.store.Archive(ctx, id) }

func (s *Service) Effective(ctx context.Context, environmentID, featureID string) (EnvironmentFeatureGroups, error) {
	result, err := s.store.Effective(ctx, environmentID, featureID)
	if err != nil {
		return EnvironmentFeatureGroups{}, err
	}
	if result.Groups == nil {
		result.Groups = make([]EffectiveConfig, 0)
	}
	return result, nil
}

func (s *Service) SetRule(ctx context.Context, environmentID, featureID, groupID string, config json.RawMessage) (result GroupRule, returnErr error) {
	tx, err := s.store.BeginRollout(ctx)
	if err != nil {
		return GroupRule{}, err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(context.Background()); rollbackErr != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("rollback group rollout update: %w", rollbackErr))
			}
		}
	}()

	environment, err := tx.FindEnvironment(ctx, environmentID)
	if err != nil {
		return GroupRule{}, err
	}
	feature, err := tx.FindFeature(ctx, featureID)
	if err != nil {
		return GroupRule{}, err
	}
	group, err := tx.FindActiveGroup(ctx, groupID)
	if err != nil {
		return GroupRule{}, err
	}
	if err := configuration.ValidateConfig(feature.Type, config); err != nil {
		return GroupRule{}, configuration.ErrInvalidConfig
	}
	version, err := tx.UpsertRule(ctx, environmentID, featureID, groupID, config)
	if err != nil {
		return GroupRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GroupRule{}, fmt.Errorf("commit group rollout update: %w", err)
	}
	committed = true
	return GroupRule{
		FeatureKey:  feature.Key,
		Environment: environment.Name,
		Group:       group,
		Config:      config,
		Version:     version.Version,
		UpdatedAt:   version.UpdatedAt,
	}, nil
}

func (s *Service) DeleteRule(ctx context.Context, environmentID, featureID, groupID string) (returnErr error) {
	tx, err := s.store.BeginRollout(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(context.Background()); rollbackErr != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("rollback group rollout deletion: %w", rollbackErr))
			}
		}
	}()

	if _, err := tx.FindEnvironment(ctx, environmentID); err != nil {
		return err
	}
	if _, err := tx.FindFeature(ctx, featureID); err != nil {
		return err
	}
	if _, err := tx.FindGroup(ctx, groupID); err != nil {
		return err
	}
	if err := tx.DeleteRule(ctx, environmentID, featureID, groupID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit group rollout deletion: %w", err)
	}
	committed = true
	return nil
}

func validName(name string) bool { return name != "" && utf8.RuneCountInString(name) <= 100 }

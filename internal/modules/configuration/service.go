package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrInvalidPagination = errors.New("invalid pagination")
	ErrInvalidConfig     = errors.New("configuration does not match feature type")
)

const MaxLimit int64 = 100

type Store interface {
	EnvironmentExists(context.Context, string) error
	List(context.Context, string, int64, int64) ([]Configuration, int64, error)
	Find(context.Context, string, string) (Configuration, error)
	Begin(context.Context) (UpdateTx, error)
}

type Service struct{ repository Store }

func NewService(repository Store) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, environmentID string, page, limit int64) (ListResult, error) {
	if !validPagination(page, limit) {
		return ListResult{}, ErrInvalidPagination
	}
	if err := s.repository.EnvironmentExists(ctx, environmentID); err != nil {
		return ListResult{}, err
	}
	items, total, err := s.repository.List(ctx, environmentID, page, limit)
	if err != nil {
		return ListResult{}, err
	}
	if items == nil {
		items = make([]Configuration, 0)
	}
	return ListResult{Data: items, Pagination: Pagination{Page: page, Limit: limit, Total: total}}, nil
}

func (s *Service) Find(ctx context.Context, environmentID, featureKey string) (Configuration, error) {
	if err := s.repository.EnvironmentExists(ctx, environmentID); err != nil {
		return Configuration{}, err
	}
	return s.repository.Find(ctx, environmentID, featureKey)
}

func (s *Service) Update(ctx context.Context, environmentID, featureKey string, config json.RawMessage) (result Configuration, returnErr error) {
	tx, err := s.repository.Begin(ctx)
	if err != nil {
		return Configuration{}, err
	}
	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(context.Background()); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("rollback configuration update: %w", err))
			}
		}
	}()

	if err := tx.EnvironmentExists(ctx, environmentID); err != nil {
		return Configuration{}, err
	}
	featureID, featureType, err := tx.FindFeature(ctx, featureKey)
	if err != nil {
		return Configuration{}, err
	}
	oldConfig, err := tx.LockConfiguration(ctx, environmentID, featureID)
	if err != nil {
		return Configuration{}, err
	}
	version, updatedAt, err := tx.UpdateConfiguration(ctx, environmentID, featureID, config)
	if err != nil {
		return Configuration{}, err
	}
	if err := tx.InsertAudit(ctx, environmentID, featureID, oldConfig, config); err != nil {
		return Configuration{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Configuration{}, fmt.Errorf("commit configuration update: %w", err)
	}
	committed = true
	return Configuration{FeatureKey: featureKey, Type: featureType, Config: config, Version: version, UpdatedAt: updatedAt}, nil
}

func validPagination(page, limit int64) bool {
	return page > 0 && limit > 0 && limit <= MaxLimit && page-1 <= (int64(^uint64(0)>>1)-limit)/limit
}

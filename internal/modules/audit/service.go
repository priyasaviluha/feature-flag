package audit

import (
	"context"
	"errors"
)

var ErrInvalidPagination = errors.New("invalid pagination")

const MaxLimit int64 = 100

type Store interface {
	List(context.Context, *string, *string, int64, int64) ([]AuditLog, int64, error)
	Find(context.Context, string) (AuditLog, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) List(ctx context.Context, environmentID, featureID *string, page, limit int64) (ListResult, error) {
	if page < 1 || limit < 1 || limit > MaxLimit || page-1 > (int64(^uint64(0)>>1)-limit)/limit {
		return ListResult{}, ErrInvalidPagination
	}
	items, total, err := s.store.List(ctx, environmentID, featureID, page, limit)
	if err != nil {
		return ListResult{}, err
	}
	if items == nil {
		items = make([]AuditLog, 0)
	}
	return ListResult{Data: items, Pagination: Pagination{Page: page, Limit: limit, Total: total}}, nil
}

func (s *Service) Find(ctx context.Context, id string) (AuditLog, error) {
	return s.store.Find(ctx, id)
}

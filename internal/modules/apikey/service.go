package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrEnvironmentNotFound = errors.New("environment not found")
	ErrAPIKeyNotFound      = errors.New("api key not found")
	ErrAPIKeyInactive      = errors.New("api key inactive")
	ErrInvalidPagination   = errors.New("invalid pagination")
	ErrInvalidInput        = errors.New("invalid input")
	ErrHashCollision       = errors.New("API key hash collision")
)

const MaxLimit int64 = 100

type Store interface {
	Create(context.Context, string, string, string, string, string, string) (CreatedAPIKey, error)
	List(context.Context, string, int64, int64) ([]APIKey, int64, error)
	Revoke(context.Context, string, string) error
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Create(ctx context.Context, name, environmentID, adminID string) (CreatedAPIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return CreatedAPIKey{}, ErrInvalidInput
	}
	for attempt := 0; attempt < 3; attempt++ {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return CreatedAPIKey{}, fmt.Errorf("generate API key: %w", err)
		}
		raw := "ff_live_" + hex.EncodeToString(secret)
		result, err := s.store.Create(ctx, name, environmentID, "ff_live", HashKey(raw), adminID, raw)
		if !errors.Is(err, ErrHashCollision) {
			return result, err
		}
	}
	return CreatedAPIKey{}, fmt.Errorf("generate unique API key: %w", ErrHashCollision)
}

// HashKey returns the SHA-256 hex digest used to persist API key secrets.
func HashKey(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func (s *Service) List(ctx context.Context, environmentID string, page, limit int64) (ListResult, error) {
	if page < 1 || limit < 1 || limit > MaxLimit || page-1 > (int64(^uint64(0)>>1)-limit)/limit {
		return ListResult{}, ErrInvalidPagination
	}
	items, total, err := s.store.List(ctx, environmentID, page, limit)
	if err != nil {
		return ListResult{}, err
	}
	if items == nil {
		items = []APIKey{}
	}
	pages := total / limit
	if total%limit != 0 {
		pages++
	}
	return ListResult{Data: items, Pagination: Pagination{Page: page, Limit: limit, Total: total, TotalPages: pages}}, nil
}

func (s *Service) Revoke(ctx context.Context, id, adminID string) error {
	return s.store.Revoke(ctx, id, adminID)
}

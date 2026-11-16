package apikey

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

type memoryStore struct {
	name, environmentID, prefix, hash, adminID, raw string
	items                                           []APIKey
	total                                           int64
	revokedID, revokedBy                            string
}

func (m *memoryStore) Create(_ context.Context, name, environmentID, prefix, hash, adminID, raw string) (CreatedAPIKey, error) {
	m.name, m.environmentID, m.prefix, m.hash, m.adminID, m.raw = name, environmentID, prefix, hash, adminID, raw
	return CreatedAPIKey{ID: "id", Name: name, EnvironmentID: environmentID, Environment: "Production", APIKey: raw, Status: "ACTIVE"}, nil
}
func (m *memoryStore) List(_ context.Context, _ string, _, _ int64) ([]APIKey, int64, error) {
	return m.items, m.total, nil
}
func (m *memoryStore) Revoke(_ context.Context, id, adminID string) error {
	m.revokedID, m.revokedBy = id, adminID
	return nil
}

func TestCreateGeneratesAndPersistsOnlyHash(t *testing.T) {
	store := &memoryStore{}
	result, err := NewService(store).Create(context.Background(), " Production ", "environment", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if result.APIKey != store.raw || len(store.raw) != len("ff_live_")+64 {
		t.Fatalf("unexpected generated key %q", result.APIKey)
	}
	if store.name != "Production" || store.prefix != "ff_live" || store.adminID != "admin" {
		t.Fatalf("unexpected stored metadata: %#v", store)
	}
	digest := sha256.Sum256([]byte(store.raw))
	if store.hash != hex.EncodeToString(digest[:]) {
		t.Fatal("store did not receive the SHA-256 hash of the complete key")
	}
	if store.hash == store.raw {
		t.Fatal("raw API key was persisted as its hash")
	}
}

func TestCreateRejectsMissingName(t *testing.T) {
	if _, err := NewService(&memoryStore{}).Create(context.Background(), "  ", "environment", "admin"); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestListPaginationAndEmptyData(t *testing.T) {
	store := &memoryStore{items: []APIKey{}, total: 0}
	result, err := NewService(store).List(context.Background(), "environment", 1, 25)
	if err != nil {
		t.Fatal(err)
	}
	if result.Data == nil || result.Pagination.TotalPages != 0 {
		t.Fatalf("unexpected empty result: %#v", result)
	}
	if _, err := NewService(store).List(context.Background(), "environment", 1, 101); err != ErrInvalidPagination {
		t.Fatalf("expected invalid pagination, got %v", err)
	}
}

func TestRevokePassesAdminIdentity(t *testing.T) {
	store := &memoryStore{}
	if err := NewService(store).Revoke(context.Background(), "key", "admin"); err != nil {
		t.Fatal(err)
	}
	if store.revokedID != "key" || store.revokedBy != "admin" {
		t.Fatalf("unexpected revoke request: %#v", store)
	}
}

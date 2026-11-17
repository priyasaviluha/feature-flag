package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu             sync.Mutex
	environmentErr error
	featureErr     error
	configExists   bool
	featureID      string
	featureType    string
	config         json.RawMessage
	version        int64
	updatedAt      time.Time
	audits         []auditEntry
	listItems      []Configuration
	listTotal      int64
	listPage       int64
	listLimit      int64
	findResult     Configuration
	findErr        error
}

type auditEntry struct{ oldValue, newValue json.RawMessage }

func (s *fakeStore) EnvironmentExists(context.Context, string) error { return s.environmentErr }
func (s *fakeStore) List(_ context.Context, _ string, page, limit int64) ([]Configuration, int64, error) {
	s.listPage, s.listLimit = page, limit
	return s.listItems, s.listTotal, nil
}
func (s *fakeStore) Find(context.Context, string, string) (Configuration, error) {
	return s.findResult, s.findErr
}
func (s *fakeStore) Begin(context.Context) (UpdateTx, error) {
	s.mu.Lock()
	return newFakeUpdateTx(s), nil
}

type fakeUpdateTx struct {
	store          *fakeStore
	closed         bool
	updateErr      error
	auditErr       error
	pendingConfig  json.RawMessage
	pendingVersion int64
	pendingAt      time.Time
	pendingAudit   *auditEntry
}

func newFakeUpdateTx(store *fakeStore) *fakeUpdateTx { return &fakeUpdateTx{store: store} }

func (t *fakeUpdateTx) EnvironmentExists(ctx context.Context, id string) error {
	return t.store.environmentErr
}
func (t *fakeUpdateTx) FindFeature(ctx context.Context, key string) (string, string, error) {
	if t.store.featureErr != nil {
		return "", "", t.store.featureErr
	}
	return t.store.featureID, t.store.featureType, nil
}
func (t *fakeUpdateTx) LockConfiguration(context.Context, string, string) (json.RawMessage, error) {
	if !t.store.configExists {
		return nil, ErrConfigNotFound
	}
	return append(json.RawMessage(nil), t.store.config...), nil
}
func (t *fakeUpdateTx) UpdateConfiguration(_ context.Context, _, _ string, config json.RawMessage) (int64, time.Time, error) {
	if t.updateErr != nil {
		return 0, time.Time{}, t.updateErr
	}
	t.pendingConfig = append(json.RawMessage(nil), config...)
	t.pendingVersion = t.store.version + 1
	t.pendingAt = time.Now().UTC()
	return t.pendingVersion, t.pendingAt, nil
}
func (t *fakeUpdateTx) InsertAudit(_ context.Context, _, _ string, oldValue, newValue json.RawMessage) error {
	if t.auditErr != nil {
		return t.auditErr
	}
	t.pendingAudit = &auditEntry{append(json.RawMessage(nil), oldValue...), append(json.RawMessage(nil), newValue...)}
	return nil
}
func (t *fakeUpdateTx) Commit(context.Context) error {
	if t.pendingConfig != nil {
		t.store.config = t.pendingConfig
		t.store.version = t.pendingVersion
		t.store.updatedAt = t.pendingAt
	}
	if t.pendingAudit != nil {
		t.store.audits = append(t.store.audits, *t.pendingAudit)
	}
	t.close()
	return nil
}
func (t *fakeUpdateTx) Rollback(context.Context) error { t.close(); return nil }
func (t *fakeUpdateTx) close() {
	if !t.closed {
		t.closed = true
		t.store.mu.Unlock()
	}
}

func TestListReturnsEmptyPaginatedResultAndChecksEnvironment(t *testing.T) {
	store := &fakeStore{}
	result, err := NewService(store).List(context.Background(), "env", 1, 25)
	if err != nil {
		t.Fatal(err)
	}
	if result.Data == nil || result.Pagination.Page != 1 || result.Pagination.Limit != 25 || result.Pagination.Total != 0 {
		t.Fatalf("result = %#v", result)
	}
	if store.listPage != 1 || store.listLimit != 25 {
		t.Fatalf("list args=%d,%d", store.listPage, store.listLimit)
	}
	store.environmentErr = ErrEnvironmentNotFound
	if _, err := NewService(store).List(context.Background(), "env", 1, 25); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("error=%v", err)
	}
}

func TestPaginationBoundsAndOverflow(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)
	for _, input := range [][2]int64{{0, 25}, {1, 0}, {1, 101}, {1 << 62, 100}} {
		if _, err := svc.List(context.Background(), "env", input[0], input[1]); !errors.Is(err, ErrInvalidPagination) {
			t.Errorf("List(%d,%d) error=%v", input[0], input[1], err)
		}
	}
}

func TestFindMapsMissingFeatureAndConfiguration(t *testing.T) {
	store := &fakeStore{findErr: ErrFeatureNotFound}
	if _, err := NewService(store).Find(context.Background(), "env", "key"); !errors.Is(err, ErrFeatureNotFound) {
		t.Fatalf("feature error=%v", err)
	}
	store.findErr = ErrConfigNotFound
	if _, err := NewService(store).Find(context.Background(), "env", "key"); !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("config error=%v", err)
	}
	store.environmentErr = ErrEnvironmentNotFound
	if _, err := NewService(store).Find(context.Background(), "env", "key"); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("environment error=%v", err)
	}
}

func TestUpdateWritesAuditAndIncrementsVersionForSupportedTypes(t *testing.T) {
	inputs := map[string]string{
		"BOOLEAN":    `{"enabled":true}`,
		"NUMBER":     `{"value":100}`,
		"THRESHOLD":  `{"operator":">=","threshold":500,"context_key":"cart_value"}`,
		"PERCENTAGE": `{"enabled":true,"percentage":25}`,
	}
	for featureType, payload := range inputs {
		store := &fakeStore{configExists: true, featureID: "feature-id", featureType: featureType, config: json.RawMessage(`{"enabled":false}`), version: 3}
		updated, err := NewService(store).Update(context.Background(), "env", "feature_key", json.RawMessage(payload))
		if err != nil {
			t.Fatalf("%s update: %v", featureType, err)
		}
		if updated.Version != 4 || updated.Type != featureType || string(updated.Config) != payload {
			t.Errorf("%s response=%#v", featureType, updated)
		}
		if len(store.audits) != 1 || string(store.audits[0].oldValue) != `{"enabled":false}` || string(store.audits[0].newValue) != payload {
			t.Errorf("%s audit=%#v", featureType, store.audits)
		}
	}
}

func TestUpdateRollsBackWhenDatabaseValidationOrAuditFails(t *testing.T) {
	for _, tc := range []struct {
		name                string
		updateErr, auditErr error
	}{
		{"trigger validation", ErrInvalidConfig, nil},
		{"audit insert", nil, errors.New("audit unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{configExists: true, featureID: "feature-id", featureType: "BOOLEAN", config: json.RawMessage(`{"enabled":false}`), version: 1}
			tx := newFakeUpdateTx(store)
			tx.updateErr, tx.auditErr = tc.updateErr, tc.auditErr
			// Override Begin to return the configured transaction while keeping its lock lifecycle.
			store.mu.Lock()
			svc := NewService(&fixedTxStore{fakeStore: store, tx: tx})
			_, err := svc.Update(context.Background(), "env", "key", json.RawMessage(`{"enabled":true}`))
			if err == nil {
				t.Fatal("expected update error")
			}
			if tx.closed != true {
				t.Fatal("transaction was not rolled back or closed")
			}
			if store.version != 1 || len(store.audits) != 0 {
				t.Fatalf("rollback left version=%d audit count=%d", store.version, len(store.audits))
			}
		})
	}
}

type fixedTxStore struct {
	*fakeStore
	tx *fakeUpdateTx
}

func (s *fixedTxStore) Begin(context.Context) (UpdateTx, error) { return s.tx, nil }

func TestConcurrentUpdatesIncrementVersionAndAuditOnceEach(t *testing.T) {
	const updates = 24
	store := &fakeStore{configExists: true, featureID: "feature-id", featureType: "BOOLEAN", config: json.RawMessage(`{"enabled":false}`), version: 1}
	service := NewService(store)
	var wg sync.WaitGroup
	errs := make(chan error, updates)
	for i := 0; i < updates; i++ {
		wg.Add(1)
		go func(enabled bool) {
			defer wg.Done()
			value := `{"enabled":false}`
			if enabled {
				value = `{"enabled":true}`
			}
			_, err := service.Update(context.Background(), "env", "key", json.RawMessage(value))
			errs <- err
		}(i%2 == 0)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if store.version != 1+updates || len(store.audits) != updates {
		t.Fatalf("version=%d audit count=%d", store.version, len(store.audits))
	}
}

package usergroup

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"flag_backend/internal/modules/configuration"
)

type fakeStore struct {
	createErr      error
	create         Group
	list           []GroupSummary
	find           Group
	findErr        error
	patch          Group
	patchErr       error
	archiveErr     error
	effective      EnvironmentFeatureGroups
	effectiveErr   error
	activeGroupErr error
	featureType    string
	version        int64
	deleteErr      error
	deleteCount    int
	mu             sync.Mutex
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		create:      Group{ID: "group-id", Name: "Dev Team"},
		find:        Group{ID: "group-id", Name: "Dev Team"},
		patch:       Group{ID: "group-id", Name: "Backend Dev Team"},
		featureType: "BOOLEAN",
	}
}

func (f *fakeStore) Create(context.Context, string, *string) (Group, error) {
	return f.create, f.createErr
}
func (f *fakeStore) List(context.Context) ([]GroupSummary, error) { return f.list, nil }
func (f *fakeStore) Find(context.Context, string) (Group, error)  { return f.find, f.findErr }
func (f *fakeStore) Patch(_ context.Context, _ string, patch GroupPatch) (Group, error) {
	f.patch = Group{ID: "group-id", Name: patch.Name, Description: patch.Description}
	return f.patch, f.patchErr
}
func (f *fakeStore) Archive(context.Context, string) error { return f.archiveErr }
func (f *fakeStore) Effective(context.Context, string, string) (EnvironmentFeatureGroups, error) {
	return f.effective, f.effectiveErr
}
func (f *fakeStore) BeginRollout(context.Context) (RolloutTx, error) {
	return &fakeRolloutTx{store: f}, nil
}

type fakeRolloutTx struct{ store *fakeStore }

func (t *fakeRolloutTx) FindEnvironment(_ context.Context, id string) (EnvironmentIdentity, error) {
	return EnvironmentIdentity{ID: id, Name: "Development"}, nil
}
func (t *fakeRolloutTx) FindFeature(_ context.Context, id string) (FeatureIdentity, error) {
	return FeatureIdentity{ID: id, Key: "new_checkout", Type: t.store.featureType}, nil
}
func (t *fakeRolloutTx) FindActiveGroup(_ context.Context, id string) (GroupIdentity, error) {
	if t.store.activeGroupErr != nil {
		return GroupIdentity{}, t.store.activeGroupErr
	}
	return GroupIdentity{ID: id, Name: "Dev Team"}, nil
}
func (t *fakeRolloutTx) FindGroup(_ context.Context, id string) (GroupIdentity, error) {
	return GroupIdentity{ID: id, Name: "Dev Team"}, nil
}
func (t *fakeRolloutTx) UpsertRule(context.Context, string, string, string, json.RawMessage) (RuleVersion, error) {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	t.store.version++
	return RuleVersion{Version: t.store.version, UpdatedAt: time.Now()}, nil
}
func (t *fakeRolloutTx) DeleteRule(context.Context, string, string, string) error {
	t.store.deleteCount++
	return t.store.deleteErr
}
func (*fakeRolloutTx) Commit(context.Context) error   { return nil }
func (*fakeRolloutTx) Rollback(context.Context) error { return nil }

func TestCreateValidatesAndTrimsName(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	if _, err := service.Create(context.Background(), "  ", nil); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Create() error = %v, want ErrInvalidName", err)
	}
	if _, err := service.Create(context.Background(), "  Dev Team  ", nil); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := service.Patch(context.Background(), "group-id", GroupPatch{}); !errors.Is(err, ErrEmptyPatch) {
		t.Fatalf("Patch() error = %v, want ErrEmptyPatch", err)
	}
	store.createErr = ErrDuplicateName
	if _, err := service.Create(context.Background(), "Dev Team", nil); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("Create() duplicate error = %v", err)
	}
}

func TestSetRuleValidatesTypesAndIncrementsVersions(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	inputs := []struct{ featureType, config string }{
		{"BOOLEAN", `{"enabled":true}`},
		{"NUMBER", `{"value":1.5}`},
		{"THRESHOLD", `{"operator":">=","threshold":10,"context_key":"cart"}`},
		{"PERCENTAGE", `{"enabled":true,"percentage":25}`},
	}
	for _, input := range inputs {
		store.featureType = input.featureType
		if _, err := service.SetRule(context.Background(), "env", "feature", "group", json.RawMessage(input.config)); err != nil {
			t.Fatalf("SetRule(%s) error = %v", input.featureType, err)
		}
	}
	store.featureType = "BOOLEAN"
	first, err := service.SetRule(context.Background(), "env", "feature", "group", json.RawMessage(`{"enabled":false}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.SetRule(context.Background(), "env", "feature", "group", json.RawMessage(`{"enabled":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != first.Version+1 {
		t.Fatalf("versions = %d, %d; want consecutive versions", first.Version, second.Version)
	}
	if _, err := service.SetRule(context.Background(), "env", "feature", "group", json.RawMessage(`{"enabled":"yes"}`)); !errors.Is(err, configuration.ErrInvalidConfig) {
		t.Fatalf("SetRule() invalid config error = %v", err)
	}
}

func TestConcurrentSetRuleCallsUseAtomicVersions(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	const count = 20
	versions := make(chan int64, count)
	errs := make(chan error, count)
	var workers sync.WaitGroup
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, err := service.SetRule(context.Background(), "env", "feature", "group", json.RawMessage(`{"enabled":true}`))
			if err != nil {
				errs <- err
				return
			}
			versions <- result.Version
		}()
	}
	workers.Wait()
	close(versions)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := make(map[int64]bool, count)
	for version := range versions {
		seen[version] = true
	}
	if len(seen) != count {
		t.Fatalf("received %d unique versions, want %d", len(seen), count)
	}
	for version := int64(1); version <= count; version++ {
		if !seen[version] {
			t.Fatalf("missing version %d", version)
		}
	}
}

func TestSetRuleAndDeleteRuleResourceErrors(t *testing.T) {
	store := newFakeStore()
	store.activeGroupErr = ErrGroupArchived
	if _, err := NewService(store).SetRule(context.Background(), "env", "feature", "group", json.RawMessage(`{"enabled":true}`)); !errors.Is(err, ErrGroupArchived) {
		t.Fatalf("SetRule() archived group error = %v", err)
	}
	store.activeGroupErr = nil
	store.deleteErr = ErrRuleNotFound
	if err := NewService(store).DeleteRule(context.Background(), "env", "feature", "group"); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("DeleteRule() error = %v, want ErrRuleNotFound", err)
	}
}

package feature

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type fakeStore struct {
	tx                   *fakeFeatureTx
	beginN               int
	listData             []Feature
	listTotal            int64
	listPage             int64
	listLimit            int64
	listType             string
	updateID             string
	updateName           string
	updateNameSet        bool
	updateDescription    *string
	updateDescriptionSet bool
	found                Feature
	findErr              error
	archiveErr           error
	archiveCalls         int
}

func (s *fakeStore) Begin(context.Context) (FeatureTx, error) { s.beginN++; return s.tx, nil }
func (s *fakeStore) List(_ context.Context, page, limit int64, featureType string) ([]Feature, int64, error) {
	s.listPage, s.listLimit, s.listType = page, limit, featureType
	return s.listData, s.listTotal, nil
}
func (s *fakeStore) FindByID(context.Context, string) (Feature, error) { return s.found, s.findErr }
func (s *fakeStore) Update(_ context.Context, id, name string, nameSet bool, description *string, descriptionSet bool) (Feature, error) {
	s.updateID, s.updateName, s.updateNameSet = id, name, nameSet
	s.updateDescription, s.updateDescriptionSet = description, descriptionSet
	return Feature{ID: id, Name: name, Description: description}, nil
}
func (s *fakeStore) Archive(context.Context, string) error { s.archiveCalls++; return s.archiveErr }

type fakeFeatureTx struct {
	environments []EnvironmentRef
	feature      Feature
	configs      [][]byte
	inserted     bool
	commitErr    error
	configErr    error
	insertErr    error
	committed    bool
	rolledBack   bool
}

func (t *fakeFeatureTx) ActiveEnvironments(context.Context) ([]EnvironmentRef, error) {
	return t.environments, nil
}
func (t *fakeFeatureTx) InsertFeature(_ context.Context, key, name, featureType string, description *string) (Feature, error) {
	if t.insertErr != nil {
		return Feature{}, t.insertErr
	}
	t.inserted = true
	t.feature = Feature{ID: "feature-id", Key: key, Name: name, Type: featureType, Description: description}
	return t.feature, nil
}
func (t *fakeFeatureTx) InsertDefaultConfigurations(_ context.Context, _ string, config []byte, environments []EnvironmentRef) error {
	for range environments {
		t.configs = append(t.configs, append([]byte(nil), config...))
	}
	return t.configErr
}
func (t *fakeFeatureTx) Commit(context.Context) error {
	if t.commitErr != nil {
		return t.commitErr
	}
	t.committed = true
	return nil
}
func (t *fakeFeatureTx) Rollback(context.Context) error { t.rolledBack = true; return nil }

func expectedEnvironments() []EnvironmentRef {
	items := make([]EnvironmentRef, 0, 9)
	for _, platform := range []string{"web", "android", "ios"} {
		for _, environmentType := range []string{"development", "staging", "production"} {
			items = append(items, EnvironmentRef{ID: platform + "-" + environmentType, Platform: platform, Type: environmentType})
		}
	}
	return items
}

func TestDefaultConfigurationShapes(t *testing.T) {
	want := map[string]string{
		TypeBoolean:    `{"enabled":false}`,
		TypeNumber:     `{"value":0}`,
		TypeThreshold:  `{"enabled":false,"operator":">=","threshold":0,"context_key":null}`,
		TypePercentage: `{"enabled":false,"percentage":0}`,
	}
	for featureType, expected := range want {
		got, err := defaultConfiguration(featureType)
		if err != nil {
			t.Fatal(err)
		}
		var actualValue, expectedValue any
		if err := json.Unmarshal(got, &actualValue); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actualValue, expectedValue) {
			t.Errorf("%s config = %s, want %s", featureType, got, expected)
		}
	}
}

func TestCreateFeatureIsAtomicAndCreatesNineConfigurations(t *testing.T) {
	tx := &fakeFeatureTx{environments: expectedEnvironments()}
	store := &fakeStore{tx: tx}
	description := "description"
	result, err := NewService(store).Create(context.Background(), CreateRequest{Key: "new_checkout", Name: "  New Checkout  ", Type: TypeThreshold, Description: &description})
	if err != nil {
		t.Fatal(err)
	}
	if !tx.committed || tx.rolledBack || !tx.inserted {
		t.Fatalf("transaction state: committed=%v rolledBack=%v inserted=%v", tx.committed, tx.rolledBack, tx.inserted)
	}
	if len(result.ConfigurationsCreated) != 9 || len(tx.configs) != 9 {
		t.Fatalf("created envs=%d configs=%d, want 9 each", len(result.ConfigurationsCreated), len(tx.configs))
	}
	for i, config := range tx.configs {
		var decoded map[string]any
		if err := json.Unmarshal(config, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["operator"] != ">=" || decoded["context_key"] != nil {
			t.Errorf("configuration %d = %s", i, config)
		}
	}
	if result.Name != "New Checkout" {
		t.Fatalf("name = %q", result.Name)
	}
}

func TestCreateRollsBackWhenEnvironmentsAreMissing(t *testing.T) {
	environments := expectedEnvironments()[:8]
	tx := &fakeFeatureTx{environments: environments}
	service := NewService(&fakeStore{tx: tx})
	_, err := service.Create(context.Background(), CreateRequest{Key: "valid_key", Name: "Valid", Type: TypeBoolean})
	if !errors.Is(err, ErrEnvironmentsState) {
		t.Fatalf("error = %v, want missing environment error", err)
	}
	if !tx.rolledBack || tx.inserted {
		t.Fatalf("rollback=%v, feature inserted=%v", tx.rolledBack, tx.inserted)
	}
}

func TestCreateRollsBackWhenConfigurationInsertFails(t *testing.T) {
	tx := &fakeFeatureTx{environments: expectedEnvironments(), configErr: errors.New("injected configuration failure")}
	_, err := NewService(&fakeStore{tx: tx}).Create(context.Background(), CreateRequest{Key: "valid_key", Name: "Valid", Type: TypeBoolean})
	if err == nil {
		t.Fatal("expected configuration failure")
	}
	if !tx.rolledBack || tx.committed {
		t.Fatalf("rollback=%v committed=%v", tx.rolledBack, tx.committed)
	}
}

func TestCreateValidatesKeyTypeAndNameBeforeBeginningTransaction(t *testing.T) {
	store := &fakeStore{tx: &fakeFeatureTx{environments: expectedEnvironments()}}
	service := NewService(store)
	for _, request := range []CreateRequest{
		{Key: "123feature", Name: "Name", Type: TypeBoolean},
		{Key: "bad-key", Name: "Name", Type: TypeBoolean},
		{Key: "valid_key", Name: "  ", Type: TypeBoolean},
		{Key: "valid_key", Name: "Name", Type: "BOOLEAN_BAD"},
	} {
		if _, err := service.Create(context.Background(), request); err == nil {
			t.Errorf("request %#v unexpectedly succeeded", request)
		}
	}
	if store.beginN != 0 {
		t.Fatalf("began %d transactions for invalid input", store.beginN)
	}
}

func TestListValidatesPaginationAndReturnsEmptyArray(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	result, err := service.List(context.Background(), 1, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Data == nil || result.Pagination.Page != 1 || result.Pagination.Limit != 20 {
		t.Fatalf("result = %#v", result)
	}
	for _, values := range [][2]int64{{0, 20}, {1, 0}, {1, 26}, {1 << 62, 25}} {
		if _, err := service.List(context.Background(), values[0], values[1], ""); !errors.Is(err, ErrInvalidPage) {
			t.Errorf("List(%d,%d) error=%v", values[0], values[1], err)
		}
	}
}

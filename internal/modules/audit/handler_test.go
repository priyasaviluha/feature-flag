package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	items                    []AuditLog
	total                    int64
	findItem                 AuditLog
	findErr                  error
	environmentID, featureID *string
	page, limit              int64
	findID                   string
}

func (s *fakeStore) List(_ context.Context, environmentID, featureID *string, page, limit int64) ([]AuditLog, int64, error) {
	s.environmentID, s.featureID, s.page, s.limit = environmentID, featureID, page, limit
	return s.items, s.total, nil
}

func (s *fakeStore) Find(_ context.Context, id string) (AuditLog, error) {
	s.findID = id
	return s.findItem, s.findErr
}

func newAuditRequest(t *testing.T, store *fakeStore, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.SetPathValue("id", strings.TrimPrefix(strings.SplitN(path, "?", 2)[0], "/audit-logs/"))
	response := httptest.NewRecorder()
	handler := NewHandler(NewService(store))
	if strings.HasPrefix(path, "/audit-logs/") {
		handler.Get(response, request)
	} else {
		handler.List(response, request)
	}
	return response
}

func TestListFiltersAndDefaultPagination(t *testing.T) {
	envID := "00000000-0000-0000-0000-000000000001"
	featureID := "00000000-0000-0000-0000-000000000002"
	store := &fakeStore{items: []AuditLog{}, total: 7}
	response := newAuditRequest(t, store, "/audit-logs?environment_id="+envID+"&feature_id="+featureID)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body)
	}
	if store.environmentID == nil || *store.environmentID != envID || store.featureID == nil || *store.featureID != featureID {
		t.Fatalf("filters = %v, %v", store.environmentID, store.featureID)
	}
	if store.page != 1 || store.limit != 20 {
		t.Fatalf("pagination = %d/%d, want 1/20", store.page, store.limit)
	}
	var envelope struct {
		Data struct {
			Data       []AuditLog `json:"data"`
			Pagination Pagination `json:"pagination"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Data == nil || envelope.Data.Pagination.Total != 7 || envelope.Data.Pagination.Page != 1 || envelope.Data.Pagination.Limit != 20 {
		t.Fatalf("unexpected list response: %s", response.Body)
	}
}

func TestListCustomPaginationAndValidation(t *testing.T) {
	store := &fakeStore{}
	response := newAuditRequest(t, store, "/audit-logs?page=3&limit=100")
	if response.Code != http.StatusOK || store.page != 3 || store.limit != 100 {
		t.Fatalf("status=%d page/limit=%d/%d", response.Code, store.page, store.limit)
	}
	for _, path := range []string{
		"/audit-logs?environment_id=bad",
		"/audit-logs?feature_id=bad",
		"/audit-logs?page=0",
		"/audit-logs?page=nope",
		"/audit-logs?limit=101",
		"/audit-logs?limit=0",
	} {
		response := newAuditRequest(t, &fakeStore{}, path)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s status=%d, want 400", path, response.Code)
		}
	}
}

func TestListSupportsEachOptionalFilterIndependently(t *testing.T) {
	envID := "00000000-0000-0000-0000-000000000001"
	featureID := "00000000-0000-0000-0000-000000000002"
	for _, tc := range []struct {
		path        string
		wantEnv     *string
		wantFeature *string
	}{
		{path: "/audit-logs"},
		{path: "/audit-logs?environment_id=" + envID, wantEnv: &envID},
		{path: "/audit-logs?feature_id=" + featureID, wantFeature: &featureID},
	} {
		store := &fakeStore{}
		response := newAuditRequest(t, store, tc.path)
		if response.Code != http.StatusOK || !equalOptionalString(store.environmentID, tc.wantEnv) || !equalOptionalString(store.featureID, tc.wantFeature) {
			t.Errorf("%s: status=%d env=%v feature=%v", tc.path, response.Code, store.environmentID, store.featureID)
		}
	}
}

func equalOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func TestGetAuditRecordAndMissingRecord(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000003"
	item := AuditLog{ID: id, EnvironmentID: "env", FeatureID: "feature", Action: "CONFIG_UPDATE", OldValue: json.RawMessage(`{"enabled":false}`), NewValue: json.RawMessage(`{"enabled":true}`), CreatedAt: time.Unix(1, 0).UTC()}
	store := &fakeStore{findItem: item}
	response := newAuditRequest(t, store, "/audit-logs/"+id)
	if response.Code != http.StatusOK || store.findID != id {
		t.Fatalf("status=%d findID=%q", response.Code, store.findID)
	}
	if !strings.Contains(response.Body.String(), `"old_value":{"enabled":false}`) || !strings.Contains(response.Body.String(), `"new_value":{"enabled":true}`) {
		t.Fatalf("JSON values not preserved: %s", response.Body)
	}
	store.findErr = ErrNotFound
	response = newAuditRequest(t, store, "/audit-logs/"+id)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing record status=%d, want 404", response.Code)
	}
	response = newAuditRequest(t, &fakeStore{}, "/audit-logs/not-a-uuid")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid UUID status=%d, want 400", response.Code)
	}
}

func TestServiceReturnsEmptySliceAndRejectsInvalidPagination(t *testing.T) {
	store := &fakeStore{}
	result, err := NewService(store).List(context.Background(), nil, nil, 1, 20)
	if err != nil || result.Data == nil {
		t.Fatalf("empty list = %#v, err=%v", result, err)
	}
	if _, err := NewService(store).List(context.Background(), nil, nil, 1, 101); !errors.Is(err, ErrInvalidPagination) {
		t.Fatalf("invalid limit error = %v", err)
	}
	if !reflect.DeepEqual(result.Data, []AuditLog{}) {
		t.Fatalf("data = %#v", result.Data)
	}
}

package configuration

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

const testEnvironmentID = "00000000-0000-0000-0000-000000000001"

func TestListHandlerPaginationAndEmptyResult(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(NewService(store))
	r := httptest.NewRequest("GET", "/api/environments/"+testEnvironmentID+"/configs?page=2&limit=50", nil)
	r.SetPathValue("environmentId", testEnvironmentID)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if store.listPage != 2 || store.listLimit != 50 {
		t.Fatalf("list args=%d,%d", store.listPage, store.listLimit)
	}
	if !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatalf("empty data missing: %s", w.Body.String())
	}
}

func TestListHandlerRejectsInvalidPaginationAndEnvironmentUUID(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}))
	for _, tc := range []struct{ id, query string }{
		{"bad-id", ""},
		{testEnvironmentID, "?page=0"},
		{testEnvironmentID, "?limit=101"},
		{testEnvironmentID, "?page=9223372036854775807&limit=100"},
	} {
		r := httptest.NewRequest("GET", "/api/environments/"+tc.id+"/configs"+tc.query, nil)
		r.SetPathValue("environmentId", tc.id)
		w := httptest.NewRecorder()
		h.List(w, r)
		if w.Code != 400 {
			t.Errorf("%s%s status=%d", tc.id, tc.query, w.Code)
		}
	}
}

func TestGetConfigurationReturnsFeatureConfiguration(t *testing.T) {
	configJSON := json.RawMessage(`{"enabled":true}`)
	store := &fakeStore{findResult: Configuration{FeatureKey: "new_checkout", Type: "BOOLEAN", Config: configJSON, Version: 3}}
	r := httptest.NewRequest("GET", "/api/environments/"+testEnvironmentID+"/configs/new_checkout", nil)
	r.SetPathValue("environmentId", testEnvironmentID)
	r.SetPathValue("featureKey", "new_checkout")
	w := httptest.NewRecorder()
	NewHandler(NewService(store)).Get(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"version":3`) || !strings.Contains(w.Body.String(), `"feature_key":"new_checkout"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestUpdateConfigurationMapsTriggerValidationAndRequiresConfig(t *testing.T) {
	store := &fakeStore{configExists: true, featureID: "feature-id", featureType: "BOOLEAN", config: json.RawMessage(`{"enabled":false}`), version: 1}
	h := NewHandler(NewService(store))
	for _, body := range []string{`{}`, `{"config":{"enabled":true},"other":1}`} {
		r := httptest.NewRequest("PUT", "/api/environments/"+testEnvironmentID+"/configs/key", strings.NewReader(body))
		r.SetPathValue("environmentId", testEnvironmentID)
		r.SetPathValue("featureKey", "key")
		w := httptest.NewRecorder()
		h.Update(w, r)
		if w.Code != 400 {
			t.Errorf("body=%s status=%d", body, w.Code)
		}
	}

	tx := newFakeUpdateTx(store)
	tx.updateErr = ErrInvalidConfig
	store.mu.Lock()
	h = NewHandler(NewService(&fixedTxStore{fakeStore: store, tx: tx}))
	r := httptest.NewRequest("PUT", "/api/environments/"+testEnvironmentID+"/configs/key", strings.NewReader(`{"config":{"enabled":"yes"}}`))
	r.SetPathValue("environmentId", testEnvironmentID)
	r.SetPathValue("featureKey", "key")
	w := httptest.NewRecorder()
	h.Update(w, r)
	if w.Code != 400 {
		t.Fatalf("invalid config status=%d body=%s", w.Code, w.Body.String())
	}
}

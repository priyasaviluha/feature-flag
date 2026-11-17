package feature

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListHandlerDefaultsPaginationAndForwardsType(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(NewService(store))
	w := httptest.NewRecorder()
	h.List(w, httptest.NewRequest("GET", "/api/features?type=BOOLEAN", nil))
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if store.listPage != 1 || store.listLimit != 20 || store.listType != TypeBoolean {
		t.Fatalf("list args = %d,%d,%q", store.listPage, store.listLimit, store.listType)
	}
	if !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Fatalf("empty data missing: %s", w.Body.String())
	}
}

func TestListHandlerCustomPagination(t *testing.T) {
	store := &fakeStore{}
	w := httptest.NewRecorder()
	NewHandler(NewService(store)).List(w, httptest.NewRequest("GET", "/api/features?page=3&limit=25&type=NUMBER", nil))
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
	if store.listPage != 3 || store.listLimit != 25 || store.listType != TypeNumber {
		t.Fatalf("list args=%d,%d,%q", store.listPage, store.listLimit, store.listType)
	}
}

func TestListHandlerRejectsInvalidPaginationAndType(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}))
	for _, path := range []string{
		"/api/features?page=0",
		"/api/features?limit=26",
		"/api/features?page=9223372036854775807&limit=25",
		"/api/features?type=UNKNOWN",
	} {
		w := httptest.NewRecorder()
		h.List(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Errorf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}

func TestPatchHandlerPreservesDescriptionNullAndRejectsProtectedFields(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	store := &fakeStore{}
	h := NewHandler(NewService(store))
	r := httptest.NewRequest("PATCH", "/api/features/"+id, strings.NewReader(`{"description":null}`))
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	h.Update(w, r)
	if w.Code != 200 {
		t.Fatalf("null description status=%d body=%s", w.Code, w.Body.String())
	}
	if !store.updateDescriptionSet || store.updateDescription != nil || store.updateNameSet {
		t.Fatalf("description patch not preserved: %#v", store)
	}

	for _, body := range []string{`{}`, `{"key":"changed"}`, `{"type":"NUMBER"}`, `{"other":true}`, `{"name":"  "}`} {
		r := httptest.NewRequest("PATCH", "/api/features/"+id, strings.NewReader(body))
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		h.Update(w, r)
		if w.Code != 400 {
			t.Errorf("body=%s status=%d", body, w.Code)
		}
	}
}

func TestFeatureIDMustBeUUID(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}))
	r := httptest.NewRequest("GET", "/api/features/not-a-uuid", nil)
	r.SetPathValue("id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != 400 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPatchHandlerUpdatesNameAndDescription(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	store := &fakeStore{}
	r := httptest.NewRequest("PATCH", "/api/features/"+id, strings.NewReader(`{"name":" Updated ","description":"new description"}`))
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	NewHandler(NewService(store)).Update(w, r)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if store.updateName != "Updated" || !store.updateNameSet || !store.updateDescriptionSet || store.updateDescription == nil || *store.updateDescription != "new description" {
		t.Fatalf("patch args=%#v", store)
	}
}

func TestGetFeatureAndMissingFeatureStatuses(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	store := &fakeStore{found: Feature{ID: id, Key: "new_checkout", Name: "Checkout", Type: TypeBoolean}}
	h := NewHandler(NewService(store))
	r := httptest.NewRequest("GET", "/api/features/"+id, nil)
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != 200 {
		t.Fatalf("active status=%d", w.Code)
	}
	store.findErr = ErrNotFound
	w = httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != 404 {
		t.Fatalf("missing/archived status=%d", w.Code)
	}
}

func TestArchiveFeatureStatus(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	store := &fakeStore{}
	h := NewHandler(NewService(store))
	r := httptest.NewRequest("DELETE", "/api/features/"+id, nil)
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	h.Archive(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Feature archived successfully") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	store.archiveErr = ErrNotFound
	w = httptest.NewRecorder()
	h.Archive(w, r)
	if w.Code != 404 {
		t.Fatalf("repeated archive status=%d", w.Code)
	}
}

func TestCreateFeatureReturnsConflictForDuplicateKey(t *testing.T) {
	tx := &fakeFeatureTx{environments: expectedEnvironments(), insertErr: ErrDuplicateKey}
	h := NewHandler(NewService(&fakeStore{tx: tx}))
	r := httptest.NewRequest("POST", "/api/features", strings.NewReader(`{"key":"new_checkout","name":"Checkout","type":"BOOLEAN"}`))
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != 409 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

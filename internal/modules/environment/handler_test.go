package environment

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type handlerStore struct {
	items      []Environment
	found      Environment
	findErr    error
	archiveN   int
	updated    Environment
	updateID   string
	updateName string
	updateErr  error
}

func (s *handlerStore) List(context.Context) ([]Environment, error) { return s.items, nil }
func (s *handlerStore) FindByID(context.Context, string) (Environment, error) {
	return s.found, s.findErr
}
func (s *handlerStore) UpdateName(_ context.Context, id, name string) (Environment, error) {
	s.updateID, s.updateName = id, name
	return s.updated, s.updateErr
}
func (s *handlerStore) Archive(context.Context, string) error { s.archiveN++; return nil }

func TestListReturnsEmptyArrayAndOmitsArchivedAt(t *testing.T) {
	h := NewHandler(NewService(&handlerStore{}))
	r := httptest.NewRequest("GET", "/api/environments", nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var got struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Data == nil || len(got.Data) != 0 {
		t.Fatalf("data = %#v, want empty array", got.Data)
	}
}

func TestListReturnsActiveEnvironmentFields(t *testing.T) {
	created := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	store := &handlerStore{items: []Environment{{ID: "id", Name: "Web Development", Platform: "web", Type: "development", CreatedAt: created}}}
	w := httptest.NewRecorder()
	NewHandler(NewService(store)).List(w, httptest.NewRequest("GET", "/api/environments", nil))
	var got struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Data) != 1 || got.Data[0]["archived_at"] != nil || got.Data[0]["platform"] != "web" {
		t.Fatalf("unexpected list item: %#v", got.Data)
	}
}

func TestPatchValidatesUUIDAndUnsupportedFields(t *testing.T) {
	h := NewHandler(NewService(&handlerStore{}))
	for _, tc := range []struct{ path, body string }{
		{"bad-id", `{"name":"x"}`},
		{"00000000-0000-0000-0000-000000000001", `{"name":"x","platform":"ios"}`},
		{"00000000-0000-0000-0000-000000000001", `{"name":"  "}`},
	} {
		r := httptest.NewRequest("PATCH", "/api/environments/"+tc.path, strings.NewReader(tc.body))
		r.SetPathValue("id", tc.path)
		w := httptest.NewRecorder()
		h.Update(w, r)
		if w.Code != 400 {
			t.Errorf("%s: status = %d, want 400", tc.body, w.Code)
		}
	}
}

func TestPatchTrimsNameAndReturnsUpdated(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	store := &handlerStore{updated: Environment{ID: id, Name: "New Name", Platform: "web", Type: "development"}}
	r := httptest.NewRequest("PATCH", "/api/environments/"+id, strings.NewReader(`{"name":"  New Name  "}`))
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	NewHandler(NewService(store)).Update(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if store.updateID != id || store.updateName != "New Name" {
		t.Fatalf("update args = %q, %q", store.updateID, store.updateName)
	}
	if !strings.Contains(w.Body.String(), `"updated":true`) {
		t.Fatalf("missing updated flag: %s", w.Body.String())
	}
}

func TestGetReturnsEnvironmentAndMapsMissingToNotFound(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	store := &handlerStore{found: Environment{ID: id, Name: "Web Development", Platform: "web", Type: "development"}}
	h := NewHandler(NewService(store))
	r := httptest.NewRequest("GET", "/api/environments/"+id, nil)
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Web Development"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	store.findErr = ErrNotFound
	w = httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != 404 {
		t.Fatalf("missing environment status = %d", w.Code)
	}
}

func TestPatchMapsDuplicateNameToConflict(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	store := &handlerStore{updateErr: ErrDuplicateName}
	r := httptest.NewRequest("PATCH", "/api/environments/"+id, strings.NewReader(`{"name":"Taken"}`))
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	NewHandler(NewService(store)).Update(w, r)
	if w.Code != 409 {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestRepeatedArchiveRequestsSucceed(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	store := &handlerStore{}
	h := NewHandler(NewService(store))
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("DELETE", "/api/environments/"+id, nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		h.Delete(w, r)
		if w.Code != 200 {
			t.Fatalf("attempt %d status = %d", i+1, w.Code)
		}
	}
	if store.archiveN != 2 {
		t.Fatalf("archive calls = %d", store.archiveN)
	}
}

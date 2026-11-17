package usergroup

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func runHandler(handler http.HandlerFunc, method, path, body string, values map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	for key, value := range values {
		request.SetPathValue(key, value)
	}
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

func TestCreateGroupRequestValidationAndDuplicate(t *testing.T) {
	store := newFakeStore()
	handler := NewHandler(NewService(store))
	if got := runHandler(handler.Create, http.MethodPost, "/api/user-groups", `{"name":"   "}`, nil).Code; got != http.StatusBadRequest {
		t.Fatalf("blank name status = %d, want 400", got)
	}
	created := runHandler(handler.Create, http.MethodPost, "/api/user-groups", `{"name":" Dev Team ","description":null}`, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}
	store.createErr = ErrDuplicateName
	duplicate := runHandler(handler.Create, http.MethodPost, "/api/user-groups", `{"name":"Dev Team"}`, nil)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", duplicate.Code)
	}
}

func TestListAndArchivedGroupDetail(t *testing.T) {
	store := newFakeStore()
	store.list = []GroupSummary{{ID: "active", Name: "Active"}}
	handler := NewHandler(NewService(store))
	listed := runHandler(handler.List, http.MethodGet, "/api/user-groups", "", nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"data":[{"id":"active"`) {
		t.Fatalf("list response = %d %s", listed.Code, listed.Body.String())
	}
	store.list = nil
	empty := runHandler(handler.List, http.MethodGet, "/api/user-groups", "", nil)
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), `"data":[]`) {
		t.Fatalf("empty list response = %d %s", empty.Code, empty.Body.String())
	}
	archivedAt := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	store.find = Group{ID: "archived", Name: "Old group", ArchivedAt: &archivedAt}
	detail := runHandler(handler.Get, http.MethodGet, "/api/user-groups/id", "", map[string]string{"id": "00000000-0000-0000-0000-000000000003"})
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"archived_at":"2026-11-16T10:00:00Z"`) {
		t.Fatalf("archived detail response = %d %s", detail.Code, detail.Body.String())
	}
}

func TestPatchAndArchiveConflicts(t *testing.T) {
	store := newFakeStore()
	handler := NewHandler(NewService(store))
	path := map[string]string{"id": "00000000-0000-0000-0000-000000000003"}
	if got := runHandler(handler.Patch, http.MethodPatch, "/api/user-groups/id", `{}`, path).Code; got != http.StatusBadRequest {
		t.Fatalf("empty patch status = %d, want 400", got)
	}
	store.patchErr = ErrDuplicateName
	if got := runHandler(handler.Patch, http.MethodPatch, "/api/user-groups/id", `{"name":"Existing"}`, path).Code; got != http.StatusConflict {
		t.Fatalf("duplicate patch status = %d, want 409", got)
	}
	store.patchErr = nil
	updated := runHandler(handler.Patch, http.MethodPatch, "/api/user-groups/id", `{"name":"Backend Dev Team","description":"Backend engineering"}`, path)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"updated":true`) || !strings.Contains(updated.Body.String(), `"description":"Backend engineering"`) {
		t.Fatalf("successful patch response = %d %s", updated.Code, updated.Body.String())
	}
	store.patchErr = ErrGroupArchived
	if got := runHandler(handler.Patch, http.MethodPatch, "/api/user-groups/id", `{"description":null}`, path).Code; got != http.StatusConflict {
		t.Fatalf("archived patch status = %d, want 409", got)
	}
	store.archiveErr = ErrAlreadyArchived
	if got := runHandler(handler.Archive, http.MethodDelete, "/api/user-groups/id", "", path).Code; got != http.StatusConflict {
		t.Fatalf("repeated archive status = %d, want 409", got)
	}
}

func TestEffectiveGroupsReturnOverrideAndFallback(t *testing.T) {
	store := newFakeStore()
	store.effective = EnvironmentFeatureGroups{
		FeatureKey: "new_checkout", Environment: "Development",
		Groups: []EffectiveConfig{
			{GroupID: "one", GroupName: "Dev Team", Config: json.RawMessage(`{"enabled":true}`), Version: 2, IsOverride: true},
			{GroupID: "two", GroupName: "QA Team", Config: json.RawMessage(`{"enabled":false}`), Version: 1, IsOverride: false},
		},
	}
	handler := NewHandler(NewService(store))
	result := runHandler(handler.GetEffective, http.MethodGet, "/api/environments/env/features/feature/groups", "", map[string]string{
		"environmentId": "00000000-0000-0000-0000-000000000001",
		"featureId":     "00000000-0000-0000-0000-000000000002",
	})
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"is_override":true`) || !strings.Contains(result.Body.String(), `"is_override":false`) {
		t.Fatalf("effective groups response = %d %s", result.Code, result.Body.String())
	}
	store.effective.Groups = nil
	empty := runHandler(handler.GetEffective, http.MethodGet, "/api/environments/env/features/feature/groups", "", map[string]string{
		"environmentId": "00000000-0000-0000-0000-000000000001",
		"featureId":     "00000000-0000-0000-0000-000000000002",
	})
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), `"groups":[]`) {
		t.Fatalf("empty effective groups response = %d %s", empty.Code, empty.Body.String())
	}
	store.effectiveErr = ErrDefaultConfigurationNotFound
	missingDefault := runHandler(handler.GetEffective, http.MethodGet, "/api/environments/env/features/feature/groups", "", map[string]string{
		"environmentId": "00000000-0000-0000-0000-000000000001",
		"featureId":     "00000000-0000-0000-0000-000000000002",
	})
	if missingDefault.Code != http.StatusNotFound {
		t.Fatalf("missing default status = %d, want 404", missingDefault.Code)
	}
}

func TestPutAndDeleteRolloutRuleErrors(t *testing.T) {
	store := newFakeStore()
	handler := NewHandler(NewService(store))
	path := map[string]string{
		"environmentId": "00000000-0000-0000-0000-000000000001",
		"featureId":     "00000000-0000-0000-0000-000000000002",
		"groupId":       "00000000-0000-0000-0000-000000000003",
	}
	invalid := runHandler(handler.PutRule, http.MethodPut, "/api/environments/e/features/f/groups/g", `{"config":{"enabled":"yes"}}`, path)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid config status = %d, want 400", invalid.Code)
	}
	store.activeGroupErr = ErrGroupArchived
	archived := runHandler(handler.PutRule, http.MethodPut, "/api/environments/e/features/f/groups/g", `{"config":{"enabled":true}}`, path)
	if archived.Code != http.StatusConflict {
		t.Fatalf("archived group status = %d, want 409", archived.Code)
	}
	store.activeGroupErr = nil
	store.version = 1
	updated := runHandler(handler.PutRule, http.MethodPut, "/api/environments/e/features/f/groups/g", `{"config":{"enabled":true}}`, path)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"version":2`) {
		t.Fatalf("rule update response = %d %s", updated.Code, updated.Body.String())
	}
	store.deleteErr = ErrRuleNotFound
	missing := runHandler(handler.DeleteRule, http.MethodDelete, "/api/environments/e/features/f/groups/g", "", path)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing rule status = %d, want 404", missing.Code)
	}
	store.deleteErr = nil
	removed := runHandler(handler.DeleteRule, http.MethodDelete, "/api/environments/e/features/f/groups/g", "", path)
	if removed.Code != http.StatusOK || !strings.Contains(removed.Body.String(), "Feature group rollout rule removed successfully") {
		t.Fatalf("delete response = %d %s", removed.Code, removed.Body.String())
	}
	if got := runHandler(handler.GetEffective, http.MethodGet, "/api/environments/e/features/f/groups", "", map[string]string{"environmentId": "bad", "featureId": path["featureId"]}).Code; got != http.StatusBadRequest {
		t.Fatalf("invalid UUID status = %d, want 400", got)
	}
}

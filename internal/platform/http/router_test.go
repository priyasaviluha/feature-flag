package http

import (
	"net/http/httptest"
	"testing"

	"flag_backend/internal/config"
)

func TestEnvironmentAndFeatureRoutesRequireAuthentication(t *testing.T) {
	router := NewRouter(nil, config.Config{JWTSecret: "a sufficiently long test secret"})
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/environments"},
		{"GET", "/api/environments/00000000-0000-0000-0000-000000000001"},
		{"PATCH", "/api/environments/00000000-0000-0000-0000-000000000001"},
		{"DELETE", "/api/environments/00000000-0000-0000-0000-000000000001"},
		{"GET", "/api/environments/00000000-0000-0000-0000-000000000001/configs"},
		{"GET", "/api/environments/00000000-0000-0000-0000-000000000001/configs/new_checkout"},
		{"PUT", "/api/environments/00000000-0000-0000-0000-000000000001/configs/new_checkout"},
		{"GET", "/api/environments/00000000-0000-0000-0000-000000000001/features/00000000-0000-0000-0000-000000000002/groups"},
		{"PUT", "/api/environments/00000000-0000-0000-0000-000000000001/features/00000000-0000-0000-0000-000000000002/groups/00000000-0000-0000-0000-000000000003"},
		{"DELETE", "/api/environments/00000000-0000-0000-0000-000000000001/features/00000000-0000-0000-0000-000000000002/groups/00000000-0000-0000-0000-000000000003"},
		{"POST", "/api/user-groups"},
		{"GET", "/api/user-groups"},
		{"GET", "/api/user-groups/00000000-0000-0000-0000-000000000003"},
		{"PATCH", "/api/user-groups/00000000-0000-0000-0000-000000000003"},
		{"DELETE", "/api/user-groups/00000000-0000-0000-0000-000000000003"},
		{"POST", "/api/features"},
		{"GET", "/api/features"},
		{"GET", "/api/features/00000000-0000-0000-0000-000000000001"},
		{"PATCH", "/api/features/00000000-0000-0000-0000-000000000001"},
		{"DELETE", "/api/features/00000000-0000-0000-0000-000000000001"},
		{"GET", "/sdk/v1/configs"},
		{"GET", "/sdk/v1/configs/new_checkout"},
		{"GET", "/audit-logs"},
		{"GET", "/audit-logs/00000000-0000-0000-0000-000000000001"},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Errorf("%s %s: status = %d, want 401", tc.method, tc.path, w.Code)
		}
	}
}

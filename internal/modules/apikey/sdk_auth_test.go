package apikey

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeAuthenticator struct {
	wantHash string
	key      AuthenticatedKey
	err      error
}

func (f fakeAuthenticator) Authenticate(_ context.Context, hash string) (AuthenticatedKey, string, error) {
	if hash != f.wantHash {
		return AuthenticatedKey{}, "", errors.New("hash mismatch")
	}
	return f.key, "ACTIVE", f.err
}

func TestAuthenticateSDKKey(t *testing.T) {
	const raw = "ff_live_test_secret"
	key := AuthenticatedKey{ID: "key-id", EnvironmentID: "env-id", Environment: "Production"}
	for _, tc := range []struct {
		name       string
		header     string
		err        error
		wantStatus int
		wantNext   bool
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "invalid", header: raw, err: ErrAPIKeyNotFound, wantStatus: http.StatusUnauthorized},
		{name: "revoked or expired", header: raw, err: ErrAPIKeyInactive, wantStatus: http.StatusForbidden},
		{name: "active", header: raw, wantStatus: http.StatusOK, wantNext: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := fakeAuthenticator{wantHash: HashKey(raw), key: key, err: tc.err}
			handlerCalled := false
			handler := AuthenticateSDKKey(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				identity, ok := SDKIdentityFromContext(r.Context())
				if !ok || identity.EnvironmentID != "env-id" || identity.Environment != "Production" || identity.APIKeyID != "key-id" {
					t.Errorf("unexpected identity: %#v, present=%v", identity, ok)
				}
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/sdk/v1/configs", nil)
			if tc.header != "" {
				req.Header.Set("X-API-Key", tc.header)
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != tc.wantStatus || handlerCalled != tc.wantNext {
				t.Fatalf("status=%d called=%v; want status=%d called=%v", res.Code, handlerCalled, tc.wantStatus, tc.wantNext)
			}
		})
	}
}

func TestHashKeyMatchesAPIKeyCreationFormat(t *testing.T) {
	const raw = "ff_live_00010203040506070809"
	if got, want := HashKey(raw), "224529d1e6705e66fd0667fb556675af938af7d76b1ef85560c2b85d1aa8e98c"; got != want {
		t.Fatalf("HashKey(%q) = %q, want %q", raw, got, want)
	}
}

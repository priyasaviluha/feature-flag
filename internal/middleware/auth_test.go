package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sharedauth "flag_backend/internal/auth"
)

func rawToken(header, payload string, secret []byte) string {
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestAuthenticationMiddleware(t *testing.T) {
	secret := []byte(strings.Repeat("z", 32))
	valid, _, err := sharedauth.Sign("00000000-0000-4000-8000-000000000001", secret, time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	wrongAlg := rawToken(`{"alg":"none","typ":"JWT"}`, `{"sub":"admin","iat":1,"exp":9999999999,"iss":"flag-backend"}`, secret)
	missingClaims := rawToken(`{"alg":"HS256","typ":"JWT"}`, `{"iat":1,"exp":9999999999,"iss":"flag-backend"}`, secret)
	expiredPayload, _ := json.Marshal(sharedauth.Claims{Subject: "admin", IssuedAt: time.Now().Add(-time.Hour).Unix(), Expires: time.Now().Add(-time.Minute).Unix(), Issuer: sharedauth.Issuer})
	expired := rawToken(`{"alg":"HS256","typ":"JWT"}`, string(expiredPayload), secret)
	tampered := valid[:len(valid)-1] + "x"
	endpoint := Authenticate(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := AdminID(r.Context()); !ok || id != "00000000-0000-4000-8000-000000000001" {
			t.Errorf("admin identity missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name, token string
		status      int
	}{
		{"valid", valid, http.StatusNoContent}, {"missing cookie", "", http.StatusUnauthorized}, {"tampered signature", tampered, http.StatusUnauthorized}, {"expired", expired, http.StatusUnauthorized}, {"malformed", "bad.token", http.StatusUnauthorized}, {"wrong algorithm", wrongAlg, http.StatusUnauthorized}, {"missing subject", missingClaims, http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/admin/settings", nil)
			if test.token != "" {
				req.AddCookie(&http.Cookie{Name: "admin_token", Value: test.token})
			}
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, req)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
		})
	}
}

func TestAuthenticationMiddlewareCSRF(t *testing.T) {
	secret := []byte(strings.Repeat("z", 32))
	token, _, _ := sharedauth.Sign("00000000-0000-4000-8000-000000000001", secret, time.Hour, time.Now())
	endpoint := Authenticate(secret)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, withHeader := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/change", nil)
		req.AddCookie(&http.Cookie{Name: "admin_token", Value: token})
		req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "csrf-value"})
		if withHeader {
			req.Header.Set("X-CSRF-Token", "csrf-value")
		}
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, req)
		want := http.StatusForbidden
		if withHeader {
			want = http.StatusNoContent
		}
		if recorder.Code != want {
			t.Fatalf("withHeader=%v: status = %d, want %d", withHeader, recorder.Code, want)
		}
	}
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowlistAndCredentials(t *testing.T) {
	called := false
	handler := CORS([]string{"https://console.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	allowed := httptest.NewRequest(http.MethodOptions, "/api/admin/login", nil)
	allowed.Header.Set("Origin", "https://console.example.com")
	allowedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(allowedRecorder, allowed)
	if allowedRecorder.Code != http.StatusNoContent || allowedRecorder.Header().Get("Access-Control-Allow-Origin") != "https://console.example.com" || allowedRecorder.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("unexpected CORS preflight response: %#v", allowedRecorder)
	}
	if called {
		t.Fatal("CORS preflight should not invoke downstream routing")
	}

	denied := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	denied.Header.Set("Origin", "https://attacker.example")
	deniedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(deniedRecorder, denied)
	if deniedRecorder.Code != http.StatusForbidden || called {
		t.Fatal("unlisted origins must be rejected before routing")
	}
}

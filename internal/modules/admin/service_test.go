package admin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sharedauth "flag_backend/internal/auth"
	"flag_backend/internal/config"
	"flag_backend/internal/middleware"
	"golang.org/x/crypto/argon2"
)

type fakeAdminStore struct {
	admin Admin
	err   error
}

func TestLogoutRequiresAuthAndCSRFAndExpiresCookies(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	loginHandler := NewHandler(nil, true)
	endpoint := middleware.Authenticate(secret)(http.HandlerFunc(loginHandler.Logout))
	token, _, err := sharedauth.Sign("00000000-0000-4000-8000-000000000001", secret, time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	request := func(authenticated, csrfCookie, csrfHeader bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
		if authenticated {
			req.AddCookie(&http.Cookie{Name: "admin_token", Value: token})
		}
		if csrfCookie {
			req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "csrf-value"})
		}
		if csrfHeader {
			req.Header.Set("X-CSRF-Token", "csrf-value")
		}
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, req)
		return recorder
	}
	if got := request(false, true, true).Code; got != http.StatusUnauthorized {
		t.Fatalf("missing auth status=%d", got)
	}
	if got := request(true, false, true).Code; got != http.StatusForbidden {
		t.Fatalf("missing CSRF cookie status=%d", got)
	}
	if got := request(true, true, false).Code; got != http.StatusForbidden {
		t.Fatalf("missing CSRF header status=%d", got)
	}

	recorder := request(true, true, true)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Logout successful") {
		t.Fatalf("unexpected logout response: %d %s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("got %d cleared cookies, want 2", len(cookies))
	}
	for _, cookie := range cookies {
		if cookie.MaxAge != -1 || cookie.Expires.After(time.Now()) || cookie.Path != "/" || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("cookie attributes differ from login: %#v", cookie)
		}
		if cookie.Name == "admin_token" && !cookie.HttpOnly {
			t.Error("admin_token must remain HttpOnly")
		}
		if cookie.Name == "csrf_token" && cookie.HttpOnly {
			t.Error("csrf_token must remain readable")
		}
	}
}

func (f fakeAdminStore) FindByEmail(context.Context, string) (Admin, error) { return f.admin, f.err }
func (f fakeAdminStore) FindByID(context.Context, string) (Admin, error)    { return f.admin, f.err }

func TestMeReturnsAuthenticatedAdminEmail(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	admin := Admin{ID: "00000000-0000-4000-8000-000000000001", Email: "admin@example.com", PasswordHash: "must-not-leak"}
	cfg := config.Config{JWTSecret: string(secret)}
	handler := NewHandler(NewService(fakeAdminStore{admin: admin}, cfg), false)
	endpoint := middleware.Authenticate(secret)(http.HandlerFunc(handler.Me))
	token, _, err := sharedauth.Sign(admin.ID, secret, time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	request.AddCookie(&http.Cookie{Name: "admin_token", Value: token})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), admin.Email) {
		t.Fatalf("unexpected profile response: %d %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), admin.PasswordHash) {
		t.Fatal("profile response exposed password hash")
	}
}

func testPasswordHash(password string, memory uint32, iterations uint32, parallelism uint8) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, 32)
	return "$argon2id$v=19$m=" + utoa(memory) + ",t=" + utoa(iterations) + ",p=" + utoa(uint32(parallelism)) + "$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
}

func utoa(value uint32) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = digits[value%10]
		value /= 10
	}
	return string(buf[i:])
}

func TestVerifyPassword(t *testing.T) {
	encoded := testPasswordHash("correct horse", 8, 1, 1)
	if !verifyPassword("correct horse", encoded) {
		t.Fatal("expected correct password to verify")
	}
	if verifyPassword("wrong", encoded) {
		t.Fatal("expected wrong password to fail")
	}
	if verifyPassword("correct horse", "not-a-hash") {
		t.Fatal("expected malformed hash to fail")
	}
	if verifyPassword("correct horse", strings.Replace(encoded, "m=8", "m=999999999", 1)) {
		t.Fatal("expected excessive Argon2 memory to fail")
	}
	if verifyPassword("correct horse", strings.Replace(encoded, "p=1", "p=0", 1)) {
		t.Fatal("expected invalid parallelism to fail")
	}
}

func TestLoginHandlerValidationAndCookies(t *testing.T) {
	admin := Admin{ID: "00000000-0000-4000-8000-000000000001", Email: "admin@example.com", PasswordHash: testPasswordHash("secret", 8, 1, 1)}
	cfg := config.Config{JWTSecret: strings.Repeat("s", 32), JWTExpiry: time.Hour}
	handler := NewHandler(NewService(fakeAdminStore{admin: admin}, cfg), false)
	request := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		handler.Login(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(body)))
		return recorder
	}

	tests := []struct {
		name, body string
		status     int
	}{
		{"missing email", `{"password":"secret"}`, http.StatusBadRequest},
		{"missing password", `{"email":"admin@example.com"}`, http.StatusBadRequest},
		{"malformed JSON", `{`, http.StatusBadRequest},
		{"unknown field", `{"email":"admin@example.com","password":"secret","extra":true}`, http.StatusBadRequest},
		{"second JSON value", `{"email":"admin@example.com","password":"secret"} {}`, http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := request(test.body).Code; got != test.status {
				t.Fatalf("status = %d, want %d", got, test.status)
			}
		})
	}

	response := request(`{"email":"admin@example.com","password":"secret"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), ".") {
		t.Fatal("login response body should not contain a JWT")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("got %d cookies, want admin and CSRF cookies", len(cookies))
	}
	var adminCookie, csrfCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "admin_token" {
			adminCookie = cookie
		}
		if cookie.Name == "csrf_token" {
			csrfCookie = cookie
		}
	}
	if adminCookie == nil || !adminCookie.HttpOnly || adminCookie.Secure || adminCookie.Path != "/" || adminCookie.SameSite != http.SameSiteLaxMode || adminCookie.MaxAge <= 0 {
		t.Fatalf("unexpected admin cookie: %#v", adminCookie)
	}
	if csrfCookie == nil || csrfCookie.HttpOnly {
		t.Fatalf("expected readable CSRF cookie, got %#v", csrfCookie)
	}
	if adminCookie.Expires.Before(time.Now().Add(50*time.Minute)) || adminCookie.Expires.After(time.Now().Add(61*time.Minute)) {
		t.Fatalf("cookie expiry does not match configured JWT expiry: %v", adminCookie.Expires)
	}
	secureHandler := NewHandler(NewService(fakeAdminStore{admin: admin}, cfg), true)
	secureRecorder := httptest.NewRecorder()
	secureHandler.Login(secureRecorder, httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"email":"admin@example.com","password":"secret"}`)))
	if cookies := secureRecorder.Result().Cookies(); len(cookies) == 0 || !cookies[0].Secure {
		t.Fatal("expected Secure cookie when configured")
	}

	failed := NewHandler(NewService(fakeAdminStore{admin: admin}, cfg), true)
	recorder := httptest.NewRecorder()
	failed.Login(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"email":"admin@example.com","password":"wrong"}`)))
	if recorder.Code != http.StatusUnauthorized || len(recorder.Result().Cookies()) != 0 {
		t.Fatal("failed login must return 401 without cookies")
	}

	notFound := NewHandler(NewService(fakeAdminStore{err: ErrAdminNotFound}, cfg), false)
	missingRecorder := httptest.NewRecorder()
	notFound.Login(missingRecorder, httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"email":"missing@example.com","password":"secret"}`)))
	if missingRecorder.Code != http.StatusUnauthorized || !strings.Contains(missingRecorder.Body.String(), "Invalid email or password") {
		t.Fatal("nonexistent admin should receive generic authentication failure")
	}
	databaseFailure := NewHandler(NewService(fakeAdminStore{err: errors.New("database detail")}, cfg), false)
	databaseRecorder := httptest.NewRecorder()
	databaseFailure.Login(databaseRecorder, httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"email":"admin@example.com","password":"secret"}`)))
	if databaseRecorder.Code != http.StatusInternalServerError || strings.Contains(databaseRecorder.Body.String(), "database detail") {
		t.Fatal("unexpected errors must be hidden from clients")
	}
}

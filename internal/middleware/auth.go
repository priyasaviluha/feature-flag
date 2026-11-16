package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	sharedauth "flag_backend/internal/auth"
	"flag_backend/internal/platform/response"
)

type contextKey string

const adminIDKey contextKey = "authenticated_admin_id"

func AdminID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(adminIDKey).(string)
	return id, ok
}

func Authenticate(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("admin_token")
			if err != nil {
				response.WriteJSON(w, http.StatusUnauthorized, response.Response{Success: false, Message: "Unauthorized"})
				return
			}
			claims, err := sharedauth.Verify(cookie.Value, secret, time.Now())
			if err != nil {
				response.WriteJSON(w, http.StatusUnauthorized, response.Response{Success: false, Message: "Unauthorized"})
				return
			}
			if changesState(r.Method) {
				csrfCookie, cookieErr := r.Cookie("csrf_token")
				header := r.Header.Get("X-CSRF-Token")
				if cookieErr != nil || header == "" || len(header) != len(csrfCookie.Value) || subtle.ConstantTimeCompare([]byte(header), []byte(csrfCookie.Value)) != 1 {
					response.WriteJSON(w, http.StatusForbidden, response.Response{Success: false, Message: "CSRF validation failed"})
					return
				}
			}
			ctx := context.WithValue(r.Context(), adminIDKey, claims.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func changesState(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

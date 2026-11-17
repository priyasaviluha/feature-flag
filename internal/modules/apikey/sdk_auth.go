package apikey

import (
	"context"
	"errors"
	"net/http"

	"flag_backend/internal/platform/response"
)

type SDKIdentity struct {
	APIKeyID      string
	EnvironmentID string
	Environment   string
}

type sdkIdentityKey struct{}

func SDKIdentityFromContext(ctx context.Context) (SDKIdentity, bool) {
	identity, ok := ctx.Value(sdkIdentityKey{}).(SDKIdentity)
	return identity, ok
}

type Authenticator interface {
	Authenticate(context.Context, string) (AuthenticatedKey, string, error)
}

func AuthenticateSDKKey(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("X-API-Key")
			if raw == "" {
				response.WriteJSON(w, http.StatusUnauthorized, response.Response{Success: false, Message: "Unauthorized"})
				return
			}
			key, _, err := auth.Authenticate(r.Context(), HashKey(raw))
			if errors.Is(err, ErrAPIKeyNotFound) {
				response.WriteJSON(w, http.StatusUnauthorized, response.Response{Success: false, Message: "Unauthorized"})
				return
			}
			if errors.Is(err, ErrAPIKeyInactive) {
				response.WriteJSON(w, http.StatusForbidden, response.Response{Success: false, Message: "API key or environment inactive"})
				return
			}
			if err != nil {
				response.WriteJSON(w, http.StatusInternalServerError, response.Response{Success: false, Message: "Internal server error"})
				return
			}
			identity := SDKIdentity{APIKeyID: key.ID, EnvironmentID: key.EnvironmentID, Environment: key.Environment}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sdkIdentityKey{}, identity)))
		})
	}
}

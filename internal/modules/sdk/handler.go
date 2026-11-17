package sdk

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"flag_backend/internal/modules/apikey"
	"flag_backend/internal/modules/configuration"
	"flag_backend/internal/platform/response"
)

type Handler struct {
	configurations *configuration.Repository
	service        *configuration.Service
}

func NewHandler(repository *configuration.Repository, service *configuration.Service) *Handler {
	return &Handler{configurations: repository, service: service}
}

type snapshotResponse struct {
	Environment string                    `json:"environment"`
	Version     int64                     `json:"version"`
	Configs     map[string]snapshotConfig `json:"configs"`
	FetchedAt   time.Time                 `json:"fetched_at"`
}

type snapshotConfig struct {
	Type    string `json:"type"`
	Config  any    `json:"config"`
	Version int64  `json:"version"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	identity, ok := apikey.SDKIdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	snapshot, err := h.configurations.Snapshot(r.Context(), identity.EnvironmentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	configs := make(map[string]snapshotConfig, len(snapshot.Configs))
	for _, item := range snapshot.Configs {
		configs[item.FeatureKey] = snapshotConfig{Type: item.Type, Config: item.Config, Version: item.Version}
	}
	writeJSON(w, http.StatusOK, snapshotResponse{Environment: snapshot.Environment, Version: snapshot.Version, Configs: configs, FetchedAt: time.Now().UTC()})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	identity, ok := apikey.SDKIdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	item, err := h.service.Find(r.Context(), identity.EnvironmentID, r.PathValue("featureKey"))
	if err != nil {
		switch {
		case errors.Is(err, configuration.ErrFeatureNotFound), errors.Is(err, configuration.ErrConfigNotFound), errors.Is(err, configuration.ErrEnvironmentNotFound):
			writeError(w, http.StatusNotFound, "Configuration not found")
		default:
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, struct {
		FeatureKey string `json:"feature_key"`
		Type       string `json:"type"`
		Config     any    `json:"config"`
		Version    int64  `json:"version"`
	}{item.FeatureKey, item.Type, item.Config, item.Version})
}

func writeError(w http.ResponseWriter, status int, message string) {
	response.WriteJSON(w, status, response.Response{Success: false, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

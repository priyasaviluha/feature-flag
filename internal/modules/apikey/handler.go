package apikey

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"flag_backend/internal/middleware"
	"flag_backend/internal/platform/response"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string `json:"name"`
		EnvironmentID string `json:"environment_id"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Name == "" || body.EnvironmentID == "" {
		writeError(w, http.StatusBadRequest, "name and environment_id are required")
		return
	}
	if !uuidPattern.MatchString(body.EnvironmentID) {
		writeError(w, http.StatusBadRequest, "Invalid environment ID")
		return
	}
	adminID, ok := middleware.AdminID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	item, err := h.service.Create(r.Context(), body.Name, body.EnvironmentID, adminID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, response.Response{Success: true, Data: item})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	values, present := r.URL.Query()["environment_id"]
	if !present || len(values) != 1 || values[0] == "" {
		writeError(w, http.StatusBadRequest, "environment_id is required")
		return
	}
	if !uuidPattern.MatchString(values[0]) {
		writeError(w, http.StatusBadRequest, "Invalid environment ID")
		return
	}
	page, err := parsePositive(r, "page", 1)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid pagination parameters")
		return
	}
	limit, err := parsePositive(r, "limit", 25)
	if err != nil || limit > MaxLimit {
		writeError(w, http.StatusBadRequest, "Invalid pagination parameters")
		return
	}
	result, err := h.service.List(r.Context(), values[0], page, limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: result})
}

func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "Invalid API key ID")
		return
	}
	adminID, ok := middleware.AdminID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if err := h.service.Revoke(r.Context(), id, adminID); err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Message: "API key revoked successfully"})
}

func parsePositive(r *http.Request, key string, defaultValue int64) (int64, error) {
	values, ok := r.URL.Query()[key]
	if !ok {
		return defaultValue, nil
	}
	if len(values) != 1 || values[0] == "" {
		return 0, errors.New("invalid integer")
	}
	value, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil || value < 1 {
		return 0, errors.New("invalid integer")
	}
	return value, nil
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidPagination):
		writeError(w, http.StatusBadRequest, "Invalid API key request")
	case errors.Is(err, ErrEnvironmentNotFound), errors.Is(err, ErrAPIKeyNotFound):
		writeError(w, http.StatusNotFound, "Resource not found")
	default:
		writeError(w, http.StatusInternalServerError, "Internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	response.WriteJSON(w, status, response.Response{Success: false, Message: message})
}

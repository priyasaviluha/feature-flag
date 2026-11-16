package configuration

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"flag_backend/internal/platform/response"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

var environmentUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	environmentID, ok := environmentID(w, r)
	if !ok {
		return
	}
	page, err := parsePageLimit(r, "page", 1)
	if err != nil {
		writeError(w, http.StatusBadRequest, "page must be a positive integer")
		return
	}
	limit, err := parsePageLimit(r, "limit", 25)
	if err != nil || limit > MaxLimit {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
		return
	}
	result, err := h.service.List(r.Context(), environmentID, page, limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: result})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	environmentID, ok := environmentID(w, r)
	if !ok {
		return
	}
	item, err := h.service.Find(r.Context(), environmentID, r.PathValue("featureKey"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: item})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	environmentID, ok := environmentID(w, r)
	if !ok {
		return
	}
	var body struct {
		Config json.RawMessage `json:"config"`
	}
	if err := decodeStrictJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Config == nil {
		writeError(w, http.StatusBadRequest, "config is required")
		return
	}
	item, err := h.service.Update(r.Context(), environmentID, r.PathValue("featureKey"), body.Config)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: item})
}

func environmentID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("environmentId")
	if !environmentUUIDPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "Invalid environment ID")
		return "", false
	}
	return id, true
}

func parsePageLimit(r *http.Request, key string, defaultValue int64) (int64, error) {
	values, present := r.URL.Query()[key]
	if !present {
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

func decodeStrictJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		writeError(w, http.StatusNotFound, "Environment not found")
	case errors.Is(err, ErrFeatureNotFound):
		writeError(w, http.StatusNotFound, "Feature not found")
	case errors.Is(err, ErrConfigNotFound):
		writeError(w, http.StatusNotFound, "Environment configuration not found")
	case errors.Is(err, ErrInvalidPagination):
		writeError(w, http.StatusBadRequest, "Invalid pagination parameters")
	case errors.Is(err, ErrInvalidConfig):
		writeError(w, http.StatusBadRequest, "Configuration does not match feature type")
	default:
		writeError(w, http.StatusInternalServerError, "Internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	response.WriteJSON(w, status, response.Response{Success: false, Message: message})
}

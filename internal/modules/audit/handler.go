package audit

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"flag_backend/internal/platform/response"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	environmentID, ok := optionalUUID(w, r, "environment_id")
	if !ok {
		return
	}
	featureID, ok := optionalUUID(w, r, "feature_id")
	if !ok {
		return
	}
	page, err := parseQueryInt(r, "page", 1)
	if err != nil || page < 1 {
		writeError(w, http.StatusBadRequest, "Invalid pagination parameters")
		return
	}
	limit, err := parseQueryInt(r, "limit", 20)
	if err != nil || limit < 1 || limit > MaxLimit {
		writeError(w, http.StatusBadRequest, "Invalid pagination parameters")
		return
	}
	result, err := h.service.List(r.Context(), environmentID, featureID, page, limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: result})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "Invalid audit log ID")
		return
	}
	item, err := h.service.Find(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: item})
}

func optionalUUID(w http.ResponseWriter, r *http.Request, key string) (*string, bool) {
	values, present := r.URL.Query()[key]
	if !present {
		return nil, true
	}
	if len(values) != 1 || !uuidPattern.MatchString(values[0]) {
		writeError(w, http.StatusBadRequest, "Invalid "+key)
		return nil, false
	}
	value := values[0]
	return &value, true
}

func parseQueryInt(r *http.Request, key string, defaultValue int64) (int64, error) {
	values, present := r.URL.Query()[key]
	if !present {
		return defaultValue, nil
	}
	if len(values) != 1 || values[0] == "" {
		return 0, errors.New("invalid integer")
	}
	return strconv.ParseInt(values[0], 10, 64)
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidPagination):
		writeError(w, http.StatusBadRequest, "Invalid pagination parameters")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "Audit log not found")
	default:
		writeError(w, http.StatusInternalServerError, "Internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	response.WriteJSON(w, status, response.Response{Success: false, Message: message})
}

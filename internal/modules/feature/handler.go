package feature

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

var featureUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var request CreateRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeFeatureError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	result, err := h.service.Create(r.Context(), request)
	if err != nil {
		writeFeatureServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, response.Response{Success: true, Data: result})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	page, err := queryInt(r, "page", 1)
	if err != nil {
		writeFeatureError(w, http.StatusBadRequest, "page must be a positive integer")
		return
	}
	limit, err := queryInt(r, "limit", 20)
	if err != nil || limit > 25 {
		writeFeatureError(w, http.StatusBadRequest, "limit must be between 1 and 25")
		return
	}
	result, err := h.service.List(r.Context(), page, limit, r.URL.Query().Get("type"))
	if err != nil {
		writeFeatureServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: result})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := featureID(w, r)
	if !ok {
		return
	}
	item, err := h.service.FindByID(r.Context(), id)
	if err != nil {
		writeFeatureServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: item})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := featureID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name        json.RawMessage `json:"name"`
		Description json.RawMessage `json:"description"`
	}
	if err := decodeStrictJSON(r, &body); err != nil {
		writeFeatureError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	var name string
	nameSet := body.Name != nil
	if nameSet {
		if string(body.Name) == "null" || json.Unmarshal(body.Name, &name) != nil {
			writeFeatureError(w, http.StatusBadRequest, "name must be a string")
			return
		}
	}
	var description *string
	descriptionSet := body.Description != nil
	if descriptionSet && string(body.Description) != "null" {
		var value string
		if err := json.Unmarshal(body.Description, &value); err != nil {
			writeFeatureError(w, http.StatusBadRequest, "description must be a string or null")
			return
		}
		description = &value
	}
	item, err := h.service.Update(r.Context(), id, name, nameSet, description, descriptionSet)
	if err != nil {
		writeFeatureServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: item})
}

func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	id, ok := featureID(w, r)
	if !ok {
		return
	}
	if err := h.service.Archive(r.Context(), id); err != nil {
		writeFeatureServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Message: "Feature archived successfully"})
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

func queryInt(r *http.Request, key string, defaultValue int64) (int64, error) {
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

func featureID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !featureUUIDPattern.MatchString(id) {
		writeFeatureError(w, http.StatusBadRequest, "Invalid feature ID")
		return "", false
	}
	return id, true
}

func writeFeatureServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeFeatureError(w, http.StatusNotFound, "Feature not found")
	case errors.Is(err, ErrDuplicateKey):
		writeFeatureError(w, http.StatusConflict, "Feature key already exists")
	case errors.Is(err, ErrEnvironmentsState):
		writeFeatureError(w, http.StatusConflict, "All nine expected environments must exist and be active")
	case errors.Is(err, ErrInvalidKey):
		writeFeatureError(w, http.StatusBadRequest, "key must be lowercase snake_case")
	case errors.Is(err, ErrInvalidName):
		writeFeatureError(w, http.StatusBadRequest, "name must not be empty and must be at most 255 characters")
	case errors.Is(err, ErrInvalidType):
		writeFeatureError(w, http.StatusBadRequest, "Unsupported feature type")
	case errors.Is(err, ErrEmptyPatch):
		writeFeatureError(w, http.StatusBadRequest, "PATCH must include name or description")
	case errors.Is(err, ErrInvalidPage):
		writeFeatureError(w, http.StatusBadRequest, "Invalid pagination parameters")
	default:
		writeFeatureError(w, http.StatusInternalServerError, "Internal server error")
	}
}

func writeFeatureError(w http.ResponseWriter, status int, message string) {
	response.WriteJSON(w, status, response.Response{Success: false, Message: message})
}

package usergroup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"

	"flag_backend/internal/modules/configuration"
	"flag_backend/internal/platform/response"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        json.RawMessage `json:"name"`
		Description json.RawMessage `json:"description"`
	}
	if err := decodeStrictJSON(r, &body); err != nil || body.Name == nil {
		writeError(w, http.StatusBadRequest, "Name is required")
		return
	}
	var name string
	if err := json.Unmarshal(body.Name, &name); err != nil {
		writeError(w, http.StatusBadRequest, "Name must be a string")
		return
	}
	description, ok := parseDescription(body.Description)
	if !ok {
		writeError(w, http.StatusBadRequest, "Description must be a string or null")
		return
	}
	group, err := h.service.Create(r.Context(), name, description)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, response.Response{Success: true, Data: group})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	groups, err := h.service.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if groups == nil {
		groups = make([]GroupSummary, 0)
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: groups})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id", "Invalid group ID")
	if !ok {
		return
	}
	group, err := h.service.Find(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: group})
}

func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id", "Invalid group ID")
	if !ok {
		return
	}
	var body struct {
		Name        json.RawMessage `json:"name"`
		Description json.RawMessage `json:"description"`
	}
	if err := decodeStrictJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	patch := GroupPatch{NameSet: body.Name != nil, DescriptionSet: body.Description != nil}
	if patch.NameSet {
		if json.Unmarshal(body.Name, &patch.Name) != nil {
			writeError(w, http.StatusBadRequest, "Name must be a string")
			return
		}
	}
	if patch.DescriptionSet {
		var valid bool
		patch.Description, valid = parseDescription(body.Description)
		if !valid {
			writeError(w, http.StatusBadRequest, "Description must be a string or null")
			return
		}
	}
	group, err := h.service.Patch(r.Context(), id, patch)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Updated     bool    `json:"updated"`
	}{group.ID, group.Name, group.Description, true}})
}

func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id", "Invalid group ID")
	if !ok {
		return
	}
	if err := h.service.Archive(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Message: "User group archived successfully"})
}

func (h *Handler) GetEffective(w http.ResponseWriter, r *http.Request) {
	environmentID, ok := pathUUID(w, r, "environmentId", "Invalid environment ID")
	if !ok {
		return
	}
	featureID, ok := pathUUID(w, r, "featureId", "Invalid feature ID")
	if !ok {
		return
	}
	result, err := h.service.Effective(r.Context(), environmentID, featureID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: result})
}

func (h *Handler) PutRule(w http.ResponseWriter, r *http.Request) {
	environmentID, ok := pathUUID(w, r, "environmentId", "Invalid environment ID")
	if !ok {
		return
	}
	featureID, ok := pathUUID(w, r, "featureId", "Invalid feature ID")
	if !ok {
		return
	}
	groupID, ok := pathUUID(w, r, "groupId", "Invalid group ID")
	if !ok {
		return
	}
	var body struct {
		Config json.RawMessage `json:"config"`
	}
	if err := decodeStrictJSON(r, &body); err != nil || body.Config == nil {
		writeError(w, http.StatusBadRequest, "Config is required")
		return
	}
	rule, err := h.service.SetRule(r.Context(), environmentID, featureID, groupID, body.Config)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: rule})
}

func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	environmentID, ok := pathUUID(w, r, "environmentId", "Invalid environment ID")
	if !ok {
		return
	}
	featureID, ok := pathUUID(w, r, "featureId", "Invalid feature ID")
	if !ok {
		return
	}
	groupID, ok := pathUUID(w, r, "groupId", "Invalid group ID")
	if !ok {
		return
	}
	if err := h.service.DeleteRule(r.Context(), environmentID, featureID, groupID); err != nil {
		writeServiceError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Message: "Feature group rollout rule removed successfully"})
}

func pathUUID(w http.ResponseWriter, r *http.Request, key, message string) (string, bool) {
	id := r.PathValue(key)
	if !uuidPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, message)
		return "", false
	}
	return id, true
}

func parseDescription(raw json.RawMessage) (*string, bool) {
	if raw == nil || string(bytes.TrimSpace(raw)) == "null" {
		return nil, true
	}
	var description string
	if json.Unmarshal(raw, &description) != nil {
		return nil, false
	}
	return &description, true
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
	case errors.Is(err, ErrGroupNotFound):
		writeError(w, http.StatusNotFound, "User group not found")
	case errors.Is(err, ErrEnvironmentNotFound):
		writeError(w, http.StatusNotFound, "Environment not found")
	case errors.Is(err, ErrFeatureNotFound):
		writeError(w, http.StatusNotFound, "Feature not found")
	case errors.Is(err, ErrDefaultConfigurationNotFound):
		writeError(w, http.StatusNotFound, "Environment configuration not found")
	case errors.Is(err, ErrRuleNotFound):
		writeError(w, http.StatusNotFound, "Feature group rollout rule not found")
	case errors.Is(err, ErrDuplicateName):
		writeError(w, http.StatusConflict, "User group name already exists")
	case errors.Is(err, ErrGroupArchived):
		writeError(w, http.StatusConflict, "Archived user groups cannot be updated")
	case errors.Is(err, ErrAlreadyArchived):
		writeError(w, http.StatusConflict, "User group is already archived")
	case errors.Is(err, ErrInvalidName):
		writeError(w, http.StatusBadRequest, "Name must not be empty and must be at most 100 characters")
	case errors.Is(err, ErrEmptyPatch):
		writeError(w, http.StatusBadRequest, "PATCH must include name or description")
	case errors.Is(err, configuration.ErrInvalidConfig):
		writeError(w, http.StatusBadRequest, "Configuration does not match feature type")
	default:
		writeError(w, http.StatusInternalServerError, "Internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	response.WriteJSON(w, status, response.Response{Success: false, Message: message})
}

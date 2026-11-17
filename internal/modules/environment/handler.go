package environment

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"flag_backend/internal/platform/response"
)

type Handler struct {
	service *Service
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		response.WriteJSON(w, http.StatusBadRequest, response.Response{Success: false, Message: "Invalid environment ID"})
		return "", false
	}
	return id, true
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.WriteJSON(w, http.StatusMethodNotAllowed, response.Response{Success: false, Message: "Method not allowed"})
		return
	}
	environments, err := h.service.List(r.Context())
	if err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, response.Response{Success: false, Message: "Internal server error"})
		return
	}
	type listItem struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Platform  string    `json:"platform"`
		Type      string    `json:"type"`
		CreatedAt time.Time `json:"created_at"`
	}
	items := make([]listItem, 0, len(environments))
	for _, environment := range environments {
		items = append(items, listItem{environment.ID, environment.Name, environment.Platform, environment.Type, environment.CreatedAt})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		Success bool       `json:"success"`
		Data    []listItem `json:"data"`
	}{Success: true, Data: items})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.WriteJSON(w, http.StatusMethodNotAllowed, response.Response{Success: false, Message: "Method not allowed"})
		return
	}
	id, ok := validID(w, r)
	if !ok {
		return
	}
	environment, err := h.service.FindByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		response.WriteJSON(w, http.StatusNotFound, response.Response{Success: false, Message: "Environment not found"})
		return
	}
	if err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, response.Response{Success: false, Message: "Internal server error"})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		Success bool                `json:"success"`
		Data    EnvironmentResponse `json:"data"`
	}{Success: true, Data: toResponse(environment)})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", http.MethodPatch)
		response.WriteJSON(w, http.StatusMethodNotAllowed, response.Response{Success: false, Message: "Method not allowed"})
		return
	}
	id, ok := validID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		response.WriteJSON(w, http.StatusBadRequest, response.Response{Success: false, Message: "Invalid request body"})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.WriteJSON(w, http.StatusBadRequest, response.Response{Success: false, Message: "Invalid request body"})
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		response.WriteJSON(w, http.StatusBadRequest, response.Response{Success: false, Message: "Name must not be empty"})
		return
	}
	environment, err := h.service.UpdateName(r.Context(), id, body.Name)
	if errors.Is(err, ErrNotFound) {
		response.WriteJSON(w, http.StatusNotFound, response.Response{Success: false, Message: "Environment not found"})
		return
	}
	if errors.Is(err, ErrDuplicateName) {
		response.WriteJSON(w, http.StatusConflict, response.Response{Success: false, Message: "Environment name already exists"})
		return
	}
	if err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, response.Response{Success: false, Message: "Internal server error"})
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Data: struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Platform string `json:"platform"`
		Type     string `json:"type"`
		Updated  bool   `json:"updated"`
	}{environment.ID, environment.Name, environment.Platform, environment.Type, true}})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		response.WriteJSON(w, http.StatusMethodNotAllowed, response.Response{Success: false, Message: "Method not allowed"})
		return
	}
	id, ok := validID(w, r)
	if !ok {
		return
	}
	if err := h.service.Archive(r.Context(), id); errors.Is(err, ErrNotFound) {
		response.WriteJSON(w, http.StatusNotFound, response.Response{Success: false, Message: "Environment not found"})
		return
	} else if err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, response.Response{Success: false, Message: "Internal server error"})
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Message: "Environment archived"})
}

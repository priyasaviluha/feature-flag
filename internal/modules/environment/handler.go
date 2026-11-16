package environment

import (
	"encoding/json"
	"errors"
	"net/http"

	"flag_backend/internal/platform/response"
)

type Handler struct {
	service *Service
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
	items := make([]EnvironmentResponse, 0, len(environments))
	for _, environment := range environments {
		items = append(items, toResponse(environment))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		Success bool                  `json:"success"`
		Data    []EnvironmentResponse `json:"data"`
	}{Success: true, Data: items})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		response.WriteJSON(w, http.StatusMethodNotAllowed, response.Response{Success: false, Message: "Method not allowed"})
		return
	}
	environment, err := h.service.FindByID(r.Context(), r.PathValue("id"))
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

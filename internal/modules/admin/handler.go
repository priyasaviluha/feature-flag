package admin

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"flag_backend/internal/middleware"
	"flag_backend/internal/platform/response"
)

type Handler struct {
	service      *Service
	cookieSecure bool
}

func NewHandler(service *Service, cookieSecure bool) *Handler {
	return &Handler{service: service, cookieSecure: cookieSecure}
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.WriteJSON(w, http.StatusMethodNotAllowed, response.Response{Success: false, Message: "Method not allowed"})
		return
	}
	expiredAt := time.Unix(1, 0)
	http.SetCookie(w, &http.Cookie{Name: "admin_token", Value: "", Path: "/", HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode, Expires: expiredAt, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "csrf_token", Value: "", Path: "/", HttpOnly: false, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode, Expires: expiredAt, MaxAge: -1})
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Message: "Logout successful"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	adminID, ok := middleware.AdminID(r.Context())
	if !ok {
		response.WriteJSON(w, http.StatusUnauthorized, response.Response{Success: false, Message: "Unauthorized"})
		return
	}
	admin, err := h.service.GetByID(r.Context(), adminID)
	if err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, response.Response{Success: false, Message: "Internal server error"})
		return
	}
	response.WriteJSON(w, http.StatusOK, response.Response{
		Success: true,
		Message: "Admin profile retrieved",
		Data:    AdminProfileResponse{Email: admin.Email},
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		response.WriteJSON(w, http.StatusMethodNotAllowed, response.Response{Success: false, Message: "Method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request LoginRequest
	if err := decoder.Decode(&request); err != nil {
		response.WriteJSON(w, http.StatusBadRequest, response.Response{Success: false, Message: "Invalid request body"})
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		response.WriteJSON(w, http.StatusBadRequest, response.Response{Success: false, Message: "Invalid request body"})
		return
	}
	if strings.TrimSpace(request.Email) == "" || request.Password == "" {
		response.WriteJSON(w, http.StatusBadRequest, response.Response{Success: false, Message: "Email and password are required"})
		return
	}
	result, err := h.service.Login(r.Context(), request.Email, request.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		response.WriteJSON(w, http.StatusUnauthorized, response.Response{Success: false, Message: "Invalid email or password"})
		return
	}
	if err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, response.Response{Success: false, Message: "Internal server error"})
		return
	}
	csrfBytes := make([]byte, 32)
	if _, err := rand.Read(csrfBytes); err != nil {
		response.WriteJSON(w, http.StatusInternalServerError, response.Response{Success: false, Message: "Internal server error"})
		return
	}
	maxAge := int(time.Until(result.ExpiresAt).Seconds())
	http.SetCookie(w, &http.Cookie{Name: "admin_token", Value: result.Token, Path: "/", HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode, Expires: result.ExpiresAt, MaxAge: maxAge})
	http.SetCookie(w, &http.Cookie{Name: "csrf_token", Value: base64.RawURLEncoding.EncodeToString(csrfBytes), Path: "/", HttpOnly: false, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode, Expires: result.ExpiresAt, MaxAge: maxAge})
	response.WriteJSON(w, http.StatusOK, response.Response{Success: true, Message: "Login successful"})
}

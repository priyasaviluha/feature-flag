// Package response contains the shared JSON response format for HTTP modules.
package response

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, response Response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

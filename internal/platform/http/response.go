package http

import (
	"net/http"

	"flag_backend/internal/platform/response"
)

type Response = response.Response

func WriteJSON(w http.ResponseWriter, status int, value Response) {
	response.WriteJSON(w, status, value)
}

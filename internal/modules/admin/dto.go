package admin

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type AdminProfileResponse struct {
	Email string `json:"email"`
}

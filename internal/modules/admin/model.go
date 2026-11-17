package admin

import "time"

// Admin maps the existing admin table. PasswordHash is never part of an API DTO.
type Admin struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

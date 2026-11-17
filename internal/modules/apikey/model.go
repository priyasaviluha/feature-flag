package apikey

import "time"

type APIKey struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	KeyPrefix     string     `json:"key_prefix"`
	EnvironmentID string     `json:"environment_id"`
	Environment   string     `json:"environment"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	LastUsedAt    *time.Time `json:"last_used_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
}

type CreatedAPIKey struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	EnvironmentID string    `json:"environment_id"`
	Environment   string    `json:"environment"`
	APIKey        string    `json:"api_key"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type Pagination struct {
	Page       int64 `json:"page"`
	Limit      int64 `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

type ListResult struct {
	Data       []APIKey   `json:"data"`
	Pagination Pagination `json:"pagination"`
}

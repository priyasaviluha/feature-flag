package configuration

import (
	"encoding/json"
	"time"
)

type Configuration struct {
	FeatureKey string          `json:"feature_key"`
	Type       string          `json:"type"`
	Config     json.RawMessage `json:"config"`
	Version    int64           `json:"version"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type Pagination struct {
	Page  int64 `json:"page"`
	Limit int64 `json:"limit"`
	Total int64 `json:"total"`
}

type ListResult struct {
	Data       []Configuration `json:"data"`
	Pagination Pagination      `json:"pagination"`
}

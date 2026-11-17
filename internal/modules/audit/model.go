package audit

import (
	"encoding/json"
	"time"
)

type AuditLog struct {
	ID            string          `json:"id"`
	EnvironmentID string          `json:"environment_id"`
	FeatureID     string          `json:"feature_id"`
	Action        string          `json:"action"`
	OldValue      json.RawMessage `json:"old_value"`
	NewValue      json.RawMessage `json:"new_value"`
	CreatedAt     time.Time       `json:"created_at"`
}

type Pagination struct {
	Page  int64 `json:"page"`
	Limit int64 `json:"limit"`
	Total int64 `json:"total"`
}

type ListResult struct {
	Data       []AuditLog `json:"data"`
	Pagination Pagination `json:"pagination"`
}

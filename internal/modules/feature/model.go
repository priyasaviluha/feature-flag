package feature

import "time"

const (
	TypeBoolean    = "BOOLEAN"
	TypeNumber     = "NUMBER"
	TypeThreshold  = "THRESHOLD"
	TypePercentage = "PERCENTAGE"
)

type Feature struct {
	ID          string     `json:"id"`
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Description *string    `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	ArchivedAt  *time.Time `json:"archived_at"`
}

type EnvironmentRef struct {
	ID       string `json:"environment_id"`
	Platform string `json:"platform"`
	Type     string `json:"environment_type"`
}

type Pagination struct {
	Page  int64 `json:"page"`
	Limit int64 `json:"limit"`
	Total int64 `json:"total"`
}

type FeatureSummary struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type ListResult struct {
	Data       []FeatureSummary `json:"data"`
	Pagination Pagination       `json:"pagination"`
}

type CreateResult struct {
	ID                    string           `json:"id"`
	Key                   string           `json:"key"`
	Name                  string           `json:"name"`
	Type                  string           `json:"type"`
	Description           *string          `json:"description"`
	CreatedAt             time.Time        `json:"created_at"`
	ConfigurationsCreated []EnvironmentRef `json:"configurations_created"`
}

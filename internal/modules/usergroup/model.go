package usergroup

import (
	"encoding/json"
	"time"
)

type Group struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	ArchivedAt  *time.Time `json:"archived_at"`
}

type GroupSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type GroupPatch struct {
	Name           string
	NameSet        bool
	Description    *string
	DescriptionSet bool
}

type EnvironmentFeatureGroups struct {
	FeatureKey  string            `json:"feature_key"`
	Environment string            `json:"environment"`
	Groups      []EffectiveConfig `json:"groups"`
}

type EffectiveConfig struct {
	GroupID    string          `json:"group_id"`
	GroupName  string          `json:"group_name"`
	Config     json.RawMessage `json:"config"`
	Version    int64           `json:"version"`
	IsOverride bool            `json:"is_override"`
}

type GroupRule struct {
	FeatureKey  string          `json:"feature_key"`
	Environment string          `json:"environment"`
	Group       GroupIdentity   `json:"group"`
	Config      json.RawMessage `json:"config"`
	Version     int64           `json:"version"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type GroupIdentity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type EnvironmentIdentity struct {
	ID   string
	Name string
}

type FeatureIdentity struct {
	ID   string
	Key  string
	Type string
}

type RuleVersion struct {
	Version   int64
	UpdatedAt time.Time
}

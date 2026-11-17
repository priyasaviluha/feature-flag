package environment

import "time"

// EnvironmentResponse is the public representation returned by environment APIs.
type EnvironmentResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Platform   string     `json:"platform"`
	Type       string     `json:"type"`
	CreatedAt  time.Time  `json:"created_at"`
	ArchivedAt *time.Time `json:"archived_at"`
}

func toResponse(environment Environment) EnvironmentResponse {
	return EnvironmentResponse{
		ID:         environment.ID,
		Name:       environment.Name,
		Platform:   environment.Platform,
		Type:       environment.Type,
		CreatedAt:  environment.CreatedAt,
		ArchivedAt: environment.ArchivedAt,
	}
}

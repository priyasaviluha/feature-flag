package feature

type CreateRequest struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description *string `json:"description"`
}

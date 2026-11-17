package configuration

import "encoding/json"

type UpdateRequest struct {
	Config json.RawMessage `json:"config"`
}

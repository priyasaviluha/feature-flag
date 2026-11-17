package configuration

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateConfigMatchesSupportedFeatureShapes(t *testing.T) {
	tests := []struct {
		name, featureType, config string
		valid                     bool
	}{
		{"boolean", "BOOLEAN", `{"enabled":true}`, true},
		{"boolean whitespace", "BOOLEAN", `{"enabled": true }`, true},
		{"number", "NUMBER", `{"value":12.5}`, true},
		{"legacy threshold", "THRESHOLD", `{"operator":">=","threshold":15,"context_key":"cart_value"}`, true},
		{"enabled threshold", "THRESHOLD", `{"enabled":true,"operator":"=","threshold":15,"context_key":"cart_value"}`, true},
		{"disabled threshold", "THRESHOLD", `{"enabled":false,"operator":"<=","threshold":15,"context_key":null}`, true},
		{"percentage", "PERCENTAGE", `{"enabled":true,"percentage":100}`, true},
		{"percentage exponent", "PERCENTAGE", `{"enabled":true,"percentage":1e2}`, true},
		{"percentage extreme exponent", "PERCENTAGE", `{"enabled":true,"percentage":1e999999}`, false},
		{"boolean wrong field", "BOOLEAN", `{"value":true}`, false},
		{"number string", "NUMBER", `{"value":"12"}`, false},
		{"legacy threshold operator", "THRESHOLD", `{"operator":"~","threshold":15,"context_key":"x"}`, false},
		{"enabled threshold empty context", "THRESHOLD", `{"enabled":true,"operator":">=","threshold":15,"context_key":""}`, false},
		{"percentage below range", "PERCENTAGE", `{"enabled":false,"percentage":-1}`, false},
		{"percentage above range", "PERCENTAGE", `{"enabled":false,"percentage":100.01}`, false},
		{"extra key", "BOOLEAN", `{"enabled":true,"extra":1}`, false},
		{"unknown feature type", "UNKNOWN", `{}`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateConfig(test.featureType, json.RawMessage(test.config))
			if test.valid && err != nil {
				t.Fatalf("ValidateConfig() error = %v", err)
			}
			if !test.valid && !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("ValidateConfig() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

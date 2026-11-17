package configuration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

var (
	ErrInvalidPagination = errors.New("invalid pagination")
	ErrInvalidConfig     = errors.New("configuration does not match feature type")
)

// ValidateConfig mirrors the database validation function used by environment
// configurations. Group rollout rules use the same shapes but do not currently
// have that trigger attached in the deployed schema.
func ValidateConfig(featureType string, config json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(config, &fields); err != nil || fields == nil {
		return ErrInvalidConfig
	}

	boolean := func(raw json.RawMessage) bool {
		var value bool
		trimmed := bytes.TrimSpace(raw)
		return len(trimmed) > 0 && json.Unmarshal(trimmed, &value) == nil && (string(trimmed) == "true" || string(trimmed) == "false")
	}
	stringValue := func(raw json.RawMessage) bool {
		var value string
		return len(raw) > 0 && json.Unmarshal(raw, &value) == nil
	}
	keysAre := func(want ...string) bool {
		if len(fields) != len(want) {
			return false
		}
		for _, key := range want {
			if _, ok := fields[key]; !ok {
				return false
			}
		}
		return true
	}

	switch featureType {
	case "BOOLEAN":
		if keysAre("enabled") && boolean(fields["enabled"]) {
			return nil
		}
	case "NUMBER":
		if keysAre("value") && isNumber(fields["value"]) {
			return nil
		}
	case "THRESHOLD":
		if !fieldsHas(fields, "enabled") {
			if keysAre("operator", "threshold", "context_key") &&
				oneOfString(fields["operator"], ">", ">=", "<", "<=", "=", "!=") &&
				isNumber(fields["threshold"]) && stringValue(fields["context_key"]) {
				return nil
			}
		} else {
			var enabled bool
			if !keysAre("enabled", "operator", "threshold", "context_key") ||
				!boolean(fields["enabled"]) || json.Unmarshal(fields["enabled"], &enabled) != nil ||
				!oneOfString(fields["operator"], ">=", "<=", "=") || !isNumber(fields["threshold"]) {
				break
			}
			if enabled && stringValue(fields["context_key"]) {
				var contextKey string
				if json.Unmarshal(fields["context_key"], &contextKey) == nil && contextKey != "" {
					return nil
				}
			} else if !enabled && (stringValue(fields["context_key"]) || string(bytes.TrimSpace(fields["context_key"])) == "null") {
				return nil
			}
		}
	case "PERCENTAGE":
		if keysAre("enabled", "percentage") && boolean(fields["enabled"]) && percentageInRange(fields["percentage"]) {
			return nil
		}
	}
	return ErrInvalidConfig
}

func fieldsHas(fields map[string]json.RawMessage, key string) bool {
	_, ok := fields[key]
	return ok
}

func oneOfString(value json.RawMessage, choices ...string) bool {
	var actual string
	if json.Unmarshal(value, &actual) != nil {
		return false
	}
	for _, choice := range choices {
		if actual == choice {
			return true
		}
	}
	return false
}

func isNumber(raw json.RawMessage) bool {
	_, ok := parseNumber(raw)
	return ok
}

func parseNumber(raw json.RawMessage) (json.Number, bool) {
	if len(raw) == 0 {
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return "", false
	}
	number, ok := value.(json.Number)
	return number, ok
}

func percentageInRange(raw json.RawMessage) bool {
	value, ok := parseNumber(raw)
	if !ok {
		return false
	}
	// PostgreSQL numeric values have bounded precision and scale. Check the
	// exponent before constructing a rational so an extreme JSON exponent cannot
	// cause an unbounded allocation.
	text := value.String()
	if exponentAt := strings.IndexAny(text, "eE"); exponentAt >= 0 {
		exponent, err := strconv.ParseInt(text[exponentAt+1:], 10, 32)
		if err != nil || exponent > 16383 || exponent < -16383 {
			return false
		}
	}
	rational, ok := new(big.Rat).SetString(text)
	return ok && rational.Cmp(big.NewRat(0, 1)) >= 0 && rational.Cmp(big.NewRat(100, 1)) <= 0
}

const MaxLimit int64 = 100

type Store interface {
	EnvironmentExists(context.Context, string) error
	List(context.Context, string, int64, int64) ([]Configuration, int64, error)
	Find(context.Context, string, string) (Configuration, error)
	Begin(context.Context) (UpdateTx, error)
}

type Service struct{ repository Store }

func NewService(repository Store) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, environmentID string, page, limit int64) (ListResult, error) {
	if !validPagination(page, limit) {
		return ListResult{}, ErrInvalidPagination
	}
	if err := s.repository.EnvironmentExists(ctx, environmentID); err != nil {
		return ListResult{}, err
	}
	items, total, err := s.repository.List(ctx, environmentID, page, limit)
	if err != nil {
		return ListResult{}, err
	}
	if items == nil {
		items = make([]Configuration, 0)
	}
	return ListResult{Data: items, Pagination: Pagination{Page: page, Limit: limit, Total: total}}, nil
}

func (s *Service) Find(ctx context.Context, environmentID, featureKey string) (Configuration, error) {
	if err := s.repository.EnvironmentExists(ctx, environmentID); err != nil {
		return Configuration{}, err
	}
	return s.repository.Find(ctx, environmentID, featureKey)
}

func (s *Service) Update(ctx context.Context, environmentID, featureKey string, config json.RawMessage) (result Configuration, returnErr error) {
	tx, err := s.repository.Begin(ctx)
	if err != nil {
		return Configuration{}, err
	}
	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(context.Background()); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("rollback configuration update: %w", err))
			}
		}
	}()

	if err := tx.EnvironmentExists(ctx, environmentID); err != nil {
		return Configuration{}, err
	}
	featureID, featureType, err := tx.FindFeature(ctx, featureKey)
	if err != nil {
		return Configuration{}, err
	}
	oldConfig, err := tx.LockConfiguration(ctx, environmentID, featureID)
	if err != nil {
		return Configuration{}, err
	}
	version, updatedAt, err := tx.UpdateConfiguration(ctx, environmentID, featureID, config)
	if err != nil {
		return Configuration{}, err
	}
	if err := tx.InsertAudit(ctx, environmentID, featureID, oldConfig, config); err != nil {
		return Configuration{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Configuration{}, fmt.Errorf("commit configuration update: %w", err)
	}
	committed = true
	return Configuration{FeatureKey: featureKey, Type: featureType, Config: config, Version: version, UpdatedAt: updatedAt}, nil
}

func validPagination(page, limit int64) bool {
	return page > 0 && limit > 0 && limit <= MaxLimit && page-1 <= (int64(^uint64(0)>>1)-limit)/limit
}

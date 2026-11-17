package feature

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidKey  = errors.New("feature key must be lowercase snake_case")
	ErrInvalidName = errors.New("feature name must not be empty")
	ErrInvalidType = errors.New("unsupported feature type")
	ErrEmptyPatch  = errors.New("patch must include name or description")
	ErrInvalidPage = errors.New("invalid pagination")
)

type FeatureTx interface {
	ActiveEnvironments(context.Context) ([]EnvironmentRef, error)
	InsertFeature(context.Context, string, string, string, *string) (Feature, error)
	InsertDefaultConfigurations(context.Context, string, []byte, []EnvironmentRef) error
	Commit(context.Context) error
	Rollback(context.Context) error
}

type Store interface {
	Begin(context.Context) (FeatureTx, error)
	List(context.Context, int64, int64, string) ([]Feature, int64, error)
	FindByID(context.Context, string) (Feature, error)
	Update(context.Context, string, string, bool, *string, bool) (Feature, error)
	Archive(context.Context, string) error
}

type Service struct{ repository Store }

func NewService(repository Store) *Service { return &Service{repository: repository} }

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)

func (s *Service) Create(ctx context.Context, request CreateRequest) (result CreateResult, returnErr error) {
	request.Name = strings.TrimSpace(request.Name)
	if !keyPattern.MatchString(request.Key) || utf8.RuneCountInString(request.Key) > 150 {
		return CreateResult{}, ErrInvalidKey
	}
	if request.Name == "" || utf8.RuneCountInString(request.Name) > 255 {
		return CreateResult{}, ErrInvalidName
	}
	if !validType(request.Type) {
		return CreateResult{}, ErrInvalidType
	}
	defaultConfig, err := defaultConfiguration(request.Type)
	if err != nil {
		return CreateResult{}, err
	}

	tx, err := s.repository.Begin(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(context.Background()); err != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("rollback feature creation: %w", err))
			}
		}
	}()

	environments, err := tx.ActiveEnvironments(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	if !hasAllExpectedEnvironments(environments) {
		return CreateResult{}, ErrEnvironmentsState
	}
	item, err := tx.InsertFeature(ctx, request.Key, request.Name, request.Type, request.Description)
	if err != nil {
		return CreateResult{}, err
	}
	if err := tx.InsertDefaultConfigurations(ctx, item.ID, defaultConfig, environments); err != nil {
		return CreateResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, fmt.Errorf("commit feature creation: %w", err)
	}
	committed = true
	return CreateResult{
		ID: item.ID, Key: item.Key, Name: item.Name, Type: item.Type,
		Description: item.Description, CreatedAt: item.CreatedAt,
		ConfigurationsCreated: environments,
	}, nil
}

func (s *Service) List(ctx context.Context, page, limit int64, featureType string) (ListResult, error) {
	if page < 1 || limit < 1 || limit > 25 || page-1 > (int64(^uint64(0)>>1)-limit)/limit {
		return ListResult{}, ErrInvalidPage
	}
	if featureType != "" && !validType(featureType) {
		return ListResult{}, ErrInvalidType
	}
	features, total, err := s.repository.List(ctx, page, limit, featureType)
	if err != nil {
		return ListResult{}, err
	}
	items := make([]FeatureSummary, 0, len(features))
	for _, item := range features {
		items = append(items, FeatureSummary{ID: item.ID, Key: item.Key, Name: item.Name, Type: item.Type, Description: item.Description, CreatedAt: item.CreatedAt})
	}
	return ListResult{Data: items, Pagination: Pagination{Page: page, Limit: limit, Total: total}}, nil
}

func (s *Service) FindByID(ctx context.Context, id string) (Feature, error) {
	return s.repository.FindByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id string, name string, nameSet bool, description *string, descriptionSet bool) (Feature, error) {
	if !nameSet && !descriptionSet {
		return Feature{}, ErrEmptyPatch
	}
	if nameSet {
		name = strings.TrimSpace(name)
		if name == "" || utf8.RuneCountInString(name) > 255 {
			return Feature{}, ErrInvalidName
		}
	}
	return s.repository.Update(ctx, id, name, nameSet, description, descriptionSet)
}

func (s *Service) Archive(ctx context.Context, id string) error { return s.repository.Archive(ctx, id) }

func validType(featureType string) bool {
	switch featureType {
	case TypeBoolean, TypeNumber, TypeThreshold, TypePercentage:
		return true
	default:
		return false
	}
}

func defaultConfiguration(featureType string) ([]byte, error) {
	var value any
	switch featureType {
	case TypeBoolean:
		value = struct {
			Enabled bool `json:"enabled"`
		}{false}
	case TypeNumber:
		value = struct {
			Value int `json:"value"`
		}{0}
	case TypeThreshold:
		value = struct {
			Enabled    bool    `json:"enabled"`
			Operator   string  `json:"operator"`
			Threshold  int     `json:"threshold"`
			ContextKey *string `json:"context_key"`
		}{false, ">=", 0, nil}
	case TypePercentage:
		value = struct {
			Enabled    bool `json:"enabled"`
			Percentage int  `json:"percentage"`
		}{false, 0}
	default:
		return nil, ErrInvalidType
	}
	return json.Marshal(value)
}

func hasAllExpectedEnvironments(environments []EnvironmentRef) bool {
	want := map[string]bool{
		"web/development": false, "web/staging": false, "web/production": false,
		"android/development": false, "android/staging": false, "android/production": false,
		"ios/development": false, "ios/staging": false, "ios/production": false,
	}
	for _, environment := range environments {
		key := environment.Platform + "/" + environment.Type
		if _, expected := want[key]; !expected || want[key] {
			return false
		}
		want[key] = true
	}
	for _, found := range want {
		if !found {
			return false
		}
	}
	return true
}

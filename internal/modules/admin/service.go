package admin

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	sharedauth "flag_backend/internal/auth"
	"flag_backend/internal/config"
	"golang.org/x/crypto/argon2"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

type AdminStore interface {
	FindByEmail(context.Context, string) (Admin, error)
	FindByID(context.Context, string) (Admin, error)
}
type AuthenticatedAdmin struct {
	Token     string
	ExpiresAt time.Time
}
type Service struct {
	repository AdminStore
	config     config.Config
}

func NewService(repository AdminStore, cfg config.Config) *Service {
	return &Service{repository: repository, config: cfg}
}

func (s *Service) Login(ctx context.Context, email, password string) (AuthenticatedAdmin, error) {
	admin, err := s.repository.FindByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		if errors.Is(err, ErrAdminNotFound) {
			return AuthenticatedAdmin{}, ErrInvalidCredentials
		}
		return AuthenticatedAdmin{}, fmt.Errorf("load admin for authentication: %w", err)
	}
	if !verifyPassword(password, admin.PasswordHash) {
		return AuthenticatedAdmin{}, ErrInvalidCredentials
	}
	token, expiresAt, err := sharedauth.Sign(admin.ID, []byte(s.config.JWTSecret), s.config.JWTExpiry, time.Now())
	if err != nil {
		return AuthenticatedAdmin{}, fmt.Errorf("create admin token: %w", err)
	}
	return AuthenticatedAdmin{Token: token, ExpiresAt: expiresAt}, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (Admin, error) {
	admin, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return Admin{}, fmt.Errorf("load admin profile: %w", err)
	}
	return admin, nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || !strings.HasPrefix(parts[2], "v=") {
		return false
	}
	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || version != argon2.Version {
		return false
	}
	params := make(map[string]uint32, 3)
	for _, item := range strings.Split(parts[3], ",") {
		pair := strings.SplitN(item, "=", 2)
		if len(pair) != 2 {
			return false
		}
		value, parseErr := strconv.ParseUint(pair[1], 10, 32)
		if parseErr != nil {
			return false
		}
		if _, exists := params[pair[0]]; exists {
			return false
		}
		params[pair[0]] = uint32(value)
	}
	memory, mok := params["m"]
	iterations, tok := params["t"]
	parallelism, pok := params["p"]
	if !mok || !tok || !pok || parallelism == 0 || memory < 8*parallelism || memory > 128*1024 || iterations == 0 || iterations > 5 || parallelism > 8 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, uint8(parallelism), uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

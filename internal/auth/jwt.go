// Package auth contains the shared JWT implementation used by login and middleware.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const Issuer = "flag-backend"

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	Subject  string `json:"sub"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
	Issuer   string `json:"iss"`
}

func Sign(subject string, secret []byte, expiry time.Duration, now time.Time) (string, time.Time, error) {
	if !validUUID(subject) || len(secret) < 32 || expiry <= 0 {
		return "", time.Time{}, fmt.Errorf("invalid JWT signing input")
	}
	expiresAt := now.Add(expiry)
	claims := Claims{Subject: subject, IssuedAt: now.Unix(), Expires: expiresAt.Unix(), Issuer: Issuer}
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("encode JWT claims: %w", err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), expiresAt, nil
}

func Verify(token string, secret []byte, now time.Time) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(secret) < 32 {
		return Claims{}, ErrInvalidToken
	}
	decode := base64.RawURLEncoding.DecodeString
	headerBytes, err := decode(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != "HS256" || header.Type != "JWT" {
		return Claims{}, ErrInvalidToken
	}
	provided, err := decode(parts[2])
	if err != nil || len(provided) != sha256.Size {
		return Claims{}, ErrInvalidToken
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return Claims{}, ErrInvalidToken
	}
	payload, err := decode(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	if json.Unmarshal(payload, &claims) != nil || !validUUID(claims.Subject) || claims.Issuer != Issuer || claims.IssuedAt <= 0 || claims.Expires <= 0 || claims.Expires <= now.Unix() || claims.IssuedAt > now.Add(time.Minute).Unix() || claims.Expires <= claims.IssuedAt {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for i := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		c := value[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

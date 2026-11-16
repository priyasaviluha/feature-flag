//	Load and validate environment variables
//Instead of every module independently reading .env, we'll have one central configuration package.

//Is my application configuration valid?

package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds the application's configuration.
type Config struct {
	DatabaseURL    string
	AdminEmail     string
	AdminPassword  string
	JWTSecret      string
	JWTExpiry      time.Duration
	CookieSecure   bool
	AllowedOrigins []string
}

// Load reads environment variables and validates the configuration.
func Load() (Config, error) {

	// Load environment variables from .env.
	// In production, environment variables can be injected directly.
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found. Using system environment variables.")
	}

	// Read database connection string.
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))

	// Validate that DATABASE_URL exists.
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	// Parse and validate the database URL.
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		return Config{}, fmt.Errorf("invalid DATABASE_URL: %w", err)
	}

	if parsedURL.Scheme != "postgres" &&
		parsedURL.Scheme != "postgresql" {
		return Config{}, fmt.Errorf("DATABASE_URL must use postgres or postgresql scheme")
	}

	if parsedURL.Hostname() == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must contain a database host")
	}

	adminEmail := strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))
	adminPassword := os.Getenv("ADMIN_PASSWORD")

	if adminEmail == "" {
		return Config{}, fmt.Errorf("ADMIN_EMAIL is required")
	}

	if adminPassword == "" {
		return Config{}, fmt.Errorf("ADMIN_PASSWORD is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 bytes")
	}

	jwtExpiry := time.Hour
	if value := strings.TrimSpace(os.Getenv("JWT_EXPIRY")); value != "" {
		jwtExpiry, err = time.ParseDuration(value)
		if err != nil || jwtExpiry <= 0 {
			return Config{}, fmt.Errorf("JWT_EXPIRY must be a positive Go duration")
		}
	}

	cookieSecure := strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production")
	if value := strings.TrimSpace(os.Getenv("COOKIE_SECURE")); value != "" {
		cookieSecure, err = strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("COOKIE_SECURE must be true or false")
		}
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") && !cookieSecure {
		return Config{}, fmt.Errorf("COOKIE_SECURE must be true in production")
	}

	var allowedOrigins []string
	for _, origin := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		parsedOrigin, parseErr := url.Parse(origin)
		if parseErr != nil || (parsedOrigin.Scheme != "http" && parsedOrigin.Scheme != "https") || parsedOrigin.Hostname() == "" || (parsedOrigin.Path != "" && parsedOrigin.Path != "/") || parsedOrigin.RawQuery != "" || parsedOrigin.Fragment != "" || parsedOrigin.User != nil || origin == "*" {
			return Config{}, fmt.Errorf("CORS_ALLOWED_ORIGINS must contain valid HTTP or HTTPS origins")
		}
		allowedOrigins = append(allowedOrigins, strings.TrimSuffix(origin, "/"))
	}

	// Return validated configuration.
	return Config{
		DatabaseURL:    databaseURL,
		AdminEmail:     adminEmail,
		AdminPassword:  adminPassword,
		JWTSecret:      jwtSecret,
		JWTExpiry:      jwtExpiry,
		CookieSecure:   cookieSecure,
		AllowedOrigins: allowedOrigins,
	}, nil
}

//Initialize the application and its dependencies
//But who calls them in the correct order the config then postgres

package bootstrap

import (
	"context"
	"fmt"

	"flag_backend/internal/config"
	"flag_backend/internal/database"

	"crypto/rand"
	"encoding/base64"
	"github.com/jackc/pgx/v5/pgxpool"

	"golang.org/x/crypto/argon2"
)

// App holds the application's initialized dependencies.
// prepares the application.
type App struct {
	DB     *pgxpool.Pool
	Config config.Config
}

// New initializes the application dependencies.
func New(ctx context.Context) (*App, error) {

	// Load application configuration.
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize PostgreSQL connection pool.
	db, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	if err := seedEnvironments(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if err := seedAdmin(ctx, db, cfg); err != nil {
		db.Close()
		return nil, err
	}

	return &App{
		DB:     db,
		Config: cfg,
	}, nil
}

// Close releases application resources.
func (app *App) Close() {
	if app.DB != nil {
		app.DB.Close()
	}
}

func seedEnvironments(ctx context.Context, db *pgxpool.Pool) error {
	query := `
        INSERT INTO environments (name, platform, type)
        VALUES
            ('Web Development', 'web', 'development'),
            ('Web Staging', 'web', 'staging'),
            ('Web Production', 'web', 'production'),
            ('Android Development', 'android', 'development'),
            ('Android Staging', 'android', 'staging'),
            ('Android Production', 'android', 'production'),
            ('iOS Development', 'ios', 'development'),
            ('iOS Staging', 'ios', 'staging'),
            ('iOS Production', 'ios', 'production')
        ON CONFLICT (platform, type) DO NOTHING;
    `

	_, err := db.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to seed environments: %w", err)
	}

	return nil
}

func seedAdmin(ctx context.Context, db *pgxpool.Pool, cfg config.Config) error {

	// Check whether an admin already exists.
	var exists bool

	err := db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1 FROM admin
        )
    `).Scan(&exists)

	if err != nil {
		return fmt.Errorf("failed to check existing admin: %w", err)
	}

	// Never overwrite an existing admin.
	if exists {
		return nil
	}

	// Generate a secure random salt.
	salt := make([]byte, 16)

	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("failed to generate password salt: %w", err)
	}

	// Hash the initial password using Argon2id.
	hash := argon2.IDKey(
		[]byte(cfg.AdminPassword),
		salt,
		3,       // Iterations
		64*1024, // Memory: 64 MB
		2,       // Parallelism
		32,      // Key length
	)

	// Encode salt and hash for database storage.
	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hash)

	passwordHash := fmt.Sprintf(
		"$argon2id$v=19$m=65536,t=3,p=2$%s$%s",
		encodedSalt,
		encodedHash,
	)

	// Insert the initial admin.
	// ON CONFLICT protects against concurrent initialization.
	_, err = db.Exec(ctx, `
        INSERT INTO admin (singleton_id, email, password_hash)
        VALUES (1, $1, $2)
        ON CONFLICT (singleton_id) DO NOTHING
    `, cfg.AdminEmail, passwordHash)

	if err != nil {
		return fmt.Errorf("failed to seed initial admin: %w", err)
	}

	return nil
}

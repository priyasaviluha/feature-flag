// This handles the PostgreSQL connection using pgxpool.
//Establish PostgreSQL connection pool
//Parse PostgreSQL parameters ,Create connection pool,Verify database connectivity

//Can I establish a working PostgreSQL connection using that configuration?

package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New creates and verifies a PostgreSQL connection pool.
func New(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {

	// Parse PostgreSQL connection configuration.
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Configure connection pool.
	poolConfig.MaxConns = 10
	poolConfig.MinConns = 0
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	// Create connection pool.
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify database connectivity.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	return pool, nil
}

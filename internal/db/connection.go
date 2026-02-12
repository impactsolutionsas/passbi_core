package db

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	pool     *pgxpool.Pool
	poolOnce sync.Once
	poolErr  error
)

// Config holds database configuration
type Config struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string
	MinConns int32
	MaxConns int32
}

// LoadConfigFromEnv loads database configuration from environment variables
func LoadConfigFromEnv() *Config {
	port, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	minConns, _ := strconv.Atoi(getEnv("DB_MIN_CONNS", "5"))
	maxConns, _ := strconv.Atoi(getEnv("DB_MAX_CONNS", "20"))

	return &Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     port,
		Database: getEnv("DB_NAME", "passbi"),
		User:     getEnv("DB_USER", "postgres"),
		Password: getEnv("DB_PASSWORD", ""),
		SSLMode:  getEnv("DB_SSLMODE", "disable"),
		MinConns: int32(minConns),
		MaxConns: int32(maxConns),
	}
}

// GetDB returns the global database connection pool (singleton pattern)
func GetDB() (*pgxpool.Pool, error) {
	poolOnce.Do(func() {
		config := LoadConfigFromEnv()
		pool, poolErr = initPool(config)
	})
	return pool, poolErr
}

// InitPoolWithConfig initializes the pool with a custom config (useful for testing)
func InitPoolWithConfig(config *Config) (*pgxpool.Pool, error) {
	return initPool(config)
}

// initPool creates and initializes a new pgxpool.Pool
func initPool(config *Config) (*pgxpool.Pool, error) {
	connString := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		config.Host,
		config.Port,
		config.Database,
		config.User,
		config.Password,
		config.SSLMode,
	)

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("unable to parse connection string: %w", err)
	}

	// Configure connection pool
	poolConfig.MinConns = config.MinConns
	poolConfig.MaxConns = config.MaxConns
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	// Disable prepared statements for Supabase pooler (transaction mode)
	// This prevents "prepared statement already exists" errors
	if config.Port == 6543 {
		poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	return pool, nil
}

// Close closes the database connection pool
func Close() {
	if pool != nil {
		pool.Close()
	}
}

// HealthCheck performs a health check on the database connection
func HealthCheck(ctx context.Context) error {
	db, err := GetDB()
	if err != nil {
		return fmt.Errorf("database connection not initialized: %w", err)
	}

	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Check PostGIS extension
	var postgisVersion string
	err = db.QueryRow(ctx, "SELECT PostGIS_Version()").Scan(&postgisVersion)
	if err != nil {
		return fmt.Errorf("PostGIS not available: %w", err)
	}

	return nil
}

// getEnv retrieves an environment variable with a fallback default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

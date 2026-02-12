package cache

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/passbi/passbi_core/internal/models"
	"github.com/redis/go-redis/v9"
)

var (
	client     *redis.Client
	clientOnce sync.Once
	clientErr  error
)

// Config holds Redis configuration
type Config struct {
	Host     string
	Port     int
	Password string
	DB       int
	TTL      time.Duration
	MutexTTL time.Duration
}

// LoadConfigFromEnv loads Redis configuration from environment variables
func LoadConfigFromEnv() *Config {
	port, _ := strconv.Atoi(getEnv("REDIS_PORT", "6379"))
	db, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	ttl, _ := time.ParseDuration(getEnv("CACHE_TTL", "10m"))
	mutexTTL, _ := time.ParseDuration(getEnv("CACHE_MUTEX_TTL", "5s"))

	return &Config{
		Host:     getEnv("REDIS_HOST", "localhost"),
		Port:     port,
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       db,
		TTL:      ttl,
		MutexTTL: mutexTTL,
	}
}

// GetClient returns the global Redis client (singleton pattern)
func GetClient() (*redis.Client, error) {
	clientOnce.Do(func() {
		config := LoadConfigFromEnv()

		// Configure Redis options
		opts := &redis.Options{
			Addr:         fmt.Sprintf("%s:%d", config.Host, config.Port),
			Password:     config.Password,
			DB:           config.DB,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     10,
			MinIdleConns: 2,
		}

		// Enable TLS if configured (required for Upstash)
		if getEnv("REDIS_TLS_ENABLED", "false") == "true" {
			opts.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
		}

		client = redis.NewClient(opts)

		// Test connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := client.Ping(ctx).Err(); err != nil {
			clientErr = fmt.Errorf("failed to connect to Redis: %w", err)
			return
		}
	})

	return client, clientErr
}

// Close closes the Redis client
func Close() {
	if client != nil {
		client.Close()
	}
}

// RouteKey generates a cache key for a route query
func RouteKey(fromLat, fromLon, toLat, toLon float64, strategy string) string {
	// Create deterministic hash of coordinates
	data := fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", fromLat, fromLon, toLat, toLon)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("route:%x:%s", hash[:8], strategy)
}

// LockKey generates a mutex lock key
func LockKey(routeKey string) string {
	return fmt.Sprintf("lock:%s", routeKey)
}

// GetRoute retrieves a cached route
func GetRoute(ctx context.Context, key string) (*models.Path, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	data, err := client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // cache miss
	}
	if err != nil {
		return nil, err
	}

	var path models.Path
	if err := json.Unmarshal(data, &path); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached path: %w", err)
	}

	return &path, nil
}

// SetRoute caches a route
func SetRoute(ctx context.Context, key string, path *models.Path, ttl time.Duration) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	data, err := json.Marshal(path)
	if err != nil {
		return fmt.Errorf("failed to marshal path: %w", err)
	}

	return client.Set(ctx, key, data, ttl).Err()
}

// AcquireLock attempts to acquire a distributed lock
// Returns true if lock was acquired, false if already locked
func AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	client, err := GetClient()
	if err != nil {
		return false, err
	}

	// Try to set the lock key with NX (only if not exists)
	ok, err := client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, err
	}

	return ok, nil
}

// ReleaseLock releases a distributed lock
func ReleaseLock(ctx context.Context, key string) error {
	client, err := GetClient()
	if err != nil {
		return err
	}

	return client.Del(ctx, key).Err()
}

// WaitForLock waits for a lock to be released and then retrieves the result
// This implements the "wait for result" pattern to avoid thundering herd
func WaitForLock(ctx context.Context, routeKey string, maxWait time.Duration) (*models.Path, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	lockKey := LockKey(routeKey)
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		// Check if lock is released
		exists, err := client.Exists(ctx, lockKey).Result()
		if err != nil {
			return nil, err
		}

		if exists == 0 {
			// Lock released, try to get cached result
			return GetRoute(ctx, routeKey)
		}

		// Wait a bit before checking again
		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("timeout waiting for lock")
}

// HealthCheck performs a health check on the Redis connection
func HealthCheck(ctx context.Context) error {
	client, err := GetClient()
	if err != nil {
		return fmt.Errorf("Redis client not initialized: %w", err)
	}

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis ping failed: %w", err)
	}

	return nil
}

// Stats returns Redis stats
func Stats(ctx context.Context) (map[string]interface{}, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	info, err := client.Info(ctx, "stats").Result()
	if err != nil {
		return nil, err
	}

	poolStats := client.PoolStats()

	return map[string]interface{}{
		"info":       info,
		"hits":       poolStats.Hits,
		"misses":     poolStats.Misses,
		"timeouts":   poolStats.Timeouts,
		"total_conns": poolStats.TotalConns,
		"idle_conns":  poolStats.IdleConns,
		"stale_conns": poolStats.StaleConns,
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// SecurityHeaders ajoute les headers HTTP sécurisés (OWASP recommendations).
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		c.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		// HSTS : activer uniquement si TLS terminé au niveau Go (pas derrière un LB)
		// c.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		return c.Next()
	}
}

// RequestID génère un identifiant unique par requête pour la corrélation des logs.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Get("X-Request-ID")
		if id == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		c.Locals("request_id", id)
		c.Set("X-Request-ID", id)
		return c.Next()
	}
}

// BruteForceProtection bloque les IPs qui échouent trop souvent sur les endpoints d'auth.
// Utilise Redis pour compter les tentatives. Fenêtre glissante de 15 minutes.
func BruteForceProtection(rdb *redis.Client, maxAttempts int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		key := fmt.Sprintf("bf:%s:%s", c.Path(), ip)
		ctx := context.Background()

		// Vérifier si l'IP est bloquée
		blocked, _ := rdb.Exists(ctx, key+":blocked").Result()
		if blocked > 0 {
			ttl, _ := rdb.TTL(ctx, key+":blocked").Result()
			return c.Status(429).JSON(fiber.Map{
				"error":       "too_many_attempts",
				"message":     "Too many failed attempts. Please try again later.",
				"retry_after": int(ttl.Seconds()),
			})
		}

		// Continuer la requête
		err := c.Next()

		// Si la réponse est 401, incrémenter le compteur
		if c.Response().StatusCode() == 401 {
			count, redisErr := rdb.Incr(ctx, key).Result()
			if redisErr == nil {
				rdb.Expire(ctx, key, 15*time.Minute)

				if count >= int64(maxAttempts) {
					// Bloquer l'IP pour 30 minutes
					rdb.Set(ctx, key+":blocked", "1", 30*time.Minute)
					rdb.Del(ctx, key)
					log.Printf("[security] IP blocked: %s on %s (attempts: %d)", ip, c.Path(), count)
				}
			}
		} else if c.Response().StatusCode() == 200 {
			// Succès : réinitialiser le compteur
			rdb.Del(ctx, key)
		}

		return err
	}
}

// SuspiciousPatternDetector détecte les patterns d'attaque courants.
func SuspiciousPatternDetector(db *pgxpool.Pool, rdb *redis.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		ua := c.Get("User-Agent")
		path := c.Path()

		// Détecter les scanners courants
		suspiciousPatterns := []string{
			"sqlmap", "nikto", "nmap", "masscan", "zgrab",
			"python-requests", "go-http-client", "curl/",
		}

		uaLower := strings.ToLower(ua)
		for _, pattern := range suspiciousPatterns {
			if strings.Contains(uaLower, pattern) {
				go logSecurityEvent(db, "suspicious_ua", ip, path, ua)
				// Ne pas bloquer — juste logguer pour analyse
				break
			}
		}

		// Détecter les paths d'injection courants
		rawPath := c.OriginalURL()
		injectionPatterns := []string{
			"../", "..\\", "<script", "javascript:", "union select",
			"' or '", "\" or \"", "exec(", "eval(",
		}
		rawLower := strings.ToLower(rawPath)
		for _, pattern := range injectionPatterns {
			if strings.Contains(rawLower, pattern) {
				go logSecurityEvent(db, "injection_attempt", ip, path, rawPath)
				return c.Status(400).JSON(fiber.Map{
					"error":   "invalid_request",
					"message": "Invalid request",
				})
			}
		}

		return c.Next()
	}
}

// IPRateLimit est un rate limiter global par IP (contre les DDoS de base).
// Distinct du rate limiter par API key.
func IPRateLimit(rdb *redis.Client, reqPerMinute int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		key := fmt.Sprintf("ip_rl:%s:%d", ip, time.Now().Unix()/60)
		ctx := context.Background()

		count, err := rdb.Incr(ctx, key).Result()
		if err == nil {
			rdb.Expire(ctx, key, 2*time.Minute)
			if count > int64(reqPerMinute) {
				c.Set("Retry-After", "60")
				c.Set("X-RateLimit-IP-Limit", strconv.Itoa(reqPerMinute))
				return c.Status(429).JSON(fiber.Map{
					"error":   "ip_rate_limit_exceeded",
					"message": "Too many requests from your IP",
				})
			}
		}

		return c.Next()
	}
}

func logSecurityEvent(db *pgxpool.Pool, eventType, ip, target, meta string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := db.Exec(ctx, `
		INSERT INTO security_event (event_type, source_ip, target, metadata)
		VALUES ($1, $2::inet, $3, $4::jsonb)`,
		eventType, ip, target,
		fmt.Sprintf(`{"detail":%q}`, meta),
	)
	if err != nil {
		log.Printf("[security] failed to log event: %v", err)
	}
}

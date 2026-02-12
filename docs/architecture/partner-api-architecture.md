# Architecture API-as-a-Service pour Partenaires PassBI

## Vue d'ensemble

Cette architecture propose une solution complète pour offrir l'API PassBI en tant que service à des partenaires externes. Elle couvre l'authentification, l'autorisation, le rate limiting, la facturation, et la gestion multi-tenant.

---

## 1. Architecture Globale

```
┌─────────────────────────────────────────────────────────────────┐
│                      PARTENAIRES EXTERNES                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│  │ Partner A│  │ Partner B│  │ Partner C│  │ Partner N│       │
│  │  Web App │  │ Mobile   │  │  Backend │  │   IoT    │       │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘       │
└───────┼─────────────┼─────────────┼─────────────┼──────────────┘
        │             │             │             │
        │ API Key     │ API Key     │ API Key     │ API Key
        │             │             │             │
        ▼             ▼             ▼             ▼
┌─────────────────────────────────────────────────────────────────┐
│                         API GATEWAY                              │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  1. API Key Validation    (Middleware)                    │  │
│  │  2. Rate Limiting          (Redis-based)                  │  │
│  │  3. Quota Management       (Daily/Monthly limits)         │  │
│  │  4. Request Logging        (Analytics)                    │  │
│  │  5. Response Caching       (Partner-specific)             │  │
│  └──────────────────────────────────────────────────────────┘  │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                      PASSBI CORE API                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │ /route-search│  │ /stops/nearby│  │ /routes/list │         │
│  └──────────────┘  └──────────────┘  └──────────────┘         │
└────────────────────────────┬────────────────────────────────────┘
                             │
        ┌────────────────────┼────────────────────┐
        ▼                    ▼                    ▼
  ┌──────────┐        ┌──────────┐        ┌──────────┐
  │PostgreSQL│        │  Redis   │        │ Metrics  │
  │ + PostGIS│        │  Cache   │        │  Store   │
  └──────────┘        └──────────┘        └──────────┘
```

---

## 2. Modèle de Données Partenaires

### 2.1 Table `partner`

```sql
CREATE TABLE partner (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Information de base
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    company VARCHAR(255),

    -- Status
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- active, suspended, inactive
    tier VARCHAR(50) NOT NULL DEFAULT 'free',     -- free, starter, business, enterprise

    -- Limites
    rate_limit_per_second INT NOT NULL DEFAULT 10,
    rate_limit_per_day INT NOT NULL DEFAULT 10000,
    rate_limit_per_month INT NOT NULL DEFAULT 300000,

    -- Configuration
    allowed_origins TEXT[],  -- CORS origins
    webhook_url VARCHAR(500),

    -- Métadonnées
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_active_at TIMESTAMP,

    -- Contact
    contact_name VARCHAR(255),
    contact_phone VARCHAR(50),

    -- Facturation
    billing_email VARCHAR(255),
    billing_address TEXT
);

CREATE INDEX idx_partner_status ON partner(status);
CREATE INDEX idx_partner_tier ON partner(tier);
```

### 2.2 Table `api_key`

```sql
CREATE TABLE api_key (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL REFERENCES partner(id) ON DELETE CASCADE,

    -- Clé API (hashée)
    key_hash VARCHAR(255) NOT NULL UNIQUE,
    key_prefix VARCHAR(20) NOT NULL, -- Pour affichage dans le dashboard (ex: "pk_live_abc...")

    -- Métadonnées
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Permissions
    scopes TEXT[] NOT NULL DEFAULT ARRAY['read:routes'], -- read:routes, read:stops, write:data

    -- Status
    is_active BOOLEAN NOT NULL DEFAULT true,

    -- Dates
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,

    -- Sécurité
    allowed_ips INET[],

    CONSTRAINT fk_partner FOREIGN KEY (partner_id) REFERENCES partner(id)
);

CREATE INDEX idx_api_key_partner ON api_key(partner_id);
CREATE INDEX idx_api_key_hash ON api_key(key_hash);
CREATE INDEX idx_api_key_active ON api_key(is_active);
```

### 2.3 Table `usage_log`

```sql
CREATE TABLE usage_log (
    id BIGSERIAL PRIMARY KEY,

    -- Identification
    partner_id UUID NOT NULL REFERENCES partner(id) ON DELETE CASCADE,
    api_key_id UUID NOT NULL REFERENCES api_key(id) ON DELETE CASCADE,

    -- Requête
    endpoint VARCHAR(255) NOT NULL,
    method VARCHAR(10) NOT NULL,

    -- Performance
    response_time_ms INT NOT NULL,
    response_status INT NOT NULL,

    -- Détails
    from_location POINT,
    to_location POINT,
    cached BOOLEAN DEFAULT false,

    -- Cache hit/miss
    cache_hit BOOLEAN DEFAULT false,

    -- Timestamp
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),

    -- IP et User Agent
    ip_address INET,
    user_agent TEXT
);

-- Index pour analytics
CREATE INDEX idx_usage_partner_timestamp ON usage_log(partner_id, timestamp DESC);
CREATE INDEX idx_usage_timestamp ON usage_log(timestamp DESC);
CREATE INDEX idx_usage_endpoint ON usage_log(endpoint);

-- Partitionnement par mois pour optimiser les requêtes
-- (À mettre en place avec pg_partman ou manuellement)
```

### 2.4 Table `quota_usage`

```sql
CREATE TABLE quota_usage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL REFERENCES partner(id) ON DELETE CASCADE,

    -- Période
    period_type VARCHAR(20) NOT NULL, -- daily, monthly
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,

    -- Compteurs
    requests_count BIGINT NOT NULL DEFAULT 0,
    successful_requests BIGINT NOT NULL DEFAULT 0,
    failed_requests BIGINT NOT NULL DEFAULT 0,

    -- Coûts (pour facturation)
    cost_cents BIGINT NOT NULL DEFAULT 0,

    -- Métadonnées
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    UNIQUE(partner_id, period_type, period_start)
);

CREATE INDEX idx_quota_partner_period ON quota_usage(partner_id, period_type, period_start);
```

---

## 3. Système d'Authentification

### 3.1 Format des API Keys

```
Format: pk_{env}_{random}_{checksum}
Exemple: pk_live_4f8b2c3a9e1d5f6b7c8d9e0f1a2b3c4d_x7Y9

Composants:
- pk: Prefix "partner key"
- env: Environment (test, live)
- random: 32 caractères aléatoires
- checksum: 4 caractères de vérification
```

### 3.2 Middleware d'Authentification (Go)

```go
// internal/middleware/auth.go
package middleware

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "strings"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/jackc/pgx/v5/pgxpool"
)

type PartnerContext struct {
    PartnerID   string
    APIKeyID    string
    Tier        string
    Scopes      []string
}

// AuthMiddleware vérifie l'API key et charge les informations du partenaire
func AuthMiddleware(db *pgxpool.Pool) fiber.Handler {
    return func(c *fiber.Ctx) error {
        // Extraire l'API key du header Authorization
        authHeader := c.Get("Authorization")
        if authHeader == "" {
            return c.Status(401).JSON(fiber.Map{
                "error": "missing_api_key",
                "message": "API key is required. Use Authorization: Bearer YOUR_API_KEY",
            })
        }

        // Format: "Bearer pk_live_..."
        parts := strings.SplitN(authHeader, " ", 2)
        if len(parts) != 2 || parts[0] != "Bearer" {
            return c.Status(401).JSON(fiber.Map{
                "error": "invalid_auth_format",
                "message": "Authorization header must be in format: Bearer YOUR_API_KEY",
            })
        }

        apiKey := parts[1]

        // Valider le format de base
        if !strings.HasPrefix(apiKey, "pk_") {
            return c.Status(401).JSON(fiber.Map{
                "error": "invalid_api_key_format",
                "message": "API key must start with pk_",
            })
        }

        // Hasher la clé
        hash := sha256.Sum256([]byte(apiKey))
        keyHash := hex.EncodeToString(hash[:])

        // Rechercher dans la base de données
        ctx := context.Background()
        query := `
            SELECT
                ak.id,
                ak.partner_id,
                ak.scopes,
                p.tier,
                p.status,
                p.rate_limit_per_second,
                p.rate_limit_per_day,
                p.rate_limit_per_month
            FROM api_key ak
            JOIN partner p ON p.id = ak.partner_id
            WHERE ak.key_hash = $1
                AND ak.is_active = true
                AND p.status = 'active'
                AND (ak.expires_at IS NULL OR ak.expires_at > NOW())
        `

        var (
            apiKeyID            string
            partnerID           string
            scopes              []string
            tier                string
            status              string
            rateLimitPerSecond  int
            rateLimitPerDay     int
            rateLimitPerMonth   int
        )

        err := db.QueryRow(ctx, query, keyHash).Scan(
            &apiKeyID,
            &partnerID,
            &scopes,
            &tier,
            &status,
            &rateLimitPerSecond,
            &rateLimitPerDay,
            &rateLimitPerMonth,
        )

        if err != nil {
            return c.Status(401).JSON(fiber.Map{
                "error": "invalid_api_key",
                "message": "The provided API key is invalid or has been revoked",
            })
        }

        // Mettre à jour last_used_at de manière asynchrone
        go updateLastUsed(db, apiKeyID)

        // Stocker les informations dans le contexte
        c.Locals("partner", &PartnerContext{
            PartnerID: partnerID,
            APIKeyID:  apiKeyID,
            Tier:      tier,
            Scopes:    scopes,
        })

        c.Locals("rate_limits", map[string]int{
            "per_second": rateLimitPerSecond,
            "per_day":    rateLimitPerDay,
            "per_month":  rateLimitPerMonth,
        })

        return c.Next()
    }
}

func updateLastUsed(db *pgxpool.Pool, apiKeyID string) {
    ctx := context.Background()
    _, _ = db.Exec(ctx,
        "UPDATE api_key SET last_used_at = NOW() WHERE id = $1",
        apiKeyID,
    )
}
```

---

## 4. Rate Limiting

### 4.1 Middleware Rate Limiter

```go
// internal/middleware/ratelimit.go
package middleware

import (
    "context"
    "fmt"
    "strconv"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/redis/go-redis/v9"
)

// RateLimitMiddleware implémente un rate limiting multi-niveaux
func RateLimitMiddleware(rdb *redis.Client) fiber.Handler {
    return func(c *fiber.Ctx) error {
        partner := c.Locals("partner").(*PartnerContext)
        rateLimits := c.Locals("rate_limits").(map[string]int)

        ctx := context.Background()
        now := time.Now()

        // Clés Redis pour les différentes périodes
        keySecond := fmt.Sprintf("rl:partner:%s:second:%d", partner.PartnerID, now.Unix())
        keyDay := fmt.Sprintf("rl:partner:%s:day:%s", partner.PartnerID, now.Format("2006-01-02"))
        keyMonth := fmt.Sprintf("rl:partner:%s:month:%s", partner.PartnerID, now.Format("2006-01"))

        // Vérifier limite par seconde
        countSecond, err := rdb.Incr(ctx, keySecond).Result()
        if err == nil {
            rdb.Expire(ctx, keySecond, 1*time.Second)
            if countSecond > int64(rateLimits["per_second"]) {
                return c.Status(429).JSON(fiber.Map{
                    "error": "rate_limit_exceeded",
                    "message": "Too many requests per second",
                    "limit": rateLimits["per_second"],
                    "retry_after": 1,
                })
            }
        }

        // Vérifier limite par jour
        countDay, err := rdb.Incr(ctx, keyDay).Result()
        if err == nil {
            rdb.Expire(ctx, keyDay, 24*time.Hour)
            if countDay > int64(rateLimits["per_day"]) {
                return c.Status(429).JSON(fiber.Map{
                    "error": "daily_quota_exceeded",
                    "message": "Daily quota exceeded",
                    "limit": rateLimits["per_day"],
                    "used": countDay,
                })
            }
        }

        // Vérifier limite par mois
        countMonth, err := rdb.Incr(ctx, keyMonth).Result()
        if err == nil {
            rdb.Expire(ctx, keyMonth, 31*24*time.Hour)
            if countMonth > int64(rateLimits["per_month"]) {
                return c.Status(429).JSON(fiber.Map{
                    "error": "monthly_quota_exceeded",
                    "message": "Monthly quota exceeded",
                    "limit": rateLimits["per_month"],
                    "used": countMonth,
                })
            }
        }

        // Ajouter les headers de rate limit
        c.Set("X-RateLimit-Limit-Second", strconv.Itoa(rateLimits["per_second"]))
        c.Set("X-RateLimit-Remaining-Second", strconv.FormatInt(int64(rateLimits["per_second"])-countSecond, 10))
        c.Set("X-RateLimit-Limit-Day", strconv.Itoa(rateLimits["per_day"]))
        c.Set("X-RateLimit-Remaining-Day", strconv.FormatInt(int64(rateLimits["per_day"])-countDay, 10))
        c.Set("X-RateLimit-Limit-Month", strconv.Itoa(rateLimits["per_month"]))
        c.Set("X-RateLimit-Remaining-Month", strconv.FormatInt(int64(rateLimits["per_month"])-countMonth, 10))

        return c.Next()
    }
}
```

---

## 5. Analytics et Logging

### 5.1 Middleware de Logging

```go
// internal/middleware/analytics.go
package middleware

import (
    "context"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/jackc/pgx/v5/pgxpool"
)

// AnalyticsMiddleware enregistre toutes les requêtes pour analytics
func AnalyticsMiddleware(db *pgxpool.Pool) fiber.Handler {
    return func(c *fiber.Ctx) error {
        start := time.Now()

        // Traiter la requête
        err := c.Next()

        // Calculer le temps de réponse
        responseTime := time.Since(start).Milliseconds()

        // Extraire les informations du partenaire
        partner := c.Locals("partner").(*PartnerContext)

        // Logger de manière asynchrone
        go logRequest(db, &RequestLog{
            PartnerID:      partner.PartnerID,
            APIKeyID:       partner.APIKeyID,
            Endpoint:       c.Path(),
            Method:         c.Method(),
            ResponseTimeMs: int(responseTime),
            ResponseStatus: c.Response().StatusCode(),
            IPAddress:      c.IP(),
            UserAgent:      c.Get("User-Agent"),
            CacheHit:       c.Locals("cache_hit") == true,
        })

        return err
    }
}

type RequestLog struct {
    PartnerID      string
    APIKeyID       string
    Endpoint       string
    Method         string
    ResponseTimeMs int
    ResponseStatus int
    IPAddress      string
    UserAgent      string
    CacheHit       bool
}

func logRequest(db *pgxpool.Pool, log *RequestLog) {
    ctx := context.Background()
    query := `
        INSERT INTO usage_log (
            partner_id, api_key_id, endpoint, method,
            response_time_ms, response_status,
            ip_address, user_agent, cache_hit
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `

    _, _ = db.Exec(ctx, query,
        log.PartnerID,
        log.APIKeyID,
        log.Endpoint,
        log.Method,
        log.ResponseTimeMs,
        log.ResponseStatus,
        log.IPAddress,
        log.UserAgent,
        log.CacheHit,
    )

    // Mettre à jour les quotas
    updateQuotaUsage(db, log.PartnerID)
}

func updateQuotaUsage(db *pgxpool.Pool, partnerID string) {
    ctx := context.Background()
    now := time.Now()

    // Quota journalier
    query := `
        INSERT INTO quota_usage (partner_id, period_type, period_start, period_end, requests_count)
        VALUES ($1, 'daily', $2, $3, 1)
        ON CONFLICT (partner_id, period_type, period_start)
        DO UPDATE SET
            requests_count = quota_usage.requests_count + 1,
            updated_at = NOW()
    `
    _, _ = db.Exec(ctx, query, partnerID, now.Format("2006-01-02"), now.Format("2006-01-02"))

    // Quota mensuel
    firstDayOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
    lastDayOfMonth := firstDayOfMonth.AddDate(0, 1, -1)
    query = `
        INSERT INTO quota_usage (partner_id, period_type, period_start, period_end, requests_count)
        VALUES ($1, 'monthly', $2, $3, 1)
        ON CONFLICT (partner_id, period_type, period_start)
        DO UPDATE SET
            requests_count = quota_usage.requests_count + 1,
            updated_at = NOW()
    `
    _, _ = db.Exec(ctx, query, partnerID, firstDayOfMonth.Format("2006-01-02"), lastDayOfMonth.Format("2006-01-02"))
}
```

---

## 6. Plans Tarifaires

### 6.1 Tiers de Service

| Tier | Prix/mois | Requêtes/jour | Requêtes/mois | Support | SLA |
|------|-----------|---------------|---------------|---------|-----|
| **Free** | 0€ | 1,000 | 30,000 | Community | - |
| **Starter** | 49€ | 10,000 | 300,000 | Email | 99% |
| **Business** | 199€ | 50,000 | 1,500,000 | Email + Chat | 99.5% |
| **Enterprise** | Custom | Unlimited | Unlimited | Dedicated | 99.9% |

### 6.2 Configuration des Tiers

```sql
-- Créer une table de configuration des tiers
CREATE TABLE tier_config (
    tier VARCHAR(50) PRIMARY KEY,
    price_cents INT NOT NULL,
    rate_limit_per_second INT NOT NULL,
    rate_limit_per_day INT NOT NULL,
    rate_limit_per_month INT NOT NULL,
    features JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

INSERT INTO tier_config (tier, price_cents, rate_limit_per_second, rate_limit_per_day, rate_limit_per_month, features) VALUES
('free', 0, 2, 1000, 30000, '{"support": "community", "sla": null, "custom_domain": false, "webhooks": false}'),
('starter', 4900, 10, 10000, 300000, '{"support": "email", "sla": "99%", "custom_domain": false, "webhooks": true}'),
('business', 19900, 50, 50000, 1500000, '{"support": "email+chat", "sla": "99.5%", "custom_domain": true, "webhooks": true}'),
('enterprise', 0, 1000, -1, -1, '{"support": "dedicated", "sla": "99.9%", "custom_domain": true, "webhooks": true, "custom_features": true}');
```

---

## 7. Dashboard Partenaire (API Backend)

### 7.1 Endpoints Dashboard

```go
// internal/api/partner_dashboard.go
package api

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "time"

    "github.com/gofiber/fiber/v2"
    "github.com/jackc/pgx/v5/pgxpool"
)

// GetPartnerInfo retourne les informations du partenaire
func GetPartnerInfo(c *fiber.Ctx) error {
    partner := c.Locals("partner").(*PartnerContext)
    pool := c.Locals("db").(*pgxpool.Pool)

    ctx := context.Background()
    query := `
        SELECT
            id, name, email, company, status, tier,
            rate_limit_per_second, rate_limit_per_day, rate_limit_per_month,
            created_at, last_active_at
        FROM partner
        WHERE id = $1
    `

    var p Partner
    err := pool.QueryRow(ctx, query, partner.PartnerID).Scan(
        &p.ID, &p.Name, &p.Email, &p.Company, &p.Status, &p.Tier,
        &p.RateLimitPerSecond, &p.RateLimitPerDay, &p.RateLimitPerMonth,
        &p.CreatedAt, &p.LastActiveAt,
    )

    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "internal_server_error"})
    }

    return c.JSON(p)
}

// GetAPIKeys liste toutes les API keys du partenaire
func GetAPIKeys(c *fiber.Ctx) error {
    partner := c.Locals("partner").(*PartnerContext)
    pool := c.Locals("db").(*pgxpool.Pool)

    ctx := context.Background()
    query := `
        SELECT
            id, name, key_prefix, description, scopes,
            is_active, created_at, expires_at, last_used_at
        FROM api_key
        WHERE partner_id = $1
        ORDER BY created_at DESC
    `

    rows, err := pool.Query(ctx, query, partner.PartnerID)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "internal_server_error"})
    }
    defer rows.Close()

    var keys []APIKey
    for rows.Next() {
        var k APIKey
        rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.Description, &k.Scopes,
            &k.IsActive, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt)
        keys = append(keys, k)
    }

    return c.JSON(fiber.Map{"api_keys": keys})
}

// CreateAPIKey crée une nouvelle API key
func CreateAPIKey(c *fiber.Ctx) error {
    partner := c.Locals("partner").(*PartnerContext)
    pool := c.Locals("db").(*pgxpool.Pool)

    var req struct {
        Name        string   `json:"name"`
        Description string   `json:"description"`
        Scopes      []string `json:"scopes"`
        ExpiresAt   *time.Time `json:"expires_at"`
    }

    if err := c.BodyParser(&req); err != nil {
        return c.Status(400).JSON(fiber.Map{"error": "invalid_request"})
    }

    // Générer une nouvelle clé API
    apiKey, keyHash, keyPrefix := generateAPIKey("live")

    ctx := context.Background()
    query := `
        INSERT INTO api_key (
            partner_id, key_hash, key_prefix, name, description, scopes, expires_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id, created_at
    `

    var keyID string
    var createdAt time.Time
    err := pool.QueryRow(ctx, query,
        partner.PartnerID, keyHash, keyPrefix, req.Name, req.Description, req.Scopes, req.ExpiresAt,
    ).Scan(&keyID, &createdAt)

    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "failed_to_create_key"})
    }

    return c.Status(201).JSON(fiber.Map{
        "id": keyID,
        "api_key": apiKey, // Afficher UNE SEULE FOIS
        "key_prefix": keyPrefix,
        "created_at": createdAt,
        "warning": "Save this key now. You won't be able to see it again!",
    })
}

// GetUsageStats retourne les statistiques d'utilisation
func GetUsageStats(c *fiber.Ctx) error {
    partner := c.Locals("partner").(*PartnerContext)
    pool := c.Locals("db").(*pgxpool.Pool)

    // Paramètres de période
    period := c.Query("period", "day") // day, week, month

    ctx := context.Background()
    query := `
        SELECT
            DATE(timestamp) as date,
            COUNT(*) as total_requests,
            COUNT(*) FILTER (WHERE response_status >= 200 AND response_status < 300) as successful,
            COUNT(*) FILTER (WHERE response_status >= 400) as failed,
            AVG(response_time_ms) as avg_response_time,
            COUNT(*) FILTER (WHERE cache_hit = true) as cache_hits
        FROM usage_log
        WHERE partner_id = $1
            AND timestamp >= NOW() - INTERVAL '30 days'
        GROUP BY DATE(timestamp)
        ORDER BY date DESC
    `

    rows, err := pool.Query(ctx, query, partner.PartnerID)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "internal_server_error"})
    }
    defer rows.Close()

    var stats []UsageStat
    for rows.Next() {
        var s UsageStat
        rows.Scan(&s.Date, &s.TotalRequests, &s.Successful, &s.Failed, &s.AvgResponseTime, &s.CacheHits)
        stats = append(stats, s)
    }

    return c.JSON(fiber.Map{"stats": stats})
}

// generateAPIKey génère une nouvelle clé API sécurisée
func generateAPIKey(env string) (key, hash, prefix string) {
    // Générer 32 bytes aléatoires
    randomBytes := make([]byte, 32)
    rand.Read(randomBytes)
    randomStr := hex.EncodeToString(randomBytes)

    // Générer checksum
    checksumBytes := sha256.Sum256([]byte(randomStr))
    checksum := hex.EncodeToString(checksumBytes[:2])

    // Construire la clé
    key = fmt.Sprintf("pk_%s_%s_%s", env, randomStr, checksum)

    // Hasher pour stockage
    hashBytes := sha256.Sum256([]byte(key))
    hash = hex.EncodeToString(hashBytes[:])

    // Prefix pour affichage
    prefix = fmt.Sprintf("pk_%s_%s...", env, randomStr[:8])

    return
}

// Types
type Partner struct {
    ID                  string     `json:"id"`
    Name                string     `json:"name"`
    Email               string     `json:"email"`
    Company             string     `json:"company"`
    Status              string     `json:"status"`
    Tier                string     `json:"tier"`
    RateLimitPerSecond  int        `json:"rate_limit_per_second"`
    RateLimitPerDay     int        `json:"rate_limit_per_day"`
    RateLimitPerMonth   int        `json:"rate_limit_per_month"`
    CreatedAt           time.Time  `json:"created_at"`
    LastActiveAt        *time.Time `json:"last_active_at"`
}

type APIKey struct {
    ID          string     `json:"id"`
    Name        string     `json:"name"`
    KeyPrefix   string     `json:"key_prefix"`
    Description string     `json:"description"`
    Scopes      []string   `json:"scopes"`
    IsActive    bool       `json:"is_active"`
    CreatedAt   time.Time  `json:"created_at"`
    ExpiresAt   *time.Time `json:"expires_at"`
    LastUsedAt  *time.Time `json:"last_used_at"`
}

type UsageStat struct {
    Date            string  `json:"date"`
    TotalRequests   int     `json:"total_requests"`
    Successful      int     `json:"successful"`
    Failed          int     `json:"failed"`
    AvgResponseTime float64 `json:"avg_response_time_ms"`
    CacheHits       int     `json:"cache_hits"`
}
```

---

## 8. Routage de l'API

### 8.1 Structure des Routes

```go
// cmd/api/main.go
package main

import (
    "log"
    "os"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/passbi/passbi_core/internal/api"
    "github.com/passbi/passbi_core/internal/middleware"
    "github.com/passbi/passbi_core/internal/db"
    "github.com/passbi/passbi_core/internal/cache"
)

func main() {
    // Initialiser la base de données
    pool, err := db.GetDB()
    if err != nil {
        log.Fatal("Failed to connect to database:", err)
    }
    defer pool.Close()

    // Initialiser Redis
    rdb := cache.GetRedis()
    defer rdb.Close()

    // Créer l'application Fiber
    app := fiber.New(fiber.Config{
        ErrorHandler: customErrorHandler,
    })

    // Middlewares globaux
    app.Use(logger.New())
    app.Use(cors.New(cors.Config{
        AllowOrigins: "*",
        AllowHeaders: "Origin, Content-Type, Accept, Authorization",
    }))

    // Health check (public)
    app.Get("/health", api.Health)

    // Routes publiques (documentation, etc.)
    app.Get("/", func(c *fiber.Ctx) error {
        return c.JSON(fiber.Map{
            "name": "PassBI Core API",
            "version": "2.0.0",
            "docs": "https://api.passbi.com/docs",
        })
    })

    // ============================================
    // API V2 - Protégée par authentification
    // ============================================
    v2 := app.Group("/v2")

    // Middlewares pour les routes protégées
    v2.Use(middleware.AuthMiddleware(pool))          // Authentification
    v2.Use(middleware.RateLimitMiddleware(rdb))      // Rate limiting
    v2.Use(middleware.AnalyticsMiddleware(pool))     // Logging & Analytics

    // Routes de l'API Core
    v2.Get("/route-search", api.RouteSearch)
    v2.Get("/stops/nearby", api.StopsNearby)
    v2.Get("/routes/list", api.RoutesList)

    // ============================================
    // Dashboard API - Gestion des partenaires
    // ============================================
    dashboard := app.Group("/dashboard")
    dashboard.Use(middleware.AuthMiddleware(pool))

    // Informations du partenaire
    dashboard.Get("/me", api.GetPartnerInfo)

    // Gestion des API Keys
    dashboard.Get("/api-keys", api.GetAPIKeys)
    dashboard.Post("/api-keys", api.CreateAPIKey)
    dashboard.Delete("/api-keys/:id", api.RevokeAPIKey)

    // Analytics
    dashboard.Get("/usage", api.GetUsageStats)
    dashboard.Get("/quota", api.GetQuotaUsage)

    // Démarrer le serveur
    port := os.Getenv("API_PORT")
    if port == "" {
        port = "8080"
    }

    log.Printf("🚀 PassBI API starting on port %s", port)
    log.Fatal(app.Listen(":" + port))
}

func customErrorHandler(c *fiber.Ctx, err error) error {
    code := fiber.StatusInternalServerError

    if e, ok := err.(*fiber.Error); ok {
        code = e.Code
    }

    return c.Status(code).JSON(fiber.Map{
        "error": "internal_error",
        "message": err.Error(),
    })
}
```

---

## 9. Migrations SQL

### 9.1 Migration Initiale

```sql
-- migrations/000004_partner_system.up.sql

-- Table partner
CREATE TABLE partner (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    company VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    tier VARCHAR(50) NOT NULL DEFAULT 'free',
    rate_limit_per_second INT NOT NULL DEFAULT 10,
    rate_limit_per_day INT NOT NULL DEFAULT 10000,
    rate_limit_per_month INT NOT NULL DEFAULT 300000,
    allowed_origins TEXT[],
    webhook_url VARCHAR(500),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_active_at TIMESTAMP,
    contact_name VARCHAR(255),
    contact_phone VARCHAR(50),
    billing_email VARCHAR(255),
    billing_address TEXT
);

CREATE INDEX idx_partner_status ON partner(status);
CREATE INDEX idx_partner_tier ON partner(tier);
CREATE INDEX idx_partner_email ON partner(email);

-- Table api_key
CREATE TABLE api_key (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL REFERENCES partner(id) ON DELETE CASCADE,
    key_hash VARCHAR(255) NOT NULL UNIQUE,
    key_prefix VARCHAR(20) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    scopes TEXT[] NOT NULL DEFAULT ARRAY['read:routes'],
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    allowed_ips INET[]
);

CREATE INDEX idx_api_key_partner ON api_key(partner_id);
CREATE INDEX idx_api_key_hash ON api_key(key_hash);
CREATE INDEX idx_api_key_active ON api_key(is_active);

-- Table usage_log
CREATE TABLE usage_log (
    id BIGSERIAL PRIMARY KEY,
    partner_id UUID NOT NULL REFERENCES partner(id) ON DELETE CASCADE,
    api_key_id UUID NOT NULL REFERENCES api_key(id) ON DELETE CASCADE,
    endpoint VARCHAR(255) NOT NULL,
    method VARCHAR(10) NOT NULL,
    response_time_ms INT NOT NULL,
    response_status INT NOT NULL,
    from_location POINT,
    to_location POINT,
    cache_hit BOOLEAN DEFAULT false,
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
    ip_address INET,
    user_agent TEXT
);

CREATE INDEX idx_usage_partner_timestamp ON usage_log(partner_id, timestamp DESC);
CREATE INDEX idx_usage_timestamp ON usage_log(timestamp DESC);
CREATE INDEX idx_usage_endpoint ON usage_log(endpoint);

-- Table quota_usage
CREATE TABLE quota_usage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL REFERENCES partner(id) ON DELETE CASCADE,
    period_type VARCHAR(20) NOT NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    requests_count BIGINT NOT NULL DEFAULT 0,
    successful_requests BIGINT NOT NULL DEFAULT 0,
    failed_requests BIGINT NOT NULL DEFAULT 0,
    cost_cents BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(partner_id, period_type, period_start)
);

CREATE INDEX idx_quota_partner_period ON quota_usage(partner_id, period_type, period_start);

-- Table tier_config
CREATE TABLE tier_config (
    tier VARCHAR(50) PRIMARY KEY,
    price_cents INT NOT NULL,
    rate_limit_per_second INT NOT NULL,
    rate_limit_per_day INT NOT NULL,
    rate_limit_per_month INT NOT NULL,
    features JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Insérer les tiers par défaut
INSERT INTO tier_config (tier, price_cents, rate_limit_per_second, rate_limit_per_day, rate_limit_per_month, features) VALUES
('free', 0, 2, 1000, 30000, '{"support": "community", "sla": null, "custom_domain": false, "webhooks": false}'),
('starter', 4900, 10, 10000, 300000, '{"support": "email", "sla": "99%", "custom_domain": false, "webhooks": true}'),
('business', 19900, 50, 50000, 1500000, '{"support": "email+chat", "sla": "99.5%", "custom_domain": true, "webhooks": true}'),
('enterprise', 0, 1000, -1, -1, '{"support": "dedicated", "sla": "99.9%", "custom_domain": true, "webhooks": true, "custom_features": true}');
```

---

## 10. Plan de Déploiement

### 10.1 Phases de Déploiement

**Phase 1: Infrastructure (Semaine 1)**
- [ ] Créer les migrations de base de données
- [ ] Déployer les nouvelles tables
- [ ] Configurer Redis pour rate limiting
- [ ] Tests de charge

**Phase 2: Backend API (Semaine 2-3)**
- [ ] Implémenter les middlewares d'authentification
- [ ] Implémenter le rate limiting
- [ ] Implémenter l'analytics et logging
- [ ] Tests unitaires et d'intégration

**Phase 3: Dashboard API (Semaine 4)**
- [ ] Endpoints de gestion de partenaires
- [ ] Endpoints de gestion des API keys
- [ ] Endpoints d'analytics
- [ ] Tests d'intégration

**Phase 4: Documentation (Semaine 5)**
- [ ] Documentation API complète
- [ ] Guides d'intégration
- [ ] Exemples de code
- [ ] Documentation du dashboard

**Phase 5: Onboarding (Semaine 6)**
- [ ] Processus d'inscription partenaires
- [ ] Email de bienvenue
- [ ] Tutoriel first-time use
- [ ] Support client

### 10.2 Configuration Render.yaml

```yaml
# Ajouter dans render.yaml
services:
  - type: web
    name: passbi-api
    # ... configuration existante ...
    envVars:
      # ... vars existantes ...

      # Partner System
      - key: ENABLE_AUTH
        value: true
      - key: ENABLE_RATE_LIMIT
        value: true
      - key: ENABLE_ANALYTICS
        value: true
```

---

## 11. Monitoring et Alertes

### 11.1 Métriques Clés

```yaml
# Métriques à surveiller
metrics:
  - name: api_requests_total
    type: counter
    labels: [partner_id, endpoint, status]

  - name: api_request_duration_seconds
    type: histogram
    labels: [partner_id, endpoint]

  - name: rate_limit_exceeded_total
    type: counter
    labels: [partner_id, limit_type]

  - name: active_partners_total
    type: gauge

  - name: quota_usage_percentage
    type: gauge
    labels: [partner_id, period_type]
```

### 11.2 Alertes

```yaml
alerts:
  - name: HighErrorRate
    condition: error_rate > 5%
    severity: warning
    duration: 5m

  - name: PartnerQuotaExceeded
    condition: quota_usage > 95%
    severity: info
    notification: webhook

  - name: APISlowResponse
    condition: p95_latency > 1s
    severity: warning
    duration: 10m
```

---

## 12. Exemple d'Utilisation

### 12.1 Pour les Partenaires

```bash
# 1. Obtenir une API key (via dashboard web ou CLI)
# Après inscription, le partenaire reçoit: pk_live_abc123...

# 2. Faire une requête API
curl -X GET "https://api.passbi.com/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467" \
  -H "Authorization: Bearer pk_live_abc123..."

# 3. Réponse avec headers de rate limit
HTTP/1.1 200 OK
X-RateLimit-Limit-Second: 10
X-RateLimit-Remaining-Second: 9
X-RateLimit-Limit-Day: 10000
X-RateLimit-Remaining-Day: 9523
X-RateLimit-Limit-Month: 300000
X-RateLimit-Remaining-Month: 287456

{
  "routes": {
    "direct": {...},
    "simple": {...},
    "fast": {...}
  }
}
```

### 12.2 Code Client (JavaScript)

```javascript
// SDK Client Example
class PassBIClient {
    constructor(apiKey) {
        this.apiKey = apiKey;
        this.baseURL = 'https://api.passbi.com/v2';
    }

    async searchRoute(from, to) {
        const response = await fetch(
            `${this.baseURL}/route-search?from=${from}&to=${to}`,
            {
                headers: {
                    'Authorization': `Bearer ${this.apiKey}`,
                    'Content-Type': 'application/json'
                }
            }
        );

        if (!response.ok) {
            if (response.status === 429) {
                throw new Error('Rate limit exceeded');
            }
            throw new Error(`API Error: ${response.statusText}`);
        }

        return await response.json();
    }
}

// Usage
const client = new PassBIClient('pk_live_abc123...');
const routes = await client.searchRoute('14.7167,-17.4677', '14.6928,-17.4467');
console.log(routes);
```

---

## 13. Sécurité

### 13.1 Best Practices

1. **API Keys**
   - Toujours utiliser HTTPS
   - Ne jamais exposer les clés dans le code frontend
   - Rotation régulière des clés
   - Support de la révocation immédiate

2. **Rate Limiting**
   - Limites adaptées par tier
   - Headers de rate limit dans les réponses
   - Gestion des pics de trafic

3. **Authentification**
   - Clés hashées en SHA-256
   - Support de l'expiration
   - IP whitelisting (optionnel)
   - Scopes pour permissions granulaires

4. **Monitoring**
   - Alertes sur comportements anormaux
   - Détection de patterns d'abus
   - Logs détaillés avec rétention

---

## 14. Roadmap Future

### V2.1 - Q2 2026
- [ ] OAuth2 support
- [ ] Webhooks pour événements
- [ ] Multi-région (edge locations)
- [ ] GraphQL API

### V2.2 - Q3 2026
- [ ] Real-time WebSocket API
- [ ] Batch API pour requêtes multiples
- [ ] API versioning avancé
- [ ] Custom SLA par partenaire

### V3.0 - Q4 2026
- [ ] Self-service portal complet
- [ ] Marketplace de plugins
- [ ] White-label API
- [ ] Enterprise features

---

## Conclusion

Cette architecture fournit une base solide pour offrir PassBI en tant qu'API-as-a-Service à des partenaires. Elle couvre :

✅ Authentification sécurisée avec API Keys
✅ Rate limiting multi-niveaux
✅ Analytics et monitoring complets
✅ Gestion multi-tenant
✅ Plans tarifaires flexibles
✅ Dashboard pour partenaires
✅ Documentation complète

**Prochaines étapes:**
1. Valider l'architecture avec l'équipe
2. Prioriser les phases de déploiement
3. Commencer par la Phase 1 (Infrastructure)
4. Mettre en place les tests automatisés
5. Déployer progressivement avec quelques partenaires pilotes

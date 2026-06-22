package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	_ "github.com/lib/pq"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("DB config parse failed: %v", err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		log.Fatalf("DB connection failed: %v", err)
	}
	defer pool.Close()

	log.Println("🌱 Seeding test data...")

	seedAdmins(pool)
	seedPartners(pool)

	log.Println("✅ Done.")
}

// ─── Admin users ─────────────────────────────────────────────────────────────

type adminSeed struct {
	Name     string
	Email    string
	Password string
	Role     string
}

func seedAdmins(pool *pgxpool.Pool) {
	admins := []adminSeed{
		{"Super Admin", "superadmin@passbi.com", "admin123", "superadmin"},
		{"Admin Test", "admin@passbi.com", "admin123", "admin"},
		{"Ops Test", "ops@passbi.com", "ops123", "ops"},
	}

	for _, a := range admins {
		hash, err := bcrypt.GenerateFromPassword([]byte(a.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("  ✗ bcrypt failed for %s: %v", a.Email, err)
			continue
		}

		_, err = pool.Exec(context.Background(), `
			INSERT INTO admin_user (name, email, password_hash, role, is_active)
			VALUES ($1, $2, $3, $4, true)
			ON CONFLICT (email) DO UPDATE
				SET password_hash = EXCLUDED.password_hash,
				    role = EXCLUDED.role
		`, a.Name, a.Email, string(hash), a.Role)

		if err != nil {
			log.Printf("  ✗ admin %s: %v", a.Email, err)
		} else {
			log.Printf("  ✓ admin  %-30s  role=%-11s  pass=%s", a.Email, a.Role, a.Password)
		}
	}
}

// ─── Partner accounts ────────────────────────────────────────────────────────

type partnerSeed struct {
	Name        string
	Email       string
	Company     string
	Tier        string
	ContactName string
	Password    string // for portal login
}

func seedPartners(pool *pgxpool.Pool) {
	partners := []partnerSeed{
		{"Tech Corp", "partner@techcorp.com", "Tech Corp SAS", "business", "Alice Diallo", "partner123"},
		{"Startup Co", "partner@startupco.com", "Startup Co SARL", "starter", "Bob Traoré", "partner123"},
		{"Free User", "partner@freeuser.com", "Free User", "free", "Charlie Koné", "partner123"},
	}

	for _, p := range partners {
		// Pull rate limits from tier_config
		var rateSecond, rateDay, rateMonth int
		_ = pool.QueryRow(context.Background(),
			`SELECT rate_limit_per_second, rate_limit_per_day, rate_limit_per_month FROM tier_config WHERE tier = $1`,
			p.Tier,
		).Scan(&rateSecond, &rateDay, &rateMonth)

		hash, _ := bcrypt.GenerateFromPassword([]byte(p.Password), bcrypt.DefaultCost)

		var partnerID string
		err := pool.QueryRow(context.Background(), `
			INSERT INTO partner (name, email, company, tier, status, contact_name,
				rate_limit_per_second, rate_limit_per_day, rate_limit_per_month,
				password_hash, portal_enabled)
			VALUES ($1, $2, $3, $4, 'active', $5, $6, $7, $8, $9, true)
			ON CONFLICT (email) DO UPDATE
				SET password_hash   = EXCLUDED.password_hash,
				    portal_enabled  = true,
				    tier            = EXCLUDED.tier
			RETURNING id
		`, p.Name, p.Email, p.Company, p.Tier, p.ContactName,
			rateSecond, rateDay, rateMonth, string(hash),
		).Scan(&partnerID)

		if err != nil {
			log.Printf("  ✗ partner %s: %v", p.Email, err)
			continue
		}

		log.Printf("  ✓ partner %-30s  tier=%-10s  pass=%s", p.Email, p.Tier, p.Password)

		// Create one test API key per partner
		seedAPIKey(pool, partnerID, p.Name)

		// Create fixed dev key for Tech Corp (first partner) for local app dev
		if p.Email == "partner@techcorp.com" {
			seedFixedDevKey(pool, partnerID)
		}
	}
}

func seedAPIKey(pool *pgxpool.Pool, partnerID, partnerName string) {
	rawKey := fmt.Sprintf("pk_test_%s_devkey123", partnerID[:8])
	h := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(h[:])
	prefix := rawKey[:20]

	_, err := pool.Exec(context.Background(), `
		INSERT INTO api_key (partner_id, key_hash, key_prefix, name, scopes, is_active)
		VALUES ($1, $2, $3, $4, ARRAY['read:routes', '*'], true)
		ON CONFLICT (key_hash) DO NOTHING
	`, partnerID, keyHash, prefix, fmt.Sprintf("Clé test — %s", partnerName))

	if err != nil {
		log.Printf("    ✗ api_key for %s: %v", partnerName, err)
	} else {
		log.Printf("    ✓ api_key  prefix=%s…  raw=%s", prefix, rawKey)
	}
}

// seedFixedDevKey inserts a well-known API key for local frontend development.
// Key: pk_dev_local_passbi2024  (set in environment.ts)
func seedFixedDevKey(pool *pgxpool.Pool, partnerID string) {
	const rawKey = "pk_dev_local_passbi2024"
	h := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(h[:])

	_, err := pool.Exec(context.Background(), `
		INSERT INTO api_key (partner_id, key_hash, key_prefix, name, scopes, is_active)
		VALUES ($1, $2, $3, $4, ARRAY['*'], true)
		ON CONFLICT (key_hash) DO NOTHING
	`, partnerID, keyHash, "pk_dev_local_pass", "Clé dev local — app mobile")

	if err != nil {
		log.Printf("    ✗ dev key: %v", err)
	} else {
		log.Printf("    ✓ dev key  raw=%s", rawKey)
	}
}

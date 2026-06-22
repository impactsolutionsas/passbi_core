package partner_portal

import (
	"context"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/audit"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login authenticates a partner for the self-service portal
func Login(db *pgxpool.Pool, auditLog *audit.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		reqID, _ := c.Locals("request_id").(string)
		ip := c.IP()

		var req loginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid_body"})
		}
		if req.Email == "" || req.Password == "" {
			return c.Status(400).JSON(fiber.Map{"error": "email and password are required"})
		}

		var (
			id            string
			name          string
			company       string
			tier          string
			status        string
			portalEnabled bool
			passwordHash  *string
		)

		err := db.QueryRow(context.Background(),
			`SELECT id, COALESCE(contact_name, ''), COALESCE(company, ''), tier, status, portal_enabled, password_hash
			 FROM partner WHERE email = $1`,
			req.Email,
		).Scan(&id, &name, &company, &tier, &status, &portalEnabled, &passwordHash)

		if err != nil {
			auditLog.LogAsync(context.Background(), audit.Entry{
				EventType: audit.EventAuthLoginFailed,
				ActorType: audit.ActorAnonymous,
				ActorIP:   ip,
				Action:    audit.ActionLogin,
				Outcome:   audit.OutcomeFailure,
				Reason:    "user_not_found",
				RequestID: reqID,
				Metadata:  map[string]any{"email": req.Email, "portal": "partner"},
			})
			return c.Status(401).JSON(fiber.Map{"error": "invalid_credentials"})
		}
		if status != "active" {
			auditLog.LogAsync(context.Background(), audit.Entry{
				EventType:    audit.EventAuthLoginFailed,
				ActorType:    audit.ActorPartner,
				ActorID:      id,
				ActorIP:      ip,
				ResourceType: "partner",
				ResourceID:   id,
				Action:       audit.ActionLogin,
				Outcome:      audit.OutcomeDenied,
				Reason:       "account_suspended",
				RequestID:    reqID,
			})
			return c.Status(403).JSON(fiber.Map{"error": "account_suspended"})
		}
		if !portalEnabled || passwordHash == nil {
			return c.Status(403).JSON(fiber.Map{"error": "portal_access_not_enabled"})
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(req.Password)); err != nil {
			auditLog.LogAsync(context.Background(), audit.Entry{
				EventType:    audit.EventAuthLoginFailed,
				ActorType:    audit.ActorPartner,
				ActorID:      id,
				ActorIP:      ip,
				ResourceType: "partner",
				ResourceID:   id,
				Action:       audit.ActionLogin,
				Outcome:      audit.OutcomeFailure,
				Reason:       "invalid_password",
				RequestID:    reqID,
			})
			return c.Status(401).JSON(fiber.Map{"error": "invalid_credentials"})
		}

		token, err := issuePartnerJWT(id, req.Email, name, company, tier)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "token_generation_failed"})
		}

		auditLog.LogAsync(context.Background(), audit.Entry{
			EventType:    audit.EventAuthLogin,
			ActorType:    audit.ActorPartner,
			ActorID:      id,
			ActorIP:      ip,
			ActorUA:      c.Get("User-Agent"),
			ResourceType: "partner",
			ResourceID:   id,
			Action:       audit.ActionLogin,
			RequestID:    reqID,
			Metadata:     map[string]any{"tier": tier, "company": company},
		})

		return c.JSON(fiber.Map{
			"token":      token,
			"expires_in": 86400,
			"partner": fiber.Map{
				"id":      id,
				"email":   req.Email,
				"name":    name,
				"company": company,
				"tier":    tier,
			},
		})
	}
}

func issuePartnerJWT(id, email, name, company, tier string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "changeme-jwt-secret"
	}

	claims := jwt.MapClaims{
		"sub":     id,
		"email":   email,
		"name":    name,
		"company": company,
		"tier":    tier,
		"aud":     "partner_portal",
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// Me returns the current partner's profile from JWT claims
func Me() fiber.Handler {
	return func(c *fiber.Ctx) error {
		partner := c.Locals("portal_partner")
		return c.JSON(partner)
	}
}

// Package audit implements RGPD-compliant append-only audit trail.
// All sensitive operations (auth, partner mutations, API key lifecycle) must
// call Log() or LogAsync(). Entries are immutable by DB policy.
package audit

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Event types — regroupés par domaine pour faciliter les exports RGPD
const (
	// Auth
	EventAuthLogin        = "auth.login"
	EventAuthLoginFailed  = "auth.login_failed"
	EventAuthLogout       = "auth.logout"
	EventAuthTokenRefresh = "auth.token_refresh"

	// API Keys
	EventAPIKeyCreated = "apikey.created"
	EventAPIKeyRevoked = "apikey.revoked"
	EventAPIKeyUsed    = "apikey.used"     // seulement pour les accès sensibles

	// Partners (données personnelles RGPD)
	EventPartnerCreated = "partner.created"
	EventPartnerUpdated = "partner.updated"
	EventPartnerDeleted = "partner.deleted"
	EventPartnerViewed  = "partner.viewed"  // accès données personnelles

	// Données / exports
	EventDataExport  = "data.export"
	EventDataAccess  = "data.access"
	EventDataErasure = "data.erasure" // droit à l'oubli RGPD

	// Admin
	EventAdminAction    = "admin.action"
	EventConfigChanged  = "config.changed"
	EventTierUpdated    = "tier.updated"
	EventImportTriggered = "gtfs.import_triggered"

	// Sécurité
	EventIPBlocked        = "security.ip_blocked"
	EventBruteForce       = "security.brute_force"
	EventSuspiciousAccess = "security.suspicious_access"
	EventRateLimitHit     = "security.rate_limit"
)

// Action verbes standardisés
const (
	ActionCreate = "CREATE"
	ActionRead   = "READ"
	ActionUpdate = "UPDATE"
	ActionDelete = "DELETE"
	ActionLogin  = "LOGIN"
	ActionLogout = "LOGOUT"
	ActionRevoke = "REVOKE"
	ActionExport = "EXPORT"
	ActionBlock  = "BLOCK"
)

// Outcome
const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomeDenied  = "denied"
)

// ActorType
const (
	ActorAdmin     = "admin"
	ActorPartner   = "partner"
	ActorSystem    = "system"
	ActorAnonymous = "anonymous"
)

// Entry represents a single audit log entry.
type Entry struct {
	EventType    string
	ActorType    string
	ActorID      string
	ActorIP      string
	ActorUA      string
	ResourceType string
	ResourceID   string
	Action       string
	Outcome      string
	Reason       string
	BeforeState  any  // sera sérialisé en JSONB — exclure les champs sensibles avant passage
	AfterState   any
	RequestID    string
	SessionID    string
	Metadata     map[string]any
}

// Logger writes audit entries to Postgres.
type Logger struct {
	db *pgxpool.Pool
}

// New creates an audit Logger.
func New(db *pgxpool.Pool) *Logger {
	return &Logger{db: db}
}

// Log writes an audit entry synchronously. Use in critical paths (auth, mutations).
func (l *Logger) Log(ctx context.Context, e Entry) error {
	return l.write(ctx, e)
}

// LogAsync writes an audit entry in a goroutine. Use for high-volume read events.
func (l *Logger) LogAsync(ctx context.Context, e Entry) {
	go func() {
		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := l.write(writeCtx, e); err != nil {
			log.Printf("[audit] write error: %v | event=%s actor=%s", err, e.EventType, e.ActorID)
		}
	}()
}

func (l *Logger) write(ctx context.Context, e Entry) error {
	before, _ := json.Marshal(e.BeforeState)
	after, _ := json.Marshal(e.AfterState)
	meta, _ := json.Marshal(e.Metadata)

	if e.Outcome == "" {
		e.Outcome = OutcomeSuccess
	}

	// Casts explicites ::inet et ::jsonb — obligatoire en mode SimpleProtocol
	// (Supabase pooler port 6543). pgx envoie tout en text, PostgreSQL fait le cast.
	_, err := l.db.Exec(ctx, `
		INSERT INTO audit_log (
			event_type, actor_type, actor_id, actor_ip, actor_ua,
			resource_type, resource_id, action, outcome, reason,
			before_state, after_state, request_id, session_id, metadata
		) VALUES (
			$1, $2, $3, NULLIF($4,'')::inet, $5,
			$6, $7, $8, $9, $10,
			NULLIF($11,'')::jsonb, NULLIF($12,'')::jsonb, $13, $14, NULLIF($15,'')::jsonb
		)`,
		e.EventType, e.ActorType, nilIfEmpty(e.ActorID), e.ActorIP, nilIfEmpty(e.ActorUA),
		nilIfEmpty(e.ResourceType), nilIfEmpty(e.ResourceID), e.Action, e.Outcome, nilIfEmpty(e.Reason),
		jsonStr(before), jsonStr(after), nilIfEmpty(e.RequestID), nilIfEmpty(e.SessionID), jsonStr(meta),
	)
	return err
}

// Sanitize removes sensitive fields from a struct before storing in audit log.
// Pass a map with only the fields you want to record.
func Sanitize(data map[string]any, exclude ...string) map[string]any {
	excSet := make(map[string]struct{}, len(exclude))
	for _, k := range exclude {
		excSet[k] = struct{}{}
	}
	result := make(map[string]any, len(data))
	for k, v := range data {
		if _, skip := excSet[k]; !skip {
			result[k] = v
		}
	}
	return result
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// jsonStr converts marshalled JSON bytes to string for SimpleProtocol compatibility.
// Returns empty string (handled by NULLIF in SQL) when null/empty.
func jsonStr(b []byte) string {
	s := string(b)
	if s == "" || s == "null" {
		return ""
	}
	return s
}

package admin

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type auditEntry struct {
	ID           int64           `json:"id"`
	EventTime    time.Time       `json:"event_time"`
	EventType    string          `json:"event_type"`
	ActorType    string          `json:"actor_type"`
	ActorID      *string         `json:"actor_id"`
	ActorIP      *string         `json:"actor_ip"`
	ResourceType *string         `json:"resource_type"`
	ResourceID   *string         `json:"resource_id"`
	Action       string          `json:"action"`
	Outcome      string          `json:"outcome"`
	Reason       *string         `json:"reason"`
	BeforeState  json.RawMessage `json:"before_state,omitempty"`
	AfterState   json.RawMessage `json:"after_state,omitempty"`
	RequestID    *string         `json:"request_id"`
}

// ── Shared WHERE builder ───────────────────────────────────────────────────────

func buildAuditWhere(c *fiber.Ctx) (where string, args []any, idx int) {
	idx = 1
	var conds []string

	add := func(col, val string) {
		if val != "" {
			conds = append(conds, fmt.Sprintf("%s = $%d", col, idx))
			args = append(args, val)
			idx++
		}
	}

	add("actor_id", c.Query("actor_id"))
	add("resource_type", c.Query("resource_type"))
	add("resource_id", c.Query("resource_id"))
	add("event_type", c.Query("event_type"))
	add("outcome", c.Query("outcome"))
	add("actor_type", c.Query("actor_type"))

	if from := c.Query("from"); from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			conds = append(conds, fmt.Sprintf("event_time >= $%d", idx))
			args = append(args, t)
			idx++
		}
	}
	if to := c.Query("to"); to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			conds = append(conds, fmt.Sprintf("event_time < $%d", idx))
			args = append(args, t.AddDate(0, 0, 1))
			idx++
		}
	}
	if q := c.Query("q"); q != "" {
		conds = append(conds, fmt.Sprintf(
			"(actor_id ILIKE $%d OR resource_id ILIKE $%d OR actor_ip::text ILIKE $%d OR reason ILIKE $%d)",
			idx, idx, idx, idx,
		))
		args = append(args, "%"+q+"%")
		idx++
	}

	where = "1=1"
	if len(conds) > 0 {
		where = strings.Join(conds, " AND ")
	}
	return
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// AuditLog GET /api/admin/audit — paginated list
func AuditLog(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page := max(c.QueryInt("page", 1), 1)
		limit := min(c.QueryInt("limit", 50), 200)
		offset := (page - 1) * limit

		where, args, idx := buildAuditWhere(c)

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		var total int
		if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log WHERE `+where, args...).Scan(&total); err != nil {
			log.Printf("[audit] count error: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		dataArgs := append(args, limit, offset)
		rows, err := db.Query(ctx, fmt.Sprintf(`
			SELECT id, event_time, event_type, actor_type, actor_id,
			       actor_ip::text, resource_type, resource_id,
			       action, outcome, reason,
			       before_state::text, after_state::text, request_id
			FROM audit_log
			WHERE %s
			ORDER BY event_time DESC
			LIMIT $%d OFFSET $%d
		`, where, idx, idx+1), dataArgs...)
		if err != nil {
			log.Printf("[audit] query error: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		entries := []auditEntry{}
		for rows.Next() {
			var e auditEntry
			var before, after *string
			if err := rows.Scan(
				&e.ID, &e.EventTime, &e.EventType, &e.ActorType, &e.ActorID,
				&e.ActorIP, &e.ResourceType, &e.ResourceID,
				&e.Action, &e.Outcome, &e.Reason,
				&before, &after, &e.RequestID,
			); err != nil {
				log.Printf("[audit] scan error: %v", err)
				continue
			}
			if before != nil {
				e.BeforeState = json.RawMessage(*before)
			}
			if after != nil {
				e.AfterState = json.RawMessage(*after)
			}
			entries = append(entries, e)
		}

		return c.JSON(fiber.Map{
			"data":  entries,
			"total": total,
			"page":  page,
			"limit": limit,
		})
	}
}

// AuditStats GET /api/admin/audit/stats — aggregate metrics for dashboard
func AuditStats(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		where, args, idx := buildAuditWhere(c)

		// Default to last 30 days if no date filter
		if c.Query("from") == "" && c.Query("to") == "" {
			where += fmt.Sprintf(" AND event_time >= $%d", idx)
			args = append(args, time.Now().AddDate(0, 0, -30))
			idx++
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Outcome breakdown
		type outcomeStat struct {
			Outcome string `json:"outcome"`
			Count   int    `json:"count"`
		}
		outRows, err := db.Query(ctx, `
			SELECT outcome, COUNT(*) FROM audit_log WHERE `+where+`
			GROUP BY outcome ORDER BY COUNT(*) DESC
		`, args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer outRows.Close()
		byOutcome := []outcomeStat{}
		for outRows.Next() {
			var s outcomeStat
			if outRows.Scan(&s.Outcome, &s.Count) == nil {
				byOutcome = append(byOutcome, s)
			}
		}
		outRows.Close()

		// Top event types (top 8)
		type eventStat struct {
			EventType string `json:"event_type"`
			Count     int    `json:"count"`
		}
		evRows, err := db.Query(ctx, `
			SELECT event_type, COUNT(*) FROM audit_log WHERE `+where+`
			GROUP BY event_type ORDER BY COUNT(*) DESC LIMIT 8
		`, args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer evRows.Close()
		byEventType := []eventStat{}
		for evRows.Next() {
			var s eventStat
			if evRows.Scan(&s.EventType, &s.Count) == nil {
				byEventType = append(byEventType, s)
			}
		}
		evRows.Close()

		// By day (last 30 days)
		type dayStat struct {
			Date     string `json:"date"`
			Total    int    `json:"total"`
			Failures int    `json:"failures"`
		}
		dayRows, err := db.Query(ctx, `
			SELECT
				DATE(event_time) as d,
				COUNT(*) as total,
				COUNT(*) FILTER (WHERE outcome != 'success') as failures
			FROM audit_log
			WHERE `+where+`
			GROUP BY d ORDER BY d ASC
		`, args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer dayRows.Close()
		byDay := []dayStat{}
		for dayRows.Next() {
			var s dayStat
			var d time.Time
			if dayRows.Scan(&d, &s.Total, &s.Failures) == nil {
				s.Date = d.Format("2006-01-02")
				byDay = append(byDay, s)
			}
		}
		dayRows.Close()

		// Total
		var total int
		db.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log WHERE `+where, args...).Scan(&total) //nolint

		return c.JSON(fiber.Map{
			"by_outcome":    byOutcome,
			"by_event_type": byEventType,
			"by_day":        byDay,
			"total":         total,
		})
	}
}

// AuditExport GET /api/admin/audit/export — CSV download (max 10k rows)
func AuditExport(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		where, args, idx := buildAuditWhere(c)

		exportLimit := 10_000
		args = append(args, exportLimit)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, fmt.Sprintf(`
			SELECT id, event_time, event_type, actor_type, actor_id,
			       actor_ip::text, resource_type, resource_id,
			       action, outcome, reason, request_id
			FROM audit_log
			WHERE %s
			ORDER BY event_time DESC
			LIMIT $%d
		`, where, idx), args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		filename := fmt.Sprintf("audit-export-%s.csv", time.Now().Format("20060102-150405"))
		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", `attachment; filename="`+filename+`"`)

		w := csv.NewWriter(c.Response().BodyWriter())
		_ = w.Write([]string{
			"id", "event_time", "event_type", "actor_type", "actor_id",
			"actor_ip", "resource_type", "resource_id",
			"action", "outcome", "reason", "request_id",
		})

		for rows.Next() {
			var (
				id                                               int64
				eventTime                                        time.Time
				eventType, actorType, action, outcome            string
				actorID, actorIP, resourceType, resourceID       *string
				reason, requestID                                *string
			)
			if err := rows.Scan(
				&id, &eventTime, &eventType, &actorType, &actorID,
				&actorIP, &resourceType, &resourceID,
				&action, &outcome, &reason, &requestID,
			); err != nil {
				continue
			}
			_ = w.Write([]string{
				fmt.Sprintf("%d", id),
				eventTime.UTC().Format(time.RFC3339),
				eventType, actorType,
				derefStr(actorID), derefStr(actorIP),
				derefStr(resourceType), derefStr(resourceID),
				action, outcome,
				derefStr(reason), derefStr(requestID),
			})
		}
		w.Flush()
		return nil
	}
}

// ── Security Events ───────────────────────────────────────────────────────────

type secEvent struct {
	EventTime time.Time `json:"event_time"`
	EventType string    `json:"event_type"`
	SourceIP  string    `json:"source_ip"`
	Target    *string   `json:"target"`
	Count     int       `json:"count"`
	Blocked   bool      `json:"blocked"`
}

func buildSecurityWhere(c *fiber.Ctx) (where string, args []any, idx int) {
	idx = 1
	var conds []string

	days := c.QueryInt("days", 7)
	if days > 90 {
		days = 90
	}
	conds = append(conds, fmt.Sprintf("event_time > NOW() - INTERVAL '%d days'", days))

	if et := c.Query("event_type"); et != "" {
		conds = append(conds, fmt.Sprintf("event_type = $%d", idx))
		args = append(args, et)
		idx++
	}
	if b := c.Query("blocked"); b == "true" {
		conds = append(conds, "blocked = true")
	} else if b == "false" {
		conds = append(conds, "blocked = false")
	}
	if ip := c.Query("ip"); ip != "" {
		conds = append(conds, fmt.Sprintf("source_ip::text ILIKE $%d", idx))
		args = append(args, "%"+ip+"%")
		idx++
	}

	where = strings.Join(conds, " AND ")
	return
}

// SecurityEvents GET /api/admin/security/events
func SecurityEvents(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		limit := min(c.QueryInt("limit", 200), 500)
		where, args, idx := buildSecurityWhere(c)
		args = append(args, limit)

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, fmt.Sprintf(`
			SELECT event_time, event_type, source_ip::text, target, count, blocked
			FROM security_event
			WHERE %s
			ORDER BY event_time DESC
			LIMIT $%d
		`, where, idx), args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		events := []secEvent{}
		for rows.Next() {
			var e secEvent
			if err := rows.Scan(&e.EventTime, &e.EventType, &e.SourceIP, &e.Target, &e.Count, &e.Blocked); err != nil {
				continue
			}
			events = append(events, e)
		}

		// Stats by day
		type dayStat struct {
			Date    string `json:"date"`
			Total   int    `json:"total"`
			Blocked int    `json:"blocked"`
		}
		dayRows, _ := db.Query(ctx, `
			SELECT DATE(event_time) as d, COUNT(*), COUNT(*) FILTER (WHERE blocked)
			FROM security_event
			WHERE `+where+`
			GROUP BY d ORDER BY d ASC
		`, args[:len(args)-1]...)
		byDay := []dayStat{}
		if dayRows != nil {
			defer dayRows.Close()
			for dayRows.Next() {
				var s dayStat
				var d time.Time
				if dayRows.Scan(&d, &s.Total, &s.Blocked) == nil {
					s.Date = d.Format("2006-01-02")
					byDay = append(byDay, s)
				}
			}
		}

		return c.JSON(fiber.Map{
			"data":   events,
			"count":  len(events),
			"by_day": byDay,
		})
	}
}

// SecurityExport GET /api/admin/security/export — CSV
func SecurityExport(db *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		where, args, idx := buildSecurityWhere(c)
		args = append(args, 10_000)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, fmt.Sprintf(`
			SELECT event_time, event_type, source_ip::text, target, count, blocked
			FROM security_event WHERE %s ORDER BY event_time DESC LIMIT $%d
		`, where, idx), args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		filename := fmt.Sprintf("security-events-%s.csv", time.Now().Format("20060102-150405"))
		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", `attachment; filename="`+filename+`"`)

		w := csv.NewWriter(c.Response().BodyWriter())
		_ = w.Write([]string{"event_time", "event_type", "source_ip", "target", "count", "blocked"})
		for rows.Next() {
			var e secEvent
			if rows.Scan(&e.EventTime, &e.EventType, &e.SourceIP, &e.Target, &e.Count, &e.Blocked) == nil {
				_ = w.Write([]string{
					e.EventTime.UTC().Format(time.RFC3339),
					e.EventType, e.SourceIP,
					derefStr(e.Target),
					fmt.Sprintf("%d", e.Count),
					fmt.Sprintf("%v", e.Blocked),
				})
			}
		}
		w.Flush()
		return nil
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

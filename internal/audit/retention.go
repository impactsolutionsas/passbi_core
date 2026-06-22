package audit

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RetentionConfig defines data retention policies (RGPD Article 5(1)(e)).
type RetentionConfig struct {
	AuditLogMonths     int // minimum 36 months per RGPD accountability requirement
	SecurityEventDays  int // high-volume, shorter retention
	AlertThreshold     int // security_event count in 1 hour to trigger alert
	AlertCallback      func(eventType, sourceIP string, count int)
}

var DefaultRetention = RetentionConfig{
	AuditLogMonths:    36,
	SecurityEventDays: 90,
	AlertThreshold:    20,
	AlertCallback:     defaultAlert,
}

// StartRetentionWorker launches a background goroutine that runs daily.
// It purges expired records and checks for security spikes.
func StartRetentionWorker(ctx context.Context, db *pgxpool.Pool, cfg RetentionConfig) {
	go func() {
		// Run immediately at startup, then every 24h
		runRetention(ctx, db, cfg)

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		// Alert check runs more frequently (every 5 minutes)
		alertTicker := time.NewTicker(5 * time.Minute)
		defer alertTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runRetention(ctx, db, cfg)
			case <-alertTicker.C:
				checkSecurityAlerts(ctx, db, cfg)
			}
		}
	}()
}

func runRetention(ctx context.Context, db *pgxpool.Pool, cfg RetentionConfig) {
	purgeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Purge audit_log older than cfg.AuditLogMonths
	auditCutoff := time.Now().AddDate(0, -cfg.AuditLogMonths, 0)
	tag, err := db.Exec(purgeCtx,
		`DELETE FROM audit_log WHERE event_time < $1`, auditCutoff,
	)
	if err != nil {
		log.Printf("[retention] audit_log purge error: %v", err)
	} else if tag.RowsAffected() > 0 {
		log.Printf("[retention] purged %d audit_log entries older than %d months", tag.RowsAffected(), cfg.AuditLogMonths)
	}

	// Purge security_event older than cfg.SecurityEventDays
	secCutoff := time.Now().AddDate(0, 0, -cfg.SecurityEventDays)
	tag, err = db.Exec(purgeCtx,
		`DELETE FROM security_event WHERE event_time < $1`, secCutoff,
	)
	if err != nil {
		log.Printf("[retention] security_event purge error: %v", err)
	} else if tag.RowsAffected() > 0 {
		log.Printf("[retention] purged %d security_event entries older than %d days", tag.RowsAffected(), cfg.SecurityEventDays)
	}
}

func checkSecurityAlerts(ctx context.Context, db *pgxpool.Pool, cfg RetentionConfig) {
	alertCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Detect IPs with spike in the last hour (brute_force or injection_attempt)
	rows, err := db.Query(alertCtx, `
		SELECT event_type, source_ip::text, COUNT(*) as cnt
		FROM security_event
		WHERE event_time > NOW() - INTERVAL '1 hour'
		  AND event_type IN ('brute_force', 'injection_attempt', 'suspicious_ua')
		GROUP BY event_type, source_ip
		HAVING COUNT(*) >= $1
	`, cfg.AlertThreshold)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var eventType, sourceIP string
		var count int
		if err := rows.Scan(&eventType, &sourceIP, &count); err != nil {
			continue
		}
		if cfg.AlertCallback != nil {
			cfg.AlertCallback(eventType, sourceIP, count)
		}
	}
}

func defaultAlert(eventType, sourceIP string, count int) {
	log.Printf("[SECURITY ALERT] event_type=%s source_ip=%s occurrences=%d in last hour — consider blocking this IP",
		eventType, sourceIP, count)
}

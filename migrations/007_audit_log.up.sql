-- RGPD Audit Trail — append-only, no UPDATE/DELETE allowed via row security
CREATE TABLE audit_log (
    id            BIGSERIAL PRIMARY KEY,
    event_time    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type    TEXT        NOT NULL, -- auth.login, partner.create, apikey.revoke, data.access, etc.
    actor_type    TEXT        NOT NULL, -- 'admin', 'partner', 'system', 'anonymous'
    actor_id      TEXT,                 -- admin_id or partner_id (nullable for anonymous)
    actor_ip      INET,
    actor_ua      TEXT,
    resource_type TEXT,                 -- 'partner', 'api_key', 'usage_log', etc.
    resource_id   TEXT,
    action        TEXT        NOT NULL, -- CREATE, READ, UPDATE, DELETE, LOGIN, LOGOUT, REVOKE
    outcome       TEXT        NOT NULL DEFAULT 'success', -- success, failure, denied
    reason        TEXT,                 -- e.g. "invalid_password", "ip_not_allowed"
    before_state  JSONB,                -- valeur avant mutation (RGPD exige traçabilité)
    after_state   JSONB,                -- valeur après mutation (champs sensibles exclus)
    request_id    TEXT,                 -- X-Request-ID pour corrélation
    session_id    TEXT,                 -- JWT jti si disponible
    metadata      JSONB                 -- données contextuelles supplémentaires
);

-- Index pour les requêtes RGPD fréquentes
CREATE INDEX idx_audit_actor      ON audit_log (actor_id, event_time DESC);
CREATE INDEX idx_audit_resource   ON audit_log (resource_type, resource_id, event_time DESC);
CREATE INDEX idx_audit_event_type ON audit_log (event_type, event_time DESC);
CREATE INDEX idx_audit_time       ON audit_log (event_time DESC);
CREATE INDEX idx_audit_outcome    ON audit_log (outcome, event_time DESC) WHERE outcome != 'success';

-- Row-level security : lecture seulement via l'application (pas de UPDATE/DELETE)
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;

-- Policy : insert autorisé, update/delete interdits pour tous
CREATE POLICY audit_insert_only ON audit_log
    FOR INSERT WITH CHECK (true);

CREATE POLICY audit_select_only ON audit_log
    FOR SELECT USING (true);

-- Commentaire RGPD
COMMENT ON TABLE audit_log IS 'Journal d''audit RGPD — Article 5(2) accountability. Append-only, rétention 36 mois minimum.';

-- Table security_event pour les événements de sécurité bruts (volume élevé → séparé)
CREATE TABLE security_event (
    id          BIGSERIAL PRIMARY KEY,
    event_time  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type  TEXT        NOT NULL, -- brute_force, rate_limit_exceeded, ip_blocked, suspicious_pattern
    source_ip   INET        NOT NULL,
    target      TEXT,                 -- endpoint ou resource ciblé
    count       INT         NOT NULL DEFAULT 1,
    blocked     BOOLEAN     NOT NULL DEFAULT FALSE,
    metadata    JSONB
);

CREATE INDEX idx_security_ip   ON security_event (source_ip, event_time DESC);
CREATE INDEX idx_security_type ON security_event (event_type, event_time DESC);
CREATE INDEX idx_security_time ON security_event (event_time DESC);

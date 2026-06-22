-- Migration: Partner Portal Authentication

ALTER TABLE partner
    ADD COLUMN password_hash VARCHAR(255),
    ADD COLUMN invite_token VARCHAR(255),
    ADD COLUMN invite_expires_at TIMESTAMP,
    ADD COLUMN portal_enabled BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX idx_partner_invite_token ON partner(invite_token) WHERE invite_token IS NOT NULL;

COMMENT ON COLUMN partner.password_hash IS 'Bcrypt hash for partner portal login';
COMMENT ON COLUMN partner.invite_token IS 'One-time token sent by email to set up portal access';
COMMENT ON COLUMN partner.portal_enabled IS 'Whether this partner can log in to the self-service portal';

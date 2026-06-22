ALTER TABLE partner
    DROP COLUMN IF EXISTS password_hash,
    DROP COLUMN IF EXISTS invite_token,
    DROP COLUMN IF EXISTS invite_expires_at,
    DROP COLUMN IF EXISTS portal_enabled;

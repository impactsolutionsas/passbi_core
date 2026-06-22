-- Migration: Admin Users for Backoffice Authentication

CREATE TABLE admin_user (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'admin',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMP,
    CONSTRAINT admin_user_role_check CHECK (role IN ('superadmin', 'admin', 'ops'))
);

CREATE INDEX idx_admin_user_email ON admin_user(email);
CREATE INDEX idx_admin_user_role ON admin_user(role);

CREATE TRIGGER update_admin_user_updated_at
    BEFORE UPDATE ON admin_user
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE admin_user IS 'Internal team accounts for backoffice access';

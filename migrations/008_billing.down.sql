-- Rollback migration 008: Billing System

DROP TABLE IF EXISTS payment      CASCADE;
DROP TABLE IF EXISTS devis_item   CASCADE;
DROP TABLE IF EXISTS devis        CASCADE;
DROP TABLE IF EXISTS invoice_item CASCADE;
DROP TABLE IF EXISTS invoice      CASCADE;

DROP SEQUENCE IF EXISTS invoice_seq;
DROP SEQUENCE IF EXISTS devis_seq;

-- Restore original role constraint (without accountant)
ALTER TABLE admin_user DROP CONSTRAINT admin_user_role_check;
ALTER TABLE admin_user ADD CONSTRAINT admin_user_role_check
    CHECK (role IN ('superadmin', 'admin', 'ops'));

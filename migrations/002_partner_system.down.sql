-- Rollback migration for Partner System

-- Drop triggers
DROP TRIGGER IF EXISTS update_partner_updated_at ON partner;
DROP TRIGGER IF EXISTS update_tier_config_updated_at ON tier_config;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables (in reverse order due to foreign keys)
DROP TABLE IF EXISTS quota_usage;
DROP TABLE IF EXISTS usage_log;
DROP TABLE IF EXISTS api_key;
DROP TABLE IF EXISTS tier_config;
DROP TABLE IF EXISTS partner;

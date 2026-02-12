-- Migration: Partner System for API-as-a-Service
-- Description: Creates tables for partner management, API keys, usage logging, and quotas

-- ============================================
-- Table: partner
-- ============================================
CREATE TABLE partner (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Basic Information
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    company VARCHAR(255),

    -- Status
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    tier VARCHAR(50) NOT NULL DEFAULT 'free',

    -- Rate Limits
    rate_limit_per_second INT NOT NULL DEFAULT 10,
    rate_limit_per_day INT NOT NULL DEFAULT 10000,
    rate_limit_per_month INT NOT NULL DEFAULT 300000,

    -- Configuration
    allowed_origins TEXT[],
    webhook_url VARCHAR(500),

    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_active_at TIMESTAMP,

    -- Contact Information
    contact_name VARCHAR(255),
    contact_phone VARCHAR(50),

    -- Billing Information
    billing_email VARCHAR(255),
    billing_address TEXT,

    -- Constraints
    CONSTRAINT partner_status_check CHECK (status IN ('active', 'suspended', 'inactive')),
    CONSTRAINT partner_tier_check CHECK (tier IN ('free', 'starter', 'business', 'enterprise'))
);

CREATE INDEX idx_partner_status ON partner(status);
CREATE INDEX idx_partner_tier ON partner(tier);
CREATE INDEX idx_partner_email ON partner(email);
CREATE INDEX idx_partner_created_at ON partner(created_at DESC);

-- ============================================
-- Table: api_key
-- ============================================
CREATE TABLE api_key (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL,

    -- API Key (hashed)
    key_hash VARCHAR(255) NOT NULL UNIQUE,
    key_prefix VARCHAR(20) NOT NULL,

    -- Metadata
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Permissions
    scopes TEXT[] NOT NULL DEFAULT ARRAY['read:routes'],

    -- Status
    is_active BOOLEAN NOT NULL DEFAULT true,

    -- Dates
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,

    -- Security
    allowed_ips INET[],

    -- Foreign Key
    CONSTRAINT fk_api_key_partner FOREIGN KEY (partner_id)
        REFERENCES partner(id) ON DELETE CASCADE
);

CREATE INDEX idx_api_key_partner ON api_key(partner_id);
CREATE INDEX idx_api_key_hash ON api_key(key_hash);
CREATE INDEX idx_api_key_active ON api_key(is_active);
CREATE INDEX idx_api_key_partner_active ON api_key(partner_id, is_active);

-- ============================================
-- Table: usage_log
-- ============================================
CREATE TABLE usage_log (
    id BIGSERIAL PRIMARY KEY,

    -- Identification
    partner_id UUID NOT NULL,
    api_key_id UUID NOT NULL,

    -- Request Details
    endpoint VARCHAR(255) NOT NULL,
    method VARCHAR(10) NOT NULL,

    -- Performance Metrics
    response_time_ms INT NOT NULL,
    response_status INT NOT NULL,

    -- Route Details (optional)
    from_location POINT,
    to_location POINT,

    -- Cache Information
    cache_hit BOOLEAN DEFAULT false,

    -- Timestamp
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Client Information
    ip_address INET,
    user_agent TEXT,

    -- Foreign Keys
    CONSTRAINT fk_usage_log_partner FOREIGN KEY (partner_id)
        REFERENCES partner(id) ON DELETE CASCADE,
    CONSTRAINT fk_usage_log_api_key FOREIGN KEY (api_key_id)
        REFERENCES api_key(id) ON DELETE CASCADE
);

-- Indexes for analytics queries
CREATE INDEX idx_usage_partner_timestamp ON usage_log(partner_id, timestamp DESC);
CREATE INDEX idx_usage_timestamp ON usage_log(timestamp DESC);
CREATE INDEX idx_usage_endpoint ON usage_log(endpoint);
CREATE INDEX idx_usage_partner_endpoint ON usage_log(partner_id, endpoint);
CREATE INDEX idx_usage_status ON usage_log(response_status);

-- ============================================
-- Table: quota_usage
-- ============================================
CREATE TABLE quota_usage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL,

    -- Period
    period_type VARCHAR(20) NOT NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,

    -- Counters
    requests_count BIGINT NOT NULL DEFAULT 0,
    successful_requests BIGINT NOT NULL DEFAULT 0,
    failed_requests BIGINT NOT NULL DEFAULT 0,

    -- Costs (for billing in cents)
    cost_cents BIGINT NOT NULL DEFAULT 0,

    -- Metadata
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Foreign Key
    CONSTRAINT fk_quota_usage_partner FOREIGN KEY (partner_id)
        REFERENCES partner(id) ON DELETE CASCADE,

    -- Unique constraint for period
    UNIQUE(partner_id, period_type, period_start),

    -- Check constraint
    CONSTRAINT quota_period_type_check CHECK (period_type IN ('daily', 'monthly'))
);

CREATE INDEX idx_quota_partner_period ON quota_usage(partner_id, period_type, period_start DESC);

-- ============================================
-- Table: tier_config
-- ============================================
CREATE TABLE tier_config (
    tier VARCHAR(50) PRIMARY KEY,

    -- Pricing
    price_cents INT NOT NULL,

    -- Rate Limits
    rate_limit_per_second INT NOT NULL,
    rate_limit_per_day INT NOT NULL,
    rate_limit_per_month INT NOT NULL,

    -- Features (JSON)
    features JSONB NOT NULL,

    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Check constraint
    CONSTRAINT tier_config_tier_check CHECK (tier IN ('free', 'starter', 'business', 'enterprise'))
);

-- Insert default tier configurations
INSERT INTO tier_config (tier, price_cents, rate_limit_per_second, rate_limit_per_day, rate_limit_per_month, features) VALUES
('free', 0, 2, 1000, 30000,
    '{"support": "community", "sla": null, "custom_domain": false, "webhooks": false, "max_api_keys": 2}'::jsonb),
('starter', 4900, 10, 10000, 300000,
    '{"support": "email", "sla": "99%", "custom_domain": false, "webhooks": true, "max_api_keys": 5}'::jsonb),
('business', 19900, 50, 50000, 1500000,
    '{"support": "email+chat", "sla": "99.5%", "custom_domain": true, "webhooks": true, "max_api_keys": 20}'::jsonb),
('enterprise', 0, 1000, -1, -1,
    '{"support": "dedicated", "sla": "99.9%", "custom_domain": true, "webhooks": true, "custom_features": true, "max_api_keys": -1}'::jsonb);

-- ============================================
-- Functions and Triggers
-- ============================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger for partner table
CREATE TRIGGER update_partner_updated_at
    BEFORE UPDATE ON partner
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Trigger for tier_config table
CREATE TRIGGER update_tier_config_updated_at
    BEFORE UPDATE ON tier_config
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================
-- Comments for documentation
-- ============================================
COMMENT ON TABLE partner IS 'Stores partner/client information for API-as-a-Service access';
COMMENT ON TABLE api_key IS 'Stores hashed API keys for authentication';
COMMENT ON TABLE usage_log IS 'Logs all API requests for analytics and billing';
COMMENT ON TABLE quota_usage IS 'Tracks usage quotas per partner per period';
COMMENT ON TABLE tier_config IS 'Configuration for different service tiers';

COMMENT ON COLUMN api_key.key_hash IS 'SHA-256 hash of the API key for secure storage';
COMMENT ON COLUMN api_key.key_prefix IS 'Visible prefix of the key (e.g., pk_live_abc...) for display purposes';
COMMENT ON COLUMN api_key.scopes IS 'Array of permission scopes (e.g., read:routes, write:data)';

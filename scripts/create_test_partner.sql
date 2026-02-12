-- Script to create a test partner with API key
-- Run this after migrations are complete

BEGIN;

-- 1. Create a test partner
INSERT INTO partner (
    id,
    name,
    email,
    company,
    status,
    tier,
    rate_limit_per_second,
    rate_limit_per_day,
    rate_limit_per_month
) VALUES (
    gen_random_uuid(),
    'Test Partner',
    'test@passbi.com',
    'PassBI Test Company',
    'active',
    'free',
    2,      -- 2 requests per second
    1000,   -- 1000 requests per day
    30000   -- 30000 requests per month
) RETURNING id, name, email;

-- Store the partner ID for the next step
-- Replace 'PARTNER_ID_HERE' below with the ID returned above

-- 2. Create an API key for the test partner
-- First, generate a key using: go run scripts/generate_api_key.go -env=test
-- Then insert the hash and prefix below

/*
INSERT INTO api_key (
    partner_id,
    key_hash,
    key_prefix,
    name,
    description,
    scopes,
    is_active
) VALUES (
    'PARTNER_ID_HERE',  -- Replace with partner ID from step 1
    'KEY_HASH_HERE',    -- Replace with hash from generate_api_key.go
    'KEY_PREFIX_HERE',  -- Replace with prefix from generate_api_key.go
    'Test API Key',
    'API key for testing the partner system',
    ARRAY['read:routes', 'read:stops'],
    true
) RETURNING id, key_prefix, created_at;
*/

COMMIT;

-- 3. Verify the setup
SELECT
    p.id as partner_id,
    p.name,
    p.email,
    p.tier,
    p.rate_limit_per_day,
    COUNT(ak.id) as api_keys_count
FROM partner p
LEFT JOIN api_key ak ON ak.partner_id = p.id
WHERE p.email = 'test@passbi.com'
GROUP BY p.id, p.name, p.email, p.tier, p.rate_limit_per_day;

-- 4. Show API keys
SELECT
    ak.id,
    ak.key_prefix,
    ak.name,
    ak.scopes,
    ak.is_active,
    ak.created_at
FROM api_key ak
JOIN partner p ON p.id = ak.partner_id
WHERE p.email = 'test@passbi.com';

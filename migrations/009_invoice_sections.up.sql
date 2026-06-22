-- Migration 009: Invoice sections + XOF pricing

-- Section grouping for invoice/devis items (Supabase-style breakdown)
ALTER TABLE invoice_item ADD COLUMN section VARCHAR(100);
ALTER TABLE devis_item   ADD COLUMN section VARCHAR(100);

-- XOF pricing in tier_config (price_cents is USD cents, price_xof is FCFA)
ALTER TABLE tier_config ADD COLUMN price_xof BIGINT NOT NULL DEFAULT 0;

UPDATE tier_config SET price_xof = 0       WHERE tier = 'free';
UPDATE tier_config SET price_xof = 29700   WHERE tier = 'starter';    -- ~$49 USD × 605 XOF/$
UPDATE tier_config SET price_xof = 120400  WHERE tier = 'business';   -- ~$199 USD × 605 XOF/$
UPDATE tier_config SET price_xof = 0       WHERE tier = 'enterprise'; -- custom pricing

COMMENT ON COLUMN invoice_item.section  IS 'Section header for grouping (e.g. "Abonnement", "Appels API", "Dépassement")';
COMMENT ON COLUMN tier_config.price_xof IS 'Monthly subscription price in XOF (FCFA). 0 = free or custom.';

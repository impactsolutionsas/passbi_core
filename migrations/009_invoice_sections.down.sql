ALTER TABLE tier_config  DROP COLUMN IF EXISTS price_xof;
ALTER TABLE devis_item   DROP COLUMN IF EXISTS section;
ALTER TABLE invoice_item DROP COLUMN IF EXISTS section;

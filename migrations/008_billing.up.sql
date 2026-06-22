-- Migration 008: Billing System (invoices, devis, payments + accountant role)

-- ============================================
-- Role: accountant (can view billing & revenue)
-- ============================================
ALTER TABLE admin_user DROP CONSTRAINT admin_user_role_check;
ALTER TABLE admin_user ADD CONSTRAINT admin_user_role_check
    CHECK (role IN ('superadmin', 'admin', 'ops', 'accountant'));

-- ============================================
-- Table: invoice
-- ============================================
CREATE TABLE invoice (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_number  VARCHAR(30)  NOT NULL UNIQUE,  -- e.g. INV-2026-0042

    -- Parties
    partner_id      UUID         NOT NULL REFERENCES partner(id) ON DELETE RESTRICT,
    created_by      UUID         NOT NULL REFERENCES admin_user(id) ON DELETE RESTRICT,

    -- Status
    status          VARCHAR(20)  NOT NULL DEFAULT 'draft',
    -- draft | sent | paid | overdue | cancelled | refunded

    -- Currency & Amounts (smallest unit: XOF=FCFA integer, USD/EUR=cents)
    currency        VARCHAR(3)   NOT NULL DEFAULT 'XOF',
    subtotal        BIGINT       NOT NULL DEFAULT 0,
    tax_rate        NUMERIC(5,2) NOT NULL DEFAULT 0,
    tax_amount      BIGINT       NOT NULL DEFAULT 0,
    total           BIGINT       NOT NULL DEFAULT 0,

    -- Optional USD equivalent (informational)
    usd_rate        NUMERIC(12,6),          -- XOF/USD rate at creation time
    usd_total       BIGINT,                 -- total in USD cents

    -- Dates
    due_date        DATE,
    sent_at         TIMESTAMP,
    paid_at         TIMESTAMP,

    -- Content
    notes           TEXT,
    payment_terms   TEXT,

    -- Metadata
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP    NOT NULL DEFAULT NOW(),

    CONSTRAINT invoice_status_check CHECK (status IN ('draft','sent','paid','overdue','cancelled','refunded')),
    CONSTRAINT invoice_currency_check CHECK (currency IN ('XOF','USD','EUR'))
);

CREATE INDEX idx_invoice_partner    ON invoice(partner_id);
CREATE INDEX idx_invoice_status     ON invoice(status);
CREATE INDEX idx_invoice_created_at ON invoice(created_at DESC);
CREATE INDEX idx_invoice_due_date   ON invoice(due_date) WHERE status IN ('sent','overdue');

-- ============================================
-- Table: invoice_item
-- ============================================
CREATE TABLE invoice_item (
    id          UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id  UUID    NOT NULL REFERENCES invoice(id) ON DELETE CASCADE,
    position    SMALLINT NOT NULL DEFAULT 1,
    description TEXT    NOT NULL,
    quantity    INT     NOT NULL DEFAULT 1,
    unit_price  BIGINT  NOT NULL,
    amount      BIGINT  NOT NULL  -- quantity * unit_price (computed on insert/update)
);

CREATE INDEX idx_invoice_item_invoice ON invoice_item(invoice_id);

-- ============================================
-- Table: devis (quotes)
-- ============================================
CREATE TABLE devis (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    devis_number    VARCHAR(30)  NOT NULL UNIQUE,  -- e.g. DEV-2026-0007

    -- Parties
    partner_id      UUID         NOT NULL REFERENCES partner(id) ON DELETE RESTRICT,
    created_by      UUID         NOT NULL REFERENCES admin_user(id) ON DELETE RESTRICT,

    -- Status
    status          VARCHAR(20)  NOT NULL DEFAULT 'draft',
    -- draft | sent | accepted | declined | expired | converted

    -- Currency & Amounts
    currency        VARCHAR(3)   NOT NULL DEFAULT 'XOF',
    subtotal        BIGINT       NOT NULL DEFAULT 0,
    tax_rate        NUMERIC(5,2) NOT NULL DEFAULT 0,
    tax_amount      BIGINT       NOT NULL DEFAULT 0,
    total           BIGINT       NOT NULL DEFAULT 0,

    -- Optional USD equivalent
    usd_rate        NUMERIC(12,6),
    usd_total       BIGINT,

    -- Dates
    valid_until     DATE,
    sent_at         TIMESTAMP,
    responded_at    TIMESTAMP,

    -- Conversion to invoice
    converted_invoice_id UUID REFERENCES invoice(id) ON DELETE SET NULL,

    -- Content
    notes           TEXT,
    terms           TEXT,

    -- Metadata
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP    NOT NULL DEFAULT NOW(),

    CONSTRAINT devis_status_check CHECK (status IN ('draft','sent','accepted','declined','expired','converted')),
    CONSTRAINT devis_currency_check CHECK (currency IN ('XOF','USD','EUR'))
);

CREATE INDEX idx_devis_partner    ON devis(partner_id);
CREATE INDEX idx_devis_status     ON devis(status);
CREATE INDEX idx_devis_created_at ON devis(created_at DESC);

-- ============================================
-- Table: devis_item
-- ============================================
CREATE TABLE devis_item (
    id          UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    devis_id    UUID    NOT NULL REFERENCES devis(id) ON DELETE CASCADE,
    position    SMALLINT NOT NULL DEFAULT 1,
    description TEXT    NOT NULL,
    quantity    INT     NOT NULL DEFAULT 1,
    unit_price  BIGINT  NOT NULL,
    amount      BIGINT  NOT NULL
);

CREATE INDEX idx_devis_item_devis ON devis_item(devis_id);

-- ============================================
-- Table: payment
-- ============================================
CREATE TABLE payment (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id  UUID NOT NULL REFERENCES invoice(id) ON DELETE RESTRICT,
    created_by  UUID NOT NULL REFERENCES admin_user(id) ON DELETE RESTRICT,

    amount      BIGINT       NOT NULL,
    currency    VARCHAR(3)   NOT NULL DEFAULT 'XOF',

    -- Payment method
    method      VARCHAR(30)  NOT NULL DEFAULT 'bank_transfer',
    -- bank_transfer | mobile_money | card | cash | other

    reference   VARCHAR(255),  -- external payment reference / receipt number
    notes       TEXT,

    paid_at     TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW(),

    CONSTRAINT payment_currency_check CHECK (currency IN ('XOF','USD','EUR')),
    CONSTRAINT payment_method_check   CHECK (method IN ('bank_transfer','mobile_money','card','cash','other'))
);

CREATE INDEX idx_payment_invoice    ON payment(invoice_id);
CREATE INDEX idx_payment_paid_at    ON payment(paid_at DESC);

-- ============================================
-- Sequence helpers for invoice/devis numbers
-- ============================================
CREATE SEQUENCE invoice_seq START 1;
CREATE SEQUENCE devis_seq   START 1;

-- ============================================
-- Triggers: updated_at
-- ============================================
CREATE TRIGGER update_invoice_updated_at
    BEFORE UPDATE ON invoice FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_devis_updated_at
    BEFORE UPDATE ON devis FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================
-- Comments
-- ============================================
COMMENT ON TABLE invoice      IS 'Client invoices (factures) — multi-currency XOF/USD/EUR';
COMMENT ON TABLE invoice_item IS 'Line items for invoices';
COMMENT ON TABLE devis        IS 'Quotes/devis — convertible to invoice';
COMMENT ON TABLE devis_item   IS 'Line items for devis';
COMMENT ON TABLE payment      IS 'Payment records attached to invoices';
COMMENT ON COLUMN invoice.subtotal  IS 'Sum of items before tax, in smallest currency unit (XOF: FCFA, USD/EUR: cents)';
COMMENT ON COLUMN invoice.usd_rate  IS 'Exchange rate XOF/USD at invoice creation, for informational display';

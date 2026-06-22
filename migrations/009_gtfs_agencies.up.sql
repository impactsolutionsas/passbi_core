-- Dynamic agency registry — replaces hard-coded knownFeeds
CREATE TABLE gtfs_agency (
    id          TEXT        PRIMARY KEY,
    label       TEXT        NOT NULL,
    country     TEXT        NOT NULL DEFAULT 'senegal',
    city        TEXT        NOT NULL DEFAULT 'dakar',
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    notes       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_gtfs_agency_country_city ON gtfs_agency(country, city);

-- Version tracking: each uploaded ZIP = one immutable version
CREATE TABLE gtfs_version (
    id               BIGSERIAL   PRIMARY KEY,
    agency_id        TEXT        NOT NULL REFERENCES gtfs_agency(id) ON DELETE CASCADE,
    version_number   INT         NOT NULL,
    filename         TEXT        NOT NULL,
    storage_path     TEXT        NOT NULL,
    file_size_bytes  BIGINT,
    sha256           TEXT,
    uploaded_by      TEXT,
    uploaded_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes            TEXT,
    status           TEXT        NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'active', 'archived', 'failed')),
    import_log_id    BIGINT      REFERENCES import_log(id)
);

CREATE UNIQUE INDEX idx_gtfs_version_agency_num ON gtfs_version(agency_id, version_number);
CREATE INDEX idx_gtfs_version_agency            ON gtfs_version(agency_id);
CREATE INDEX idx_gtfs_version_status            ON gtfs_version(status);

-- Link each import run to the version it came from
ALTER TABLE import_log
    ADD COLUMN IF NOT EXISTS gtfs_version_id BIGINT REFERENCES gtfs_version(id);

-- Seed existing agencies
INSERT INTO gtfs_agency (id, label, country, city) VALUES
    ('dakar_ter',      'TER (Train Express Régional)', 'senegal', 'dakar'),
    ('dakar_brt',      'BRT (Bus Rapid Transit)',       'senegal', 'dakar'),
    ('dakar_dem_dikk', 'Dem Dikk',                     'senegal', 'dakar'),
    ('dakar_aftu',     'AFTU',                         'senegal', 'dakar')
ON CONFLICT DO NOTHING;

-- Index for case-insensitive stop name search
CREATE INDEX idx_stop_name_lower ON stop (lower(name));

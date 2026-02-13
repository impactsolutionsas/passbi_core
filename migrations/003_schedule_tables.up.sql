-- Calendar table: weekly service patterns from calendar.txt
CREATE TABLE calendar (
    service_id TEXT NOT NULL,
    agency_id TEXT NOT NULL,
    monday    BOOLEAN NOT NULL DEFAULT false,
    tuesday   BOOLEAN NOT NULL DEFAULT false,
    wednesday BOOLEAN NOT NULL DEFAULT false,
    thursday  BOOLEAN NOT NULL DEFAULT false,
    friday    BOOLEAN NOT NULL DEFAULT false,
    saturday  BOOLEAN NOT NULL DEFAULT false,
    sunday    BOOLEAN NOT NULL DEFAULT false,
    start_date DATE NOT NULL,
    end_date   DATE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (agency_id, service_id)
);

CREATE INDEX idx_calendar_service ON calendar(service_id);
CREATE INDEX idx_calendar_agency ON calendar(agency_id);

-- Calendar date table: exceptions from calendar_dates.txt
-- exception_type: 1=service added, 2=service removed
CREATE TABLE calendar_date (
    service_id     TEXT NOT NULL,
    agency_id      TEXT NOT NULL,
    date           DATE NOT NULL,
    exception_type INT NOT NULL CHECK (exception_type IN (1, 2)),
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (agency_id, service_id, date)
);

CREATE INDEX idx_calendar_date_service ON calendar_date(service_id);
CREATE INDEX idx_calendar_date_date ON calendar_date(date);
CREATE INDEX idx_calendar_date_agency ON calendar_date(agency_id);

-- Trip table: trips from trips.txt
CREATE TABLE trip (
    trip_id    TEXT NOT NULL,
    agency_id  TEXT NOT NULL,
    route_id   TEXT NOT NULL REFERENCES route(id) ON DELETE CASCADE,
    service_id TEXT NOT NULL,
    headsign   TEXT,
    direction  INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (agency_id, trip_id)
);

CREATE INDEX idx_trip_route ON trip(route_id);
CREATE INDEX idx_trip_service ON trip(service_id);
CREATE INDEX idx_trip_agency ON trip(agency_id);
CREATE INDEX idx_trip_route_service ON trip(route_id, service_id);

-- Stop time table: stop_times from stop_times.txt
-- arrival_seconds/departure_seconds store GTFS time as seconds-since-midnight
-- Values >= 86400 represent next-day service (GTFS allows 25:00:00 etc.)
CREATE TABLE stop_time (
    id                BIGSERIAL PRIMARY KEY,
    trip_id           TEXT NOT NULL,
    agency_id         TEXT NOT NULL,
    stop_id           TEXT NOT NULL REFERENCES stop(id) ON DELETE CASCADE,
    stop_sequence     INT NOT NULL,
    arrival_time      TEXT,
    departure_time    TEXT,
    arrival_seconds   INT,
    departure_seconds INT,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (agency_id, trip_id, stop_sequence)
);

CREATE INDEX idx_stop_time_trip ON stop_time(trip_id);
CREATE INDEX idx_stop_time_stop ON stop_time(stop_id);
CREATE INDEX idx_stop_time_agency ON stop_time(agency_id);
CREATE INDEX idx_stop_time_stop_departure ON stop_time(stop_id, departure_seconds);
CREATE INDEX idx_stop_time_trip_seq ON stop_time(trip_id, stop_sequence);

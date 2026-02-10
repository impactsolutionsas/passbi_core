-- Enable PostGIS extension
CREATE EXTENSION IF NOT EXISTS postgis;

-- Stop table: represents physical transit stops
CREATE TABLE stop (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    lat DOUBLE PRECISION NOT NULL,
    lon DOUBLE PRECISION NOT NULL,
    geom GEOGRAPHY(Point, 4326),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Populate geom from lat/lon
CREATE OR REPLACE FUNCTION update_stop_geom()
RETURNS TRIGGER AS $$
BEGIN
    NEW.geom := ST_SetSRID(ST_MakePoint(NEW.lon, NEW.lat), 4326)::GEOGRAPHY;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_stop_geom
BEFORE INSERT OR UPDATE ON stop
FOR EACH ROW
EXECUTE FUNCTION update_stop_geom();

-- Index for spatial queries
CREATE INDEX idx_stop_geom ON stop USING GIST (geom);

-- Route table: represents transit routes (lines)
CREATE TABLE route (
    id TEXT PRIMARY KEY,
    agency_id TEXT NOT NULL,
    short_name TEXT,
    long_name TEXT,
    mode TEXT NOT NULL CHECK (mode IN ('BUS', 'BRT', 'TER', 'FERRY', 'TRAM')),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_route_agency ON route(agency_id);
CREATE INDEX idx_route_mode ON route(mode);

-- Node table: represents (stop, route) pairs in the routing graph
-- Each node is a unique combination of a stop and a route serving that stop
CREATE TABLE node (
    id BIGSERIAL PRIMARY KEY,
    stop_id TEXT NOT NULL REFERENCES stop(id) ON DELETE CASCADE,
    route_id TEXT NOT NULL REFERENCES route(id) ON DELETE CASCADE,
    mode TEXT NOT NULL,
    geom GEOGRAPHY(Point, 4326),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(stop_id, route_id)
);

-- Populate node geom from stop
CREATE OR REPLACE FUNCTION update_node_geom()
RETURNS TRIGGER AS $$
BEGIN
    SELECT geom INTO NEW.geom FROM stop WHERE id = NEW.stop_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_node_geom
BEFORE INSERT OR UPDATE ON node
FOR EACH ROW
EXECUTE FUNCTION update_node_geom();

CREATE INDEX idx_node_geom ON node USING GIST (geom);
CREATE INDEX idx_node_route ON node(route_id);
CREATE INDEX idx_node_stop ON node(stop_id);
CREATE INDEX idx_node_mode ON node(mode);

-- Edge table: represents connections between nodes
CREATE TABLE edge (
    id BIGSERIAL PRIMARY KEY,
    from_node_id BIGINT NOT NULL REFERENCES node(id) ON DELETE CASCADE,
    to_node_id BIGINT NOT NULL REFERENCES node(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('WALK', 'RIDE', 'TRANSFER')),
    cost_time INT NOT NULL CHECK (cost_time >= 0),
    cost_walk INT NOT NULL DEFAULT 0 CHECK (cost_walk >= 0),
    cost_transfer INT NOT NULL DEFAULT 0 CHECK (cost_transfer >= 0),
    trip_id TEXT,
    sequence INT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_edge_from ON edge(from_node_id);
CREATE INDEX idx_edge_to ON edge(to_node_id);
CREATE INDEX idx_edge_type ON edge(type);
CREATE INDEX idx_edge_trip ON edge(trip_id) WHERE trip_id IS NOT NULL;

-- Import log table: tracks GTFS import operations
CREATE TABLE import_log (
    id BIGSERIAL PRIMARY KEY,
    agency_id TEXT NOT NULL,
    started_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    status TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed')),
    stops_count INT DEFAULT 0,
    routes_count INT DEFAULT 0,
    nodes_count INT DEFAULT 0,
    edges_count INT DEFAULT 0,
    error_message TEXT
);

CREATE INDEX idx_import_log_agency ON import_log(agency_id);
CREATE INDEX idx_import_log_status ON import_log(status);
CREATE INDEX idx_import_log_started ON import_log(started_at DESC);

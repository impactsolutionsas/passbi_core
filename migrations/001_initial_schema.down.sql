-- Drop tables in reverse order (respecting foreign key constraints)
DROP TABLE IF EXISTS import_log;
DROP TABLE IF EXISTS edge;
DROP TABLE IF EXISTS node;
DROP TABLE IF EXISTS route;

-- Drop triggers and functions before dropping stop table
DROP TRIGGER IF EXISTS trg_stop_geom ON stop;
DROP TRIGGER IF EXISTS trg_node_geom ON node;
DROP FUNCTION IF EXISTS update_stop_geom();
DROP FUNCTION IF EXISTS update_node_geom();

DROP TABLE IF EXISTS stop;

-- Note: We don't drop PostGIS extension as it might be used by other databases

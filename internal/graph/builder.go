package graph

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/gtfs"
	"github.com/passbi/passbi_core/internal/models"
)

const (
	maxWalkDistance  = 500  // meters
	walkingSpeed     = 1.4  // meters per second
	transferTime     = 180  // seconds (3 minutes)
	batchSize        = 1000 // batch insert size
)

// Builder constructs the routing graph from GTFS data
type Builder struct {
	db *pgxpool.Pool
}

// NewBuilder creates a new graph builder
func NewBuilder(db *pgxpool.Pool) *Builder {
	return &Builder{db: db}
}

// BuildGraph constructs the complete routing graph
// This includes nodes (stop × route) and edges (RIDE, WALK, TRANSFER)
func (b *Builder) BuildGraph(ctx context.Context, feed *gtfs.GTFSFeed) error {
	log.Println("Starting graph construction...")

	// Build nodes first
	nodeCount, err := b.BuildNodes(ctx, feed)
	if err != nil {
		return fmt.Errorf("failed to build nodes: %w", err)
	}
	log.Printf("Created %d nodes", nodeCount)

	// Build edges
	edgeCount, err := b.BuildEdges(ctx, feed)
	if err != nil {
		return fmt.Errorf("failed to build edges: %w", err)
	}
	log.Printf("Created %d edges", edgeCount)

	// Analyze tables for query optimization
	if err := b.analyzeGraph(ctx); err != nil {
		log.Printf("Warning: failed to analyze tables: %v", err)
	}

	log.Println("Graph construction completed successfully")
	return nil
}

// BuildNodes creates nodes for each (stop, route) combination
func (b *Builder) BuildNodes(ctx context.Context, feed *gtfs.GTFSFeed) (int, error) {
	// Build a map of route_id -> mode
	routeModes := make(map[string]models.TransitMode)
	for _, route := range feed.Routes {
		routeModes[route.RouteID] = gtfs.InferMode(route)
	}

	// Build a map of stop_id -> coordinates
	stopCoords := make(map[string]struct{ lat, lon float64 })
	for _, stop := range feed.Stops {
		stopCoords[stop.StopID] = struct{ lat, lon float64 }{lat: stop.Lat, lon: stop.Lon}
	}

	// Build a set of unique (stop_id, route_id) pairs from trips
	type nodeKey struct {
		stopID  string
		routeID string
	}
	nodeSet := make(map[nodeKey]bool)

	// Extract from stop_times via trips
	tripRoutes := make(map[string]string)
	for _, trip := range feed.Trips {
		tripRoutes[trip.TripID] = trip.RouteID
	}

	for _, st := range feed.StopTimes {
		routeID, ok := tripRoutes[st.TripID]
		if !ok {
			continue
		}
		key := nodeKey{stopID: st.StopID, routeID: routeID}
		nodeSet[key] = true
	}

	log.Printf("Found %d unique (stop, route) pairs", len(nodeSet))

	// Batch insert nodes
	batch := &pgx.Batch{}
	count := 0

	for key := range nodeSet {
		mode := routeModes[key.routeID]
		if mode == "" {
			mode = models.ModeBus // default
		}

		coords, ok := stopCoords[key.stopID]
		if !ok {
			log.Printf("Warning: stop %s not found in stops, skipping node", key.stopID)
			continue
		}

		batch.Queue(`
			INSERT INTO node (stop_id, route_id, mode, lat, lon)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (stop_id, route_id) DO NOTHING
		`, key.stopID, key.routeID, mode, coords.lat, coords.lon)

		count++

		if batch.Len() >= batchSize {
			if err := b.executeBatch(ctx, batch); err != nil {
				return 0, err
			}
			batch = &pgx.Batch{}
		}
	}

	// Execute remaining batch
	if batch.Len() > 0 {
		if err := b.executeBatch(ctx, batch); err != nil {
			return 0, err
		}
	}

	return count, nil
}

// BuildEdges creates RIDE, WALK, and TRANSFER edges
func (b *Builder) BuildEdges(ctx context.Context, feed *gtfs.GTFSFeed) (int, error) {
	totalEdges := 0

	// 1. Build RIDE edges (from stop_times)
	rideEdges, err := b.buildRideEdges(ctx, feed)
	if err != nil {
		return 0, fmt.Errorf("failed to build ride edges: %w", err)
	}
	totalEdges += rideEdges
	log.Printf("Created %d RIDE edges", rideEdges)

	// 2. Build WALK edges (nearby stops)
	walkEdges, err := b.buildWalkEdges(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to build walk edges: %w", err)
	}
	totalEdges += walkEdges
	log.Printf("Created %d WALK edges", walkEdges)

	// 3. Build TRANSFER edges (same stop, different routes)
	transferEdges, err := b.buildTransferEdges(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to build transfer edges: %w", err)
	}
	totalEdges += transferEdges
	log.Printf("Created %d TRANSFER edges", transferEdges)

	return totalEdges, nil
}

// BuildGraphFromDB builds the complete routing graph from PostgreSQL database
// This reads ALL agencies' data and reconstructs the entire graph
func (b *Builder) BuildGraphFromDB(ctx context.Context) error {
	log.Println("🔄 Building complete routing graph from database...")

	// 1. Clear existing graph
	if err := b.clearGraph(ctx); err != nil {
		return fmt.Errorf("failed to clear graph: %w", err)
	}

	// 2. Build nodes from database
	nodeCount, err := b.buildNodesFromDB(ctx)
	if err != nil {
		return fmt.Errorf("failed to build nodes: %w", err)
	}
	log.Printf("✅ Created %d nodes", nodeCount)

	// 3. Build edges from database
	edgeCount, err := b.buildEdgesFromDB(ctx)
	if err != nil {
		return fmt.Errorf("failed to build edges: %w", err)
	}
	log.Printf("✅ Created %d edges", edgeCount)

	// 4. Analyze tables for query optimization
	if err := b.analyzeGraph(ctx); err != nil {
		return fmt.Errorf("failed to analyze graph: %w", err)
	}

	log.Println("✅ Graph rebuild complete!")
	return nil
}

// clearGraph removes all nodes and edges
func (b *Builder) clearGraph(ctx context.Context) error {
	log.Println("Clearing existing graph...")

	_, err := b.db.Exec(ctx, "TRUNCATE TABLE edge, node CASCADE")
	if err != nil {
		return err
	}

	log.Println("Graph cleared")
	return nil
}

// buildNodesFromDB creates nodes from all routes and stops in the database
func (b *Builder) buildNodesFromDB(ctx context.Context) (int, error) {
	log.Println("Building nodes from database...")

	// Get all unique (stop_id, route_id, lat, lon) combinations
	// This ensures we have nodes for all stop × route pairs
	query := `
		INSERT INTO node (stop_id, route_id, lat, lon)
		SELECT DISTINCT
			st.stop_id,
			t.route_id,
			s.lat,
			s.lon
		FROM stop_time st
		JOIN trip t ON st.trip_id = t.trip_id
		JOIN stop s ON st.stop_id = s.stop_id
		WHERE s.lat IS NOT NULL AND s.lon IS NOT NULL
		ON CONFLICT (stop_id, route_id) DO NOTHING
	`

	result, err := b.db.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to insert nodes: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// buildEdgesFromDB creates all edges from database data
func (b *Builder) buildEdgesFromDB(ctx context.Context) (int, error) {
	totalEdges := 0

	// 1. Build RIDE edges
	rideEdges, err := b.buildRideEdgesFromDB(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to build ride edges: %w", err)
	}
	totalEdges += rideEdges
	log.Printf("Created %d RIDE edges", rideEdges)

	// 2. Build WALK edges
	walkEdges, err := b.buildWalkEdges(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to build walk edges: %w", err)
	}
	totalEdges += walkEdges
	log.Printf("Created %d WALK edges", walkEdges)

	// 3. Build TRANSFER edges
	transferEdges, err := b.buildTransferEdges(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to build transfer edges: %w", err)
	}
	totalEdges += transferEdges
	log.Printf("Created %d TRANSFER edges", transferEdges)

	return totalEdges, nil
}

// buildRideEdgesFromDB creates RIDE edges from stop_times in database
func (b *Builder) buildRideEdgesFromDB(ctx context.Context) (int, error) {
	log.Println("Building RIDE edges from database...")

	// Create edges between consecutive stops on each trip
	query := `
		INSERT INTO edge (from_node_id, to_node_id, type, cost_time, cost_walk, cost_transfer, trip_id, sequence)
		SELECT
			n1.id as from_node_id,
			n2.id as to_node_id,
			'RIDE' as type,
			GREATEST(
				CASE
					WHEN st1.departure_time IS NOT NULL AND st2.arrival_time IS NOT NULL
					THEN EXTRACT(EPOCH FROM (st2.arrival_time::time - st1.departure_time::time))::INT
					ELSE 300
				END,
				60
			) as cost_time,
			0 as cost_walk,
			0 as cost_transfer,
			st1.trip_id,
			st1.stop_sequence as sequence
		FROM stop_time st1
		JOIN stop_time st2 ON st1.trip_id = st2.trip_id AND st2.stop_sequence = st1.stop_sequence + 1
		JOIN trip t ON st1.trip_id = t.trip_id
		JOIN node n1 ON n1.stop_id = st1.stop_id AND n1.route_id = t.route_id
		JOIN node n2 ON n2.stop_id = st2.stop_id AND n2.route_id = t.route_id
		ON CONFLICT DO NOTHING
	`

	result, err := b.db.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to insert ride edges: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// buildRideEdges creates edges between consecutive stops on the same trip
func (b *Builder) buildRideEdges(ctx context.Context, feed *gtfs.GTFSFeed) (int, error) {
	// Group stop_times by trip and sort by sequence
	tripStops := make(map[string][]models.GTFSStopTime)
	for _, st := range feed.StopTimes {
		tripStops[st.TripID] = append(tripStops[st.TripID], st)
	}

	// Sort each trip's stops by sequence
	for tripID := range tripStops {
		stops := tripStops[tripID]
		sort.Slice(stops, func(i, j int) bool {
			return stops[i].StopSequence < stops[j].StopSequence
		})
		tripStops[tripID] = stops
	}

	// Build route map from trips
	tripRoutes := make(map[string]string)
	for _, trip := range feed.Trips {
		tripRoutes[trip.TripID] = trip.RouteID
	}

	// Create RIDE edges
	batch := &pgx.Batch{}
	count := 0

	for tripID, stops := range tripStops {
		routeID := tripRoutes[tripID]
		if routeID == "" {
			continue
		}

		for i := 0; i < len(stops)-1; i++ {
			fromStop := stops[i]
			toStop := stops[i+1]

			// Calculate time cost
			timeCost := 300 // default 5 minutes if times not available

			if fromStop.DepartureTime != "" && toStop.ArrivalTime != "" {
				fromTime, err1 := gtfs.ParseTimeToSeconds(fromStop.DepartureTime)
				toTime, err2 := gtfs.ParseTimeToSeconds(toStop.ArrivalTime)

				if err1 == nil && err2 == nil && toTime > fromTime {
					timeCost = toTime - fromTime
				}
			}

			// Ensure minimum travel time
			if timeCost < 60 {
				timeCost = 60 // minimum 1 minute
			}

			batch.Queue(`
				INSERT INTO edge (from_node_id, to_node_id, type, cost_time, cost_walk, cost_transfer, trip_id, sequence)
				SELECT n1.id, n2.id, 'RIDE', $1, 0, 0, $2, $3
				FROM node n1
				JOIN node n2 ON n2.stop_id = $5 AND n2.route_id = $6
				WHERE n1.stop_id = $4 AND n1.route_id = $6
				ON CONFLICT DO NOTHING
			`, timeCost, tripID, fromStop.StopSequence, fromStop.StopID, toStop.StopID, routeID)

			count++

			if batch.Len() >= batchSize {
				if err := b.executeBatch(ctx, batch); err != nil {
					return 0, err
				}
				batch = &pgx.Batch{}
			}
		}
	}

	// Execute remaining batch
	if batch.Len() > 0 {
		if err := b.executeBatch(ctx, batch); err != nil {
			return 0, err
		}
	}

	return count, nil
}

// buildWalkEdges creates walking edges between nearby stops
func (b *Builder) buildWalkEdges(ctx context.Context) (int, error) {
	log.Printf("Building WALK edges for stops within %d meters...", maxWalkDistance)

	// Simplified version without PostGIS - uses Haversine formula
	// Note: This is less efficient than PostGIS spatial indexes but works without the extension
	query := `
		INSERT INTO edge (from_node_id, to_node_id, type, cost_time, cost_walk, cost_transfer)
		SELECT
			n1.id,
			n2_with_dist.id,
			'WALK',
			CEIL(n2_with_dist.distance / $1)::INT,
			CEIL(n2_with_dist.distance)::INT,
			0
		FROM node n1
		CROSS JOIN LATERAL (
			SELECT
				n2.id,
				(
					6371000 * acos(
						LEAST(1.0, GREATEST(-1.0,
							cos(radians(n1.lat)) * cos(radians(n2.lat)) *
							cos(radians(n2.lon) - radians(n1.lon)) +
							sin(radians(n1.lat)) * sin(radians(n2.lat))
						))
					)
				) as distance
			FROM node n2
			WHERE n2.id != n1.id
				AND n2.stop_id != n1.stop_id
		) n2_with_dist
		WHERE n2_with_dist.distance <= $2
		ORDER BY n1.id, n2_with_dist.distance
		LIMIT 100000
		ON CONFLICT DO NOTHING
	`

	result, err := b.db.Exec(ctx, query, walkingSpeed, float64(maxWalkDistance))
	if err != nil {
		return 0, err
	}

	return int(result.RowsAffected()), nil
}

// buildTransferEdges creates transfer edges between different routes at the same stop
func (b *Builder) buildTransferEdges(ctx context.Context) (int, error) {
	log.Println("Building TRANSFER edges for same-stop transfers...")

	query := `
		INSERT INTO edge (from_node_id, to_node_id, type, cost_time, cost_walk, cost_transfer)
		SELECT
			n1.id,
			n2.id,
			'TRANSFER',
			$1,
			0,
			1
		FROM node n1
		JOIN node n2 ON n1.stop_id = n2.stop_id AND n1.route_id != n2.route_id
		ON CONFLICT DO NOTHING
	`

	result, err := b.db.Exec(ctx, query, transferTime)
	if err != nil {
		return 0, err
	}

	return int(result.RowsAffected()), nil
}

// executeBatch executes a batch of queries
func (b *Builder) executeBatch(ctx context.Context, batch *pgx.Batch) error {
	results := b.db.SendBatch(ctx, batch)
	defer results.Close()

	for i := 0; i < batch.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("batch execution failed at query %d: %w", i, err)
		}
	}

	return nil
}

// analyzeGraph runs ANALYZE on graph tables for query optimization
func (b *Builder) analyzeGraph(ctx context.Context) error {
	tables := []string{"stop", "route", "node", "edge"}

	for _, table := range tables {
		_, err := b.db.Exec(ctx, fmt.Sprintf("ANALYZE %s", table))
		if err != nil {
			return err
		}
		log.Printf("Analyzed table: %s", table)
	}

	return nil
}

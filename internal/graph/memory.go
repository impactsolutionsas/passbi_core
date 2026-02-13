package graph

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/models"
)

// InMemoryGraph holds the entire routing graph in memory for fast A* lookups
type InMemoryGraph struct {
	mu        sync.RWMutex
	Nodes     map[int64]models.Node     // nodeID -> Node
	Edges     map[int64][]models.Edge   // fromNodeID -> []Edge
	StopNodes map[string][]int64        // stopID -> []nodeID
	loaded    bool
}

var (
	globalGraph     *InMemoryGraph
	globalGraphOnce sync.Once
)

// GetGraph returns the singleton in-memory graph
func GetGraph() *InMemoryGraph {
	globalGraphOnce.Do(func() {
		globalGraph = &InMemoryGraph{
			Nodes:     make(map[int64]models.Node),
			Edges:     make(map[int64][]models.Edge),
			StopNodes: make(map[string][]int64),
		}
	})
	return globalGraph
}

// LoadFromDB loads the entire graph from PostgreSQL into memory
func (g *InMemoryGraph) LoadFromDB(ctx context.Context, db *pgxpool.Pool) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	startTime := time.Now()
	log.Println("Loading graph into memory...")

	// 1. Load all nodes
	nodes := make(map[int64]models.Node)
	stopNodes := make(map[string][]int64)

	nodeRows, err := db.Query(ctx, `
		SELECT n.id, n.stop_id, s.name, n.route_id,
		       COALESCE(rt.short_name, rt.long_name, rt.id) as route_name,
		       n.mode, s.lat, s.lon
		FROM node n
		JOIN stop s ON s.id = n.stop_id
		LEFT JOIN route rt ON rt.id = n.route_id
	`)
	if err != nil {
		return fmt.Errorf("failed to load nodes: %w", err)
	}
	defer nodeRows.Close()

	for nodeRows.Next() {
		var node models.Node
		if err := nodeRows.Scan(&node.ID, &node.StopID, &node.StopName, &node.RouteID,
			&node.RouteName, &node.Mode, &node.Lat, &node.Lon); err != nil {
			log.Printf("Warning: failed to scan node: %v", err)
			continue
		}
		nodes[node.ID] = node
		stopNodes[node.StopID] = append(stopNodes[node.StopID], node.ID)
	}

	log.Printf("  Loaded %d nodes", len(nodes))

	// 2. Load all edges grouped by from_node_id
	edges := make(map[int64][]models.Edge)

	edgeRows, err := db.Query(ctx, `
		SELECT id, from_node_id, to_node_id, type, cost_time, cost_walk, cost_transfer
		FROM edge
		ORDER BY from_node_id
	`)
	if err != nil {
		return fmt.Errorf("failed to load edges: %w", err)
	}
	defer edgeRows.Close()

	edgeCount := 0
	for edgeRows.Next() {
		var edge models.Edge
		if err := edgeRows.Scan(&edge.ID, &edge.FromNodeID, &edge.ToNodeID, &edge.Type,
			&edge.CostTime, &edge.CostWalk, &edge.CostTransfer); err != nil {
			log.Printf("Warning: failed to scan edge: %v", err)
			continue
		}
		edges[edge.FromNodeID] = append(edges[edge.FromNodeID], edge)
		edgeCount++
	}

	log.Printf("  Loaded %d edges", edgeCount)

	// Swap in the new data
	g.Nodes = nodes
	g.Edges = edges
	g.StopNodes = stopNodes
	g.loaded = true

	duration := time.Since(startTime)
	log.Printf("Graph loaded in %v (%d nodes, %d edges)", duration, len(nodes), edgeCount)

	return nil
}

// IsLoaded returns true if the graph has been loaded
func (g *InMemoryGraph) IsLoaded() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.loaded
}

// GetNode returns a node by ID (in-memory lookup)
func (g *InMemoryGraph) GetNode(nodeID int64) (models.Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	node, ok := g.Nodes[nodeID]
	return node, ok
}

// GetEdges returns outgoing edges for a node (in-memory lookup)
func (g *InMemoryGraph) GetEdges(nodeID int64) []models.Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.Edges[nodeID]
}

// FindNearestNodes finds the N nearest nodes to coordinates using in-memory search
// BRT/TER stops are searched within a wider radius (2km) to prioritize mass transit
func (g *InMemoryGraph) FindNearestNodes(lat, lon float64, limit int) []models.Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Find nearest stops, with mode awareness
	type stopInfo struct {
		stopID    string
		dist      float64
		hasMassTransit bool // true if stop has BRT or TER nodes
	}

	stopMap := make(map[string]*stopInfo)
	for _, node := range g.Nodes {
		si, seen := stopMap[node.StopID]
		if !seen {
			dist := haversineDistanceFast(lat, lon, node.Lat, node.Lon)
			si = &stopInfo{stopID: node.StopID, dist: dist}
			stopMap[node.StopID] = si
		}
		if node.Mode == models.ModeBRT || node.Mode == models.ModeTER {
			si.hasMassTransit = true
		}
	}

	// Separate mass transit vs regular stops with different radii
	var massTransitStops []stopInfo // BRT/TER - wider radius
	var regularStops []stopInfo     // BUS - standard radius

	for _, si := range stopMap {
		if si.hasMassTransit && si.dist <= 2000 {
			// BRT/TER stops: 2km radius
			massTransitStops = append(massTransitStops, *si)
		} else if si.dist <= 1000 {
			// Regular stops: 1km radius
			regularStops = append(regularStops, *si)
		}
	}

	// Sort each group by distance
	sortStops := func(stops []stopInfo) {
		for i := 0; i < len(stops); i++ {
			for j := i + 1; j < len(stops); j++ {
				if stops[j].dist < stops[i].dist {
					stops[i], stops[j] = stops[j], stops[i]
				}
			}
		}
	}
	sortStops(massTransitStops)
	sortStops(regularStops)

	// Take top mass transit stops (up to 2) + top regular stops (up to 3)
	maxMassTransit := 2
	if maxMassTransit > len(massTransitStops) {
		maxMassTransit = len(massTransitStops)
	}
	maxRegular := 3
	if maxRegular > len(regularStops) {
		maxRegular = len(regularStops)
	}

	// Collect selected stops (mass transit first for priority)
	selectedStops := make(map[string]bool)
	var orderedStopIDs []string

	for i := 0; i < maxMassTransit; i++ {
		sid := massTransitStops[i].stopID
		if !selectedStops[sid] {
			selectedStops[sid] = true
			orderedStopIDs = append(orderedStopIDs, sid)
		}
	}
	for i := 0; i < maxRegular; i++ {
		sid := regularStops[i].stopID
		if !selectedStops[sid] {
			selectedStops[sid] = true
			orderedStopIDs = append(orderedStopIDs, sid)
		}
	}

	// Collect all nodes from selected stops
	var result []models.Node
	for _, stopID := range orderedStopIDs {
		for _, nodeID := range g.StopNodes[stopID] {
			if node, ok := g.Nodes[nodeID]; ok {
				result = append(result, node)
			}
		}
	}

	// Limit total nodes
	if len(result) > limit {
		result = result[:limit]
	}

	return result
}

// haversineDistanceFast calculates approximate distance in meters (fast version)
func haversineDistanceFast(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}

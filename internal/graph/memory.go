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
func (g *InMemoryGraph) FindNearestNodes(lat, lon float64, limit int) []models.Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type nodeWithDist struct {
		node models.Node
		dist float64
	}

	// Find nearest stops first (deduplicate by stop)
	stopDists := make(map[string]float64)
	for _, node := range g.Nodes {
		if _, seen := stopDists[node.StopID]; seen {
			continue
		}
		dist := haversineDistanceFast(lat, lon, node.Lat, node.Lon)
		stopDists[node.StopID] = dist
	}

	// Get closest stops
	type stopWithDist struct {
		stopID string
		dist   float64
	}
	var closestStops []stopWithDist
	for stopID, dist := range stopDists {
		if dist <= 1000 { // Only consider stops within 1km
			closestStops = append(closestStops, stopWithDist{stopID, dist})
		}
	}

	// Sort by distance
	for i := 0; i < len(closestStops); i++ {
		for j := i + 1; j < len(closestStops); j++ {
			if closestStops[j].dist < closestStops[i].dist {
				closestStops[i], closestStops[j] = closestStops[j], closestStops[i]
			}
		}
	}

	// Limit number of stops to consider
	maxStops := 3
	if maxStops > len(closestStops) {
		maxStops = len(closestStops)
	}
	closestStops = closestStops[:maxStops]

	// Collect all nodes from closest stops
	var result []models.Node
	for _, s := range closestStops {
		for _, nodeID := range g.StopNodes[s.stopID] {
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

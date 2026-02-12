package routing

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/models"
)

const (
	maxExploredNodes = 50000
	routingTimeout   = 10 * time.Second
)

// Router handles pathfinding operations
type Router struct {
	db *pgxpool.Pool
}

// NewRouter creates a new router instance
func NewRouter(db *pgxpool.Pool) *Router {
	return &Router{db: db}
}

// FindPath finds a route from origin to destination using the specified strategy
func (r *Router) FindPath(ctx context.Context, fromLat, fromLon, toLat, toLon float64, strategy Strategy) (*models.Path, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, routingTimeout)
	defer cancel()

	// Find candidate start nodes (nearest stops to origin)
	startNodes, err := r.findNearestNodes(ctx, fromLat, fromLon, 5)
	if err != nil || len(startNodes) == 0 {
		return nil, fmt.Errorf("no start nodes found near origin: %w", err)
	}

	// Find candidate goal nodes (nearest stops to destination)
	goalNodes, err := r.findNearestNodes(ctx, toLat, toLon, 5)
	if err != nil || len(goalNodes) == 0 {
		return nil, fmt.Errorf("no goal nodes found near destination: %w", err)
	}

	// Build goal node set for quick lookup
	goalSet := make(map[int64]models.Node)
	for _, node := range goalNodes {
		goalSet[node.ID] = node
	}

	// Run A* search
	path, err := r.astar(ctx, startNodes, goalSet, toLat, toLon, strategy)
	if err != nil {
		return nil, err
	}

	// Build Path response
	result := &models.Path{
		Nodes:     path.nodes,
		Edges:     path.edges,
		TotalTime: path.gScore,
		Strategy:  strategy.Name(),
	}

	// Calculate metrics
	for _, edge := range path.edges {
		result.TotalWalk += edge.CostWalk
		result.Transfers += edge.CostTransfer
	}

	result.DurationMins = result.TotalTime / 60
	result.WalkDistanceM = result.TotalWalk

	// Build steps
	result.Steps = r.buildSteps(result.Nodes, result.Edges)

	return result, nil
}

// astar implements the A* pathfinding algorithm
func (r *Router) astar(ctx context.Context, startNodes []models.Node, goalSet map[int64]models.Node, goalLat, goalLon float64, strategy Strategy) (*searchPath, error) {
	// Initialize open set (priority queue)
	openSet := &PriorityQueue{}
	heap.Init(openSet)

	// Track best paths to each node
	bestPaths := make(map[int64]*searchPath)

	// Add all start nodes to open set
	for _, node := range startNodes {
		heuristic := haversineDistance(node.Lat, node.Lon, goalLat, goalLon) / 1.4 // walking speed
		path := &searchPath{
			nodeID:    node.ID,
			nodes:     []models.Node{node},
			edges:     []models.Edge{},
			gScore:    0,
			fScore:    int(heuristic),
			transfers: 0,
		}
		heap.Push(openSet, path)
		bestPaths[node.ID] = path
	}

	exploredCount := 0

	for openSet.Len() > 0 {
		// Check timeout
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("routing timeout exceeded")
		default:
		}

		// Check exploration limit
		if exploredCount > maxExploredNodes {
			return nil, fmt.Errorf("explored too many nodes (%d), no path found", exploredCount)
		}

		// Pop node with lowest fScore
		current := heap.Pop(openSet).(*searchPath)
		exploredCount++

		// Check if we reached goal
		if _, isGoal := goalSet[current.nodeID]; isGoal {
			return current, nil
		}

		// Check strategy stopping criteria
		state := &PathState{
			Nodes:         current.nodes,
			Edges:         current.edges,
			TotalTime:     current.gScore,
			Transfers:     current.transfers,
			ExploredNodes: exploredCount,
		}
		if strategy.ShouldStop(state) {
			continue
		}

		// Get neighbors (lazy load edges from database)
		neighbors, err := r.loadEdges(ctx, current.nodeID)
		if err != nil {
			continue
		}

		// Explore neighbors
		for _, edge := range neighbors {
			// Get neighbor node info
			neighborNode, err := r.getNode(ctx, edge.ToNodeID)
			if err != nil {
				continue
			}

			// Calculate tentative gScore
			edgeCost := strategy.EdgeCost(edge)
			tentativeG := current.gScore + edgeCost

			// Check if this is a better path
			if existing, ok := bestPaths[edge.ToNodeID]; ok && tentativeG >= existing.gScore {
				continue
			}

			// Calculate heuristic
			h := haversineDistance(neighborNode.Lat, neighborNode.Lon, goalLat, goalLon) / 1.4

			// Build new path
			newPath := &searchPath{
				nodeID:    edge.ToNodeID,
				nodes:     append(append([]models.Node{}, current.nodes...), neighborNode),
				edges:     append(append([]models.Edge{}, current.edges...), edge),
				gScore:    tentativeG,
				fScore:    tentativeG + int(h),
				transfers: current.transfers + edge.CostTransfer,
			}

			bestPaths[edge.ToNodeID] = newPath
			heap.Push(openSet, newPath)
		}
	}

	return nil, fmt.Errorf("no path found after exploring %d nodes", exploredCount)
}

// findNearestNodes finds the N nearest nodes to a coordinate using Haversine formula
func (r *Router) findNearestNodes(ctx context.Context, lat, lon float64, limit int) ([]models.Node, error) {
	query := `
		SELECT n.id, n.stop_id, s.name, n.route_id, COALESCE(rt.short_name, rt.long_name, rt.id), n.mode, s.lat, s.lon
		FROM node n
		JOIN stop s ON s.id = n.stop_id
		LEFT JOIN route rt ON rt.id = n.route_id
		ORDER BY (
			6371000 * acos(
				LEAST(1.0, GREATEST(-1.0,
					cos(radians($2)) * cos(radians(s.lat)) *
					cos(radians(s.lon) - radians($1)) +
					sin(radians($2)) * sin(radians(s.lat))
				))
			)
		)
		LIMIT $3
	`

	rows, err := r.db.Query(ctx, query, lon, lat, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []models.Node
	for rows.Next() {
		var node models.Node
		if err := rows.Scan(&node.ID, &node.StopID, &node.StopName, &node.RouteID, &node.RouteName, &node.Mode, &node.Lat, &node.Lon); err != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// loadEdges loads outgoing edges for a node
func (r *Router) loadEdges(ctx context.Context, nodeID int64) ([]models.Edge, error) {
	query := `
		SELECT id, from_node_id, to_node_id, type, cost_time, cost_walk, cost_transfer
		FROM edge
		WHERE from_node_id = $1
	`

	rows, err := r.db.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []models.Edge
	for rows.Next() {
		var edge models.Edge
		if err := rows.Scan(&edge.ID, &edge.FromNodeID, &edge.ToNodeID, &edge.Type,
			&edge.CostTime, &edge.CostWalk, &edge.CostTransfer); err != nil {
			continue
		}
		edges = append(edges, edge)
	}

	return edges, nil
}

// getNode retrieves node information
func (r *Router) getNode(ctx context.Context, nodeID int64) (models.Node, error) {
	query := `
		SELECT n.id, n.stop_id, s.name, n.route_id, COALESCE(rt.short_name, rt.long_name, rt.id), n.mode, s.lat, s.lon
		FROM node n
		JOIN stop s ON s.id = n.stop_id
		LEFT JOIN route rt ON rt.id = n.route_id
		WHERE n.id = $1
	`

	var node models.Node
	err := r.db.QueryRow(ctx, query, nodeID).Scan(
		&node.ID, &node.StopID, &node.StopName, &node.RouteID, &node.RouteName, &node.Mode, &node.Lat, &node.Lon,
	)

	return node, err
}

// buildSteps constructs user-friendly step-by-step directions
// Consolidates consecutive RIDE steps on the same route into a single step
func (r *Router) buildSteps(nodes []models.Node, edges []models.Edge) []models.Step {
	if len(nodes) == 0 || len(edges) == 0 {
		return []models.Step{}
	}

	steps := []models.Step{}
	var currentStep *models.Step

	for i, edge := range edges {
		fromNode := nodes[i]
		toNode := nodes[i+1]

		step := models.Step{
			Type:         edge.Type,
			FromStop:     fromNode.StopID,
			FromStopName: fromNode.StopName,
			ToStop:       toNode.StopID,
			ToStopName:   toNode.StopName,
			Route:        fromNode.RouteID,
			RouteName:    fromNode.RouteName,
			Mode:         fromNode.Mode,
			Duration:     edge.CostTime,
			Distance:     edge.CostWalk,
			NumStops:     1, // Each edge represents moving through 1 stop
		}

		// Try to consolidate consecutive RIDE steps on the same route
		if currentStep != nil &&
			currentStep.Type == models.EdgeRide &&
			step.Type == models.EdgeRide &&
			currentStep.Route == step.Route {
			// Same route, extend the current step
			currentStep.ToStop = step.ToStop
			currentStep.ToStopName = step.ToStopName
			currentStep.Duration += step.Duration
			currentStep.Distance += step.Distance
			currentStep.NumStops++ // Increment stop count
		} else {
			// Different type, route, or first step - save current and start new
			if currentStep != nil {
				steps = append(steps, *currentStep)
			}
			currentStep = &step
		}
	}

	// Add the last step
	if currentStep != nil {
		steps = append(steps, *currentStep)
	}

	return steps
}

// haversineDistance calculates distance between two coordinates in meters
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000 // meters

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

// searchPath represents a path during A* search
type searchPath struct {
	nodeID    int64
	nodes     []models.Node
	edges     []models.Edge
	gScore    int
	fScore    int
	transfers int
	index     int // for heap
}

// PriorityQueue implements heap.Interface for A* open set
type PriorityQueue []*searchPath

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].fScore < pq[j].fScore
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	path := x.(*searchPath)
	path.index = n
	*pq = append(*pq, path)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	path := old[n-1]
	old[n-1] = nil
	path.index = -1
	*pq = old[0 : n-1]
	return path
}

package routing

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/passbi/passbi_core/internal/graph"
	"github.com/passbi/passbi_core/internal/models"
)

// getMaxExploredNodes reads MAX_EXPLORED_NODES from env or returns default
func getMaxExploredNodes() int {
	if val := os.Getenv("MAX_EXPLORED_NODES"); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return 50000
}

// getRoutingTimeout reads ROUTE_TIMEOUT from env or returns default
func getRoutingTimeout() time.Duration {
	if val := os.Getenv("ROUTE_TIMEOUT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return 10 * time.Second
}

// Router handles pathfinding operations using in-memory graph
type Router struct {
	graph *graph.InMemoryGraph
}

// NewRouter creates a new router instance using the in-memory graph
func NewRouter() *Router {
	return &Router{graph: graph.GetGraph()}
}

// FindPath finds a route from origin to destination using the specified strategy
func (r *Router) FindPath(ctx context.Context, fromLat, fromLon, toLat, toLon float64, strategy Strategy) (*models.Path, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, getRoutingTimeout())
	defer cancel()

	if !r.graph.IsLoaded() {
		return nil, fmt.Errorf("graph not loaded into memory")
	}

	// Find candidate start nodes (nearest stops to origin) - in-memory
	// Higher limit to include BRT/TER stops from wider search radius
	startNodes := r.graph.FindNearestNodes(fromLat, fromLon, 20)
	if len(startNodes) == 0 {
		return nil, fmt.Errorf("no start nodes found near origin")
	}

	// Find candidate goal nodes (nearest stops to destination) - in-memory
	goalNodes := r.graph.FindNearestNodes(toLat, toLon, 20)
	if len(goalNodes) == 0 {
		return nil, fmt.Errorf("no goal nodes found near destination")
	}

	// Build goal node set for quick lookup
	goalSet := make(map[int64]models.Node)
	for _, node := range goalNodes {
		goalSet[node.ID] = node
	}

	// Run A* search - entirely in-memory
	path, err := r.astar(ctx, startNodes, goalSet, toLat, toLon, strategy)
	if err != nil {
		return nil, err
	}

	// Build steps and compute metrics
	steps := buildSteps(path.nodes, path.edges)

	// Count actual transfers (route changes between RIDE steps)
	transfers := 0
	totalWalk := 0
	lastRideRoute := ""
	for _, step := range steps {
		if step.Type == models.EdgeRide {
			if lastRideRoute != "" && step.Route != lastRideRoute {
				transfers++
			}
			lastRideRoute = step.Route
		}
		totalWalk += step.Distance
	}

	result := &models.Path{
		Nodes:         path.nodes,
		Edges:         path.edges,
		TotalTime:     path.gScore,
		Strategy:      strategy.Name(),
		TotalWalk:     totalWalk,
		Transfers:     transfers,
		DurationMins:  path.gScore / 60,
		WalkDistanceM: totalWalk,
		Steps:         steps,
	}

	return result, nil
}

// astar implements the A* pathfinding algorithm using in-memory graph
func (r *Router) astar(ctx context.Context, startNodes []models.Node, goalSet map[int64]models.Node, goalLat, goalLon float64, strategy Strategy) (*searchPath, error) {
	// Initialize open set (priority queue)
	openSet := &PriorityQueue{}
	heap.Init(openSet)

	// Track best gScore to each node (just the cost, not the full path)
	bestG := make(map[int64]int)

	// Add all start nodes to open set
	for _, node := range startNodes {
		heuristic := haversineDistance(node.Lat, node.Lon, goalLat, goalLon) / 5.5
		path := &searchPath{
			nodeID:    node.ID,
			nodes:     []models.Node{node},
			edges:     []models.Edge{},
			gScore:    0,
			fScore:    int(heuristic),
			transfers: 0,
		}
		heap.Push(openSet, path)
		bestG[node.ID] = 0
	}

	exploredCount := 0
	maxNodes := getMaxExploredNodes()

	for openSet.Len() > 0 {
		// Check timeout periodically (every 1000 nodes to reduce overhead)
		if exploredCount%1000 == 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("routing timeout exceeded after exploring %d nodes", exploredCount)
			default:
			}
		}

		// Check exploration limit
		if exploredCount > maxNodes {
			return nil, fmt.Errorf("explored too many nodes (%d), no path found", exploredCount)
		}

		// Pop node with lowest fScore
		current := heap.Pop(openSet).(*searchPath)
		exploredCount++

		// Skip if we already found a better path to this node
		if bestScore, ok := bestG[current.nodeID]; ok && current.gScore > bestScore {
			continue
		}

		// Check if we reached goal
		if _, isGoal := goalSet[current.nodeID]; isGoal {
			return current, nil
		}

		// Check strategy stopping criteria
		state := &PathState{
			TotalTime:     current.gScore,
			Transfers:     current.transfers,
			ExploredNodes: exploredCount,
		}
		if strategy.ShouldStop(state) {
			continue
		}

		// Get neighbors from in-memory graph (instant lookup)
		neighbors := r.graph.GetEdges(current.nodeID)

		// Explore neighbors
		for _, edge := range neighbors {
			// Skip walk edges longer than 200m
			if edge.Type == models.EdgeWalk && edge.CostWalk > 200 {
				continue
			}

			// Get neighbor node info from in-memory graph (instant lookup)
			neighborNode, ok := r.graph.GetNode(edge.ToNodeID)
			if !ok {
				continue
			}

			// Calculate tentative gScore
			edgeCost := strategy.EdgeCost(edge)

			// Mode bonus: BRT/TER rides are cheaper (faster, higher capacity)
			if edge.Type == models.EdgeRide {
				switch neighborNode.Mode {
				case models.ModeTER:
					edgeCost = edgeCost * 50 / 100 // TER: 50% cost (train is fastest)
				case models.ModeBRT:
					edgeCost = edgeCost * 65 / 100 // BRT: 65% cost (dedicated lanes)
				}
			}

			tentativeG := current.gScore + edgeCost

			// Check if this is a better path
			if existingG, ok := bestG[edge.ToNodeID]; ok && tentativeG >= existingG {
				continue
			}

			// Calculate heuristic
			h := haversineDistance(neighborNode.Lat, neighborNode.Lon, goalLat, goalLon) / 5.5

			// Build new path
			newNodes := make([]models.Node, len(current.nodes)+1)
			copy(newNodes, current.nodes)
			newNodes[len(current.nodes)] = neighborNode

			newEdges := make([]models.Edge, len(current.edges)+1)
			copy(newEdges, current.edges)
			newEdges[len(current.edges)] = edge

			newPath := &searchPath{
				nodeID:    edge.ToNodeID,
				nodes:     newNodes,
				edges:     newEdges,
				gScore:    tentativeG,
				fScore:    tentativeG + int(h),
				transfers: current.transfers + edge.CostTransfer,
			}

			bestG[edge.ToNodeID] = tentativeG
			heap.Push(openSet, newPath)
		}
	}

	return nil, fmt.Errorf("no path found after exploring %d nodes", exploredCount)
}

// buildSteps constructs user-friendly step-by-step directions
// - Consolidates consecutive RIDE edges on the same route into one step with stops list
// - WALK steps don't show route/mode info
// - Eliminates redundant back-and-forth walks
// - TRANSFER edges between routes at same stop are merged into context
func buildSteps(nodes []models.Node, edges []models.Edge) []models.Step {
	if len(nodes) == 0 || len(edges) == 0 {
		return []models.Step{}
	}

	// Phase 1: Build raw steps
	var rawSteps []models.Step
	var currentStep *models.Step

	for i, edge := range edges {
		fromNode := nodes[i]
		toNode := nodes[i+1]

		switch edge.Type {
		case models.EdgeRide:
			// Consolidate consecutive RIDE edges on the same route
			if currentStep != nil &&
				currentStep.Type == models.EdgeRide &&
				currentStep.Route == fromNode.RouteID {
				// Extend current ride step
				currentStep.ToStop = toNode.StopID
				currentStep.ToStopName = toNode.StopName
				currentStep.Duration += edge.CostTime
				currentStep.NumStops++
				currentStep.Stops = append(currentStep.Stops, models.StopInfo{
					ID:   toNode.StopID,
					Name: toNode.StopName,
				})
			} else {
				// Save previous step and start new RIDE
				if currentStep != nil {
					rawSteps = append(rawSteps, *currentStep)
				}
				currentStep = &models.Step{
					Type:         models.EdgeRide,
					FromStop:     fromNode.StopID,
					FromStopName: fromNode.StopName,
					ToStop:       toNode.StopID,
					ToStopName:   toNode.StopName,
					Route:        fromNode.RouteID,
					RouteName:    fromNode.RouteName,
					Mode:         fromNode.Mode,
					Duration:     edge.CostTime,
					NumStops:     1,
					Stops: []models.StopInfo{
						{ID: fromNode.StopID, Name: fromNode.StopName},
						{ID: toNode.StopID, Name: toNode.StopName},
					},
				}
			}

		case models.EdgeWalk:
			// Save previous step
			if currentStep != nil {
				rawSteps = append(rawSteps, *currentStep)
				currentStep = nil
			}
			// WALK steps: no route/mode info
			rawSteps = append(rawSteps, models.Step{
				Type:         models.EdgeWalk,
				FromStop:     fromNode.StopID,
				FromStopName: fromNode.StopName,
				ToStop:       toNode.StopID,
				ToStopName:   toNode.StopName,
				Duration:     edge.CostTime,
				Distance:     edge.CostWalk,
			})

		case models.EdgeTransfer:
			// Save previous step - transfers are implicit between ride steps
			if currentStep != nil {
				rawSteps = append(rawSteps, *currentStep)
				currentStep = nil
			}
			// Only add explicit transfer step if there's actual wait time
			if edge.CostTime > 0 {
				rawSteps = append(rawSteps, models.Step{
					Type:         models.EdgeTransfer,
					FromStop:     fromNode.StopID,
					FromStopName: fromNode.StopName,
					ToStop:       toNode.StopID,
					ToStopName:   toNode.StopName,
					Duration:     edge.CostTime,
				})
			}
		}
	}

	// Don't forget the last step
	if currentStep != nil {
		rawSteps = append(rawSteps, *currentStep)
	}

	// Phase 2: Clean up - remove redundant walks and micro-walks
	var cleanSteps []models.Step
	for i, step := range rawSteps {
		// Skip very short walks (< 15m) that are just stop-matching artifacts
		if step.Type == models.EdgeWalk && step.Distance < 15 {
			// Check if this walk goes to the same named stop (duplicate stop IDs)
			if step.FromStopName == step.ToStopName {
				continue
			}
			// Check for back-and-forth: walk A→B followed by walk B→A
			if i+1 < len(rawSteps) && rawSteps[i+1].Type == models.EdgeWalk {
				next := rawSteps[i+1]
				if step.FromStop == next.ToStop && step.ToStop == next.FromStop {
					continue // Skip both walks (the next one will also be skipped)
				}
			}
			// Check if previous was a reverse walk
			if len(cleanSteps) > 0 {
				prev := cleanSteps[len(cleanSteps)-1]
				if prev.Type == models.EdgeWalk && prev.FromStop == step.ToStop && prev.ToStop == step.FromStop {
					// Remove previous walk too
					cleanSteps = cleanSteps[:len(cleanSteps)-1]
					continue
				}
			}
		}
		cleanSteps = append(cleanSteps, step)
	}

	if cleanSteps == nil {
		cleanSteps = []models.Step{}
	}

	return cleanSteps
}

// haversineDistance calculates distance between two coordinates in meters
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
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

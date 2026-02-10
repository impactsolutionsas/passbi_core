package routing

import "github.com/passbi/passbi_core/internal/models"

// Strategy defines the interface for routing strategies
// Each strategy can define custom edge costs and stopping criteria
type Strategy interface {
	Name() string
	EdgeCost(edge models.Edge) int
	ShouldStop(path *PathState) bool
}

// PathState represents the current state of a path during search
type PathState struct {
	Nodes         []models.Node
	Edges         []models.Edge
	TotalTime     int
	TotalWalk     int
	Transfers     int
	ExploredNodes int
}

// DirectStrategy prioritizes routes with no transfers
// Penalizes walking and makes transfers effectively impossible
type DirectStrategy struct{}

func (s *DirectStrategy) Name() string {
	return "direct"
}

func (s *DirectStrategy) EdgeCost(e models.Edge) int {
	switch e.Type {
	case models.EdgeTransfer:
		return 999999 // Effectively infinite - avoid transfers
	case models.EdgeWalk:
		return e.CostTime * 10 // Heavy walk penalty
	case models.EdgeRide:
		return e.CostTime
	default:
		return e.CostTime
	}
}

func (s *DirectStrategy) ShouldStop(p *PathState) bool {
	// Stop if we've made any transfer
	return p.Transfers > 0 || p.ExploredNodes > 5000
}

// SimpleStrategy balances time, walking, and transfers
// Good middle-ground option for most users
type SimpleStrategy struct{}

func (s *SimpleStrategy) Name() string {
	return "simple"
}

func (s *SimpleStrategy) EdgeCost(e models.Edge) int {
	// Balanced cost function:
	// time (1x) + walk distance (2x) + transfer penalty (3 minutes each)
	cost := e.CostTime

	if e.Type == models.EdgeWalk {
		cost += e.CostWalk * 2 // walking is 2x as costly as riding
	}

	if e.Type == models.EdgeTransfer {
		cost += 180 * e.CostTransfer // 3 min penalty per transfer
	}

	return cost
}

func (s *SimpleStrategy) ShouldStop(p *PathState) bool {
	// Stop after 2 transfers or too many explored nodes
	return p.Transfers > 2 || p.ExploredNodes > 10000
}

// FastStrategy optimizes purely for minimum travel time
// Willing to make more transfers and walk more to save time
type FastStrategy struct{}

func (s *FastStrategy) Name() string {
	return "fast"
}

func (s *FastStrategy) EdgeCost(e models.Edge) int {
	// Only consider actual time cost
	return e.CostTime
}

func (s *FastStrategy) ShouldStop(p *PathState) bool {
	// Allow up to 3 transfers, stop if exploring too many nodes
	return p.Transfers > 3 || p.ExploredNodes > 10000
}

// NoTransferStrategy absolutely forbids transfers - single line only
// Most restrictive strategy that guarantees zero transfers
type NoTransferStrategy struct{}

func (s *NoTransferStrategy) Name() string {
	return "no_transfer"
}

func (s *NoTransferStrategy) EdgeCost(e models.Edge) int {
	switch e.Type {
	case models.EdgeTransfer:
		return 999999999 // Absolutely infinite - no transfers allowed
	case models.EdgeWalk:
		return e.CostTime * 5 // Moderate walk penalty (less than direct)
	case models.EdgeRide:
		return e.CostTime
	default:
		return e.CostTime
	}
}

func (s *NoTransferStrategy) ShouldStop(p *PathState) bool {
	// Stop immediately if ANY transfer is encountered
	// Or if too many nodes explored (less aggressive than others)
	return p.Transfers > 0 || p.ExploredNodes > 3000
}

// GetStrategy returns a strategy by name
func GetStrategy(name string) Strategy {
	switch name {
	case "direct":
		return &DirectStrategy{}
	case "simple":
		return &SimpleStrategy{}
	case "fast":
		return &FastStrategy{}
	case "no_transfer":
		return &NoTransferStrategy{}
	default:
		return &SimpleStrategy{}
	}
}

// GetAllStrategies returns all available strategies
func GetAllStrategies() []Strategy {
	return []Strategy{
		&NoTransferStrategy{},
		&DirectStrategy{},
		&SimpleStrategy{},
		&FastStrategy{},
	}
}

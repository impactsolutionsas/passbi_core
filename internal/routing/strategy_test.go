package routing

import (
	"testing"

	"github.com/passbi/passbi_core/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestDirectStrategy(t *testing.T) {
	strategy := &DirectStrategy{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "direct", strategy.Name())
	})

	t.Run("Transfer edge has very high cost", func(t *testing.T) {
		edge := models.Edge{
			Type:         models.EdgeTransfer,
			CostTime:     180,
			CostTransfer: 1,
		}
		cost := strategy.EdgeCost(edge)
		assert.Equal(t, 999999, cost)
	})

	t.Run("Walk edge has high penalty", func(t *testing.T) {
		edge := models.Edge{
			Type:     models.EdgeWalk,
			CostTime: 120,
			CostWalk: 150,
		}
		cost := strategy.EdgeCost(edge)
		assert.Equal(t, 1200, cost) // 120 * 10
	})

	t.Run("Ride edge has normal cost", func(t *testing.T) {
		edge := models.Edge{
			Type:     models.EdgeRide,
			CostTime: 300,
		}
		cost := strategy.EdgeCost(edge)
		assert.Equal(t, 300, cost)
	})

	t.Run("Should stop after any transfer", func(t *testing.T) {
		path := &PathState{
			Transfers: 1,
		}
		assert.True(t, strategy.ShouldStop(path))
	})

	t.Run("Should not stop with no transfers", func(t *testing.T) {
		path := &PathState{
			Transfers: 0,
		}
		assert.False(t, strategy.ShouldStop(path))
	})
}

func TestSimpleStrategy(t *testing.T) {
	strategy := &SimpleStrategy{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "simple", strategy.Name())
	})

	t.Run("Balanced cost calculation", func(t *testing.T) {
		edge := models.Edge{
			Type:         models.EdgeWalk,
			CostTime:     120,
			CostWalk:     150,
			CostTransfer: 0,
		}
		cost := strategy.EdgeCost(edge)
		// 120 (time) + 150*2 (walk) = 420
		assert.Equal(t, 420, cost)
	})

	t.Run("Transfer penalty", func(t *testing.T) {
		edge := models.Edge{
			Type:         models.EdgeTransfer,
			CostTime:     180,
			CostTransfer: 1,
		}
		cost := strategy.EdgeCost(edge)
		// 180 (time) + 180*1 (transfer) = 360
		assert.Equal(t, 360, cost)
	})

	t.Run("Should stop after 2 transfers", func(t *testing.T) {
		path := &PathState{
			Transfers: 3,
		}
		assert.True(t, strategy.ShouldStop(path))
	})

	t.Run("Should not stop with 2 or fewer transfers", func(t *testing.T) {
		path := &PathState{
			Transfers: 2,
		}
		assert.False(t, strategy.ShouldStop(path))
	})
}

func TestFastStrategy(t *testing.T) {
	strategy := &FastStrategy{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "fast", strategy.Name())
	})

	t.Run("Only considers time", func(t *testing.T) {
		edge := models.Edge{
			Type:         models.EdgeWalk,
			CostTime:     120,
			CostWalk:     500,
			CostTransfer: 0,
		}
		cost := strategy.EdgeCost(edge)
		assert.Equal(t, 120, cost) // Only time matters
	})

	t.Run("Transfer has only time cost", func(t *testing.T) {
		edge := models.Edge{
			Type:         models.EdgeTransfer,
			CostTime:     180,
			CostTransfer: 1,
		}
		cost := strategy.EdgeCost(edge)
		assert.Equal(t, 180, cost)
	})

	t.Run("Should stop after 3 transfers", func(t *testing.T) {
		path := &PathState{
			Transfers: 4,
		}
		assert.True(t, strategy.ShouldStop(path))
	})

	t.Run("Should not stop with 3 or fewer transfers", func(t *testing.T) {
		path := &PathState{
			Transfers: 3,
		}
		assert.False(t, strategy.ShouldStop(path))
	})
}

func TestNoTransferStrategy(t *testing.T) {
	strategy := &NoTransferStrategy{}

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "no_transfer", strategy.Name())
	})

	t.Run("Transfer edge has absolute maximum cost", func(t *testing.T) {
		edge := models.Edge{
			Type:         models.EdgeTransfer,
			CostTime:     180,
			CostTransfer: 1,
		}
		cost := strategy.EdgeCost(edge)
		assert.Equal(t, 999999999, cost)
	})

	t.Run("Walk edge has moderate penalty", func(t *testing.T) {
		edge := models.Edge{
			Type:     models.EdgeWalk,
			CostTime: 120,
			CostWalk: 150,
		}
		cost := strategy.EdgeCost(edge)
		assert.Equal(t, 600, cost) // 120 * 5
	})

	t.Run("Ride edge has normal cost", func(t *testing.T) {
		edge := models.Edge{
			Type:     models.EdgeRide,
			CostTime: 300,
		}
		cost := strategy.EdgeCost(edge)
		assert.Equal(t, 300, cost)
	})

	t.Run("Should stop immediately with any transfer", func(t *testing.T) {
		path := &PathState{
			Transfers: 1,
		}
		assert.True(t, strategy.ShouldStop(path))
	})

	t.Run("Should not stop with zero transfers", func(t *testing.T) {
		path := &PathState{
			Transfers: 0,
		}
		assert.False(t, strategy.ShouldStop(path))
	})

	t.Run("Should stop after exploring 3000 nodes", func(t *testing.T) {
		path := &PathState{
			Transfers:     0,
			ExploredNodes: 3001,
		}
		assert.True(t, strategy.ShouldStop(path))
	})
}

func TestGetStrategy(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"no_transfer", "no_transfer"},
		{"direct", "direct"},
		{"simple", "simple"},
		{"fast", "fast"},
		{"unknown", "simple"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := GetStrategy(tt.name)
			assert.Equal(t, tt.expected, strategy.Name())
		})
	}
}

func TestGetAllStrategies(t *testing.T) {
	strategies := GetAllStrategies()
	assert.Equal(t, 4, len(strategies))

	names := make([]string, len(strategies))
	for i, s := range strategies {
		names[i] = s.Name()
	}

	assert.Contains(t, names, "no_transfer")
	assert.Contains(t, names, "direct")
	assert.Contains(t, names, "simple")
	assert.Contains(t, names, "fast")
}

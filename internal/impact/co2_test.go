package impact

import (
	"testing"
)

func TestBRTDirectJourney(t *testing.T) {
	legs := []Leg{
		{Type: "RIDE", Mode: "BRT", DistanceMeters: 8200},
	}
	result := Compute(legs)

	if result.CO2SavedGrams <= 0 {
		t.Errorf("expected co2_saved > 0, got %d", result.CO2SavedGrams)
	}
	// taxi: 8.2 * 180 = 1476g, transit: 8.2 * 18 = 147.6 → 147g, saved: 1328
	// 8.2 * 180 = 1475.999... → truncates to 1475 due to float64 representation
	if result.CO2TaxiGrams != 1475 {
		t.Errorf("expected co2_taxi=1475, got %d", result.CO2TaxiGrams)
	}
	// 8.2 * 18 = 147.599... → truncates to 147
	if result.CO2TransitGrams != 147 {
		t.Errorf("expected co2_transit=147, got %d", result.CO2TransitGrams)
	}
	if len(result.ModeBreakdown) != 1 || result.ModeBreakdown[0].Mode != "BRT" {
		t.Errorf("expected 1 BRT breakdown entry, got %v", result.ModeBreakdown)
	}
}

func TestWalkOnlyJourney(t *testing.T) {
	legs := []Leg{
		{Type: "WALK", Mode: "WALK", DistanceMeters: 500},
	}
	result := Compute(legs)

	// Walk-only: no RIDE legs, so transit CO₂ = 0
	if result.CO2TransitGrams != 0 {
		t.Errorf("expected co2_transit=0 for walk-only, got %d", result.CO2TransitGrams)
	}
	// Taxi baseline covers full distance (you'd have taken a taxi otherwise), so savings > 0
	if result.CO2SavedGrams <= 0 {
		t.Errorf("expected co2_saved > 0 (walking beats taxi), got %d", result.CO2SavedGrams)
	}
	if len(result.ModeBreakdown) != 0 {
		t.Errorf("expected empty mode_breakdown for walk-only, got %v", result.ModeBreakdown)
	}
}

func TestMixedDDDBRTJourney(t *testing.T) {
	legs := []Leg{
		{Type: "WALK", Mode: "WALK", DistanceMeters: 300},
		{Type: "RIDE", Mode: "DDD", DistanceMeters: 4000},
		{Type: "WALK", Mode: "WALK", DistanceMeters: 200},
		{Type: "RIDE", Mode: "BRT", DistanceMeters: 5000},
	}
	result := Compute(legs)

	if len(result.ModeBreakdown) != 2 {
		t.Errorf("expected 2 mode_breakdown entries, got %d", len(result.ModeBreakdown))
	}

	modes := map[string]bool{}
	for _, b := range result.ModeBreakdown {
		modes[b.Mode] = true
	}
	if !modes["DDD"] || !modes["BRT"] {
		t.Errorf("expected DDD and BRT in breakdown, got %v", result.ModeBreakdown)
	}

	if result.CO2SavedGrams <= 0 {
		t.Errorf("expected co2_saved > 0, got %d", result.CO2SavedGrams)
	}
}

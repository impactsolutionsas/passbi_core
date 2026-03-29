package impact

import (
	"fmt"
	"math"
)

// CO2 emission factors in gCO₂ per passenger per km
// Sources: ADEME, UITP Africa, local estimates for Dakar network
var EmissionFactors = map[string]float64{
	"BRT":  18.0,  // electric/CNG BRT SunuBRT
	"TER":   6.0,  // electric TER Dakar
	"DDD":  32.0,  // Dem Dikk diesel urban bus
	"AFTU": 38.0,  // AFTU minibus diesel
	"WALK":  0.0,  // walking
	"taxi": 180.0, // baseline: private taxi in Dakar
}

const DefaultTransitFactor = 35.0 // fallback for unknown modes

// LegImpact holds per-leg CO₂ data
type LegImpact struct {
	Mode       string  `json:"mode"`
	DistanceKm float64 `json:"distance_km"`
	CO2Grams   int64   `json:"co2_grams"`
}

// JourneyImpact is added to every journey in the API response
type JourneyImpact struct {
	CO2SavedGrams   int64       `json:"co2_saved_grams"`
	CO2TaxiGrams    int64       `json:"co2_taxi_grams"`
	CO2TransitGrams int64       `json:"co2_transit_grams"`
	DistanceKm      float64     `json:"distance_km"`
	EquivalentLabel string      `json:"equivalent_label"`
	ModeBreakdown   []LegImpact `json:"mode_breakdown"`
}

// Leg is the minimal interface needed from the journey leg
type Leg struct {
	Type           string // "RIDE" | "WALK"
	Mode           string // "BRT" | "TER" | "DDD" | "AFTU"
	DistanceMeters float64
}

// Compute calculates the climate impact of a journey vs taking a taxi
func Compute(legs []Leg) JourneyImpact {
	var totalDistanceM float64
	var transitCO2 float64
	var breakdown []LegImpact

	for _, leg := range legs {
		distKm := leg.DistanceMeters / 1000.0
		totalDistanceM += leg.DistanceMeters

		if leg.Type != "RIDE" {
			continue
		}

		factor, ok := EmissionFactors[leg.Mode]
		if !ok {
			factor = DefaultTransitFactor
		}

		legCO2 := distKm * factor
		transitCO2 += legCO2

		breakdown = append(breakdown, LegImpact{
			Mode:       leg.Mode,
			DistanceKm: round2(distKm),
			CO2Grams:   int64(legCO2),
		})
	}

	totalDistanceKm := totalDistanceM / 1000.0
	taxiCO2 := totalDistanceKm * EmissionFactors["taxi"]
	saved := taxiCO2 - transitCO2

	return JourneyImpact{
		CO2SavedGrams:   int64(saved),
		CO2TaxiGrams:    int64(taxiCO2),
		CO2TransitGrams: int64(transitCO2),
		DistanceKm:      round2(totalDistanceKm),
		EquivalentLabel: EquivalentLabel(int64(saved), totalDistanceKm),
		ModeBreakdown:   breakdown,
	}
}

// EquivalentLabel returns a human-readable comparison
func EquivalentLabel(co2SavedGrams int64, distanceKm float64) string {
	switch {
	case co2SavedGrams <= 0:
		return "Trajet bas carbone"
	case co2SavedGrams < 500:
		return fmt.Sprintf("Vous évitez %.0fg de CO₂ vs taxi", float64(co2SavedGrams))
	case co2SavedGrams < 2000:
		return fmt.Sprintf("Équivalent à %.1f km en voiture évités", distanceKm)
	default:
		trees := float64(co2SavedGrams) / 21000.0
		return fmt.Sprintf("Comme planter %.2f arbre ce jour", trees)
	}
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}

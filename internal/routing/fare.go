package routing

import "github.com/passbi/passbi_core/internal/models"

// EstimateFare estimates the fare for a journey based on agencies and distance
// Uses known Dakar transit pricing (XOF)
func EstimateFare(journey *models.Journey) *models.FareInfo {
	totalFare := 0

	for _, leg := range journey.Legs {
		if leg.Type != models.EdgeRide {
			continue
		}

		switch {
		case containsCI(leg.AgencyName, "BRT") || containsCI(string(leg.Mode), "BRT"):
			// BRT: 400 XOF intra-zone, 500 XOF inter-zone
			if leg.NumStops <= 8 {
				totalFare += 400
			} else {
				totalFare += 500
			}

		case containsCI(leg.AgencyName, "TER") || containsCI(string(leg.Mode), "TER"):
			// TER: 500-1500 XOF depending on distance
			distKm := float64(leg.Duration) * 0.04 // ~40m/s avg train speed → km
			switch {
			case distKm <= 10:
				totalFare += 500
			case distKm <= 25:
				totalFare += 1000
			default:
				totalFare += 1500
			}

		case containsCI(leg.AgencyName, "Dem Dikk") || containsCI(leg.AgencyName, "DDD"):
			// Dem Dikk: 150-300 XOF
			if leg.NumStops <= 10 {
				totalFare += 150
			} else {
				totalFare += 250
			}

		default:
			// AFTU / other bus: 150-250 XOF
			if leg.NumStops <= 8 {
				totalFare += 150
			} else {
				totalFare += 200
			}
		}
	}

	if totalFare == 0 {
		return nil
	}

	return &models.FareInfo{
		Amount:   totalFare,
		Currency: "XOF",
	}
}

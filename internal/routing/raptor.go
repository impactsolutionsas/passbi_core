package routing

import (
	"fmt"
	"math"

	"github.com/passbi/passbi_core/internal/graph"
	"github.com/passbi/passbi_core/internal/models"
)

const (
	maxTransfers      = 4
	maxWalkDistanceM  = 500
	minTransferTimeSec = 120
	infinity          = math.MaxInt32
	maxNearbyStops    = 10
)

// raptorLabel stores the best known arrival at a stop for a given round
type raptorLabel struct {
	arrivalSec    int
	boardStopIdx  int // stop where we boarded
	boardPos      int // position in route where we boarded
	alightStopIdx int // stop where we alighted (this stop)
	routeIdx      int // route taken
	globalTripIdx int // trip taken
	isTransfer    bool
	fromStopIdx   int // stop we transferred from (for walk legs)
	walkTimeSec   int
	walkDistM     int
}

// RaptorRouter implements the RAPTOR algorithm for time-dependent routing
type RaptorRouter struct {
	tt *graph.Timetable
}

// NewRaptorRouter creates a RAPTOR router using the loaded timetable
func NewRaptorRouter(tt *graph.Timetable) *RaptorRouter {
	return &RaptorRouter{tt: tt}
}

// FindJourneys computes Pareto-optimal journeys from origin to destination
// Returns journeys sorted by arrival time, each with different transfer counts
func (r *RaptorRouter) FindJourneys(fromLat, fromLon, toLat, toLon float64, departureTimeSec int) ([]models.Journey, error) {
	tt := r.tt
	if !tt.IsLoaded() {
		return nil, fmt.Errorf("timetable not loaded")
	}

	// Find nearest stops to origin and destination
	originStops := tt.FindNearestStops(fromLat, fromLon, maxWalkDistanceM, maxNearbyStops)
	if len(originStops) == 0 {
		return nil, fmt.Errorf("no stops found near origin")
	}
	destStops := tt.FindNearestStops(toLat, toLon, maxWalkDistanceM, maxNearbyStops)
	if len(destStops) == 0 {
		return nil, fmt.Errorf("no stops found near destination")
	}

	destSet := make(map[int]bool, len(destStops))
	for _, s := range destStops {
		destSet[s] = true
	}

	numStops := len(tt.StopIDs)

	// tau[k][s] = best arrival time at stop s with at most k transfers
	tau := make([][]int, maxTransfers+2)
	for k := range tau {
		tau[k] = make([]int, numStops)
		for s := range tau[k] {
			tau[k][s] = infinity
		}
	}

	// bestOverall[s] = best arrival time at stop s across all rounds
	bestOverall := make([]int, numStops)
	for s := range bestOverall {
		bestOverall[s] = infinity
	}

	// labels[k][s] = label for reconstruction
	labels := make([][]raptorLabel, maxTransfers+2)
	for k := range labels {
		labels[k] = make([]raptorLabel, numStops)
	}

	// marked stops: stops improved in current round
	marked := make([]bool, numStops)

	// Initialize: walk from origin to nearby stops (round 0)
	for _, sIdx := range originStops {
		walkDist := haversineDistance(fromLat, fromLon, tt.StopLats[sIdx], tt.StopLons[sIdx])
		walkTime := int(walkDist / 1.4) // 1.4 m/s walking speed
		arrTime := departureTimeSec + walkTime
		if arrTime < tau[0][sIdx] {
			tau[0][sIdx] = arrTime
			bestOverall[sIdx] = arrTime
			marked[sIdx] = true
			labels[0][sIdx] = raptorLabel{
				arrivalSec:    arrTime,
				isTransfer:    true, // initial walk
				fromStopIdx:   -1,   // from origin (not a stop)
				walkTimeSec:   walkTime,
				walkDistM:     int(walkDist),
				alightStopIdx: sIdx,
			}
		}
	}

	// RAPTOR rounds
	for k := 0; k <= maxTransfers; k++ {
		// Copy previous round's arrivals as baseline for this round
		copy(tau[k+1], tau[k])

		// Collect routes to scan: routes serving any marked stop
		routesToScan := make(map[int]int) // routeIdx → earliest marked stop position
		for sIdx := 0; sIdx < numStops; sIdx++ {
			if !marked[sIdx] {
				continue
			}
			for _, sr := range tt.StopRoutes[sIdx] {
				if existing, ok := routesToScan[sr.RouteIdx]; !ok || sr.PosInRoute < existing {
					routesToScan[sr.RouteIdx] = sr.PosInRoute
				}
			}
		}

		// Clear marks for next round
		newMarked := make([]bool, numStops)

		// Scan each route
		for routeIdx, startPos := range routesToScan {
			routeStops := tt.RouteStops[routeIdx]
			if len(routeStops) == 0 {
				continue
			}

			// Walk along the route from startPos
			var currentTrip *graph.TripEntry
			var currentTripIdx int = -1
			var boardStopIdx int = -1
			var boardPos int = -1

			for pos := startPos; pos < len(routeStops); pos++ {
				sIdx := routeStops[pos]

				// Can we catch an earlier trip at this stop?
				if tau[k][sIdx] < infinity {
					depNeeded := tau[k][sIdx]
					if k > 0 {
						depNeeded = max(depNeeded, tau[k][sIdx]+minTransferTimeSec)
					}
					trip, tripIdx := tt.EarliestTrip(routeIdx, pos, depNeeded)
					if trip != nil {
						if currentTrip == nil || tripIdx != currentTripIdx {
							// Board this trip (it's earlier or we had no trip)
							stTimes := tt.StopTimes[trip.GlobalTripIdx]
							if pos < len(stTimes) && stTimes[pos].DepartureSec >= depNeeded {
								if currentTrip == nil || stTimes[pos].DepartureSec < tt.StopTimes[currentTripIdx][pos].DepartureSec {
									currentTrip = trip
									currentTripIdx = trip.GlobalTripIdx
									boardStopIdx = sIdx
									boardPos = pos
								}
							}
						}
					}
				}

				// If we're on a trip, check if alighting here improves arrival
				if currentTrip != nil && currentTripIdx >= 0 {
					stTimes := tt.StopTimes[currentTripIdx]
					if pos < len(stTimes) {
						arrTime := stTimes[pos].ArrivalSec
						if arrTime < tau[k+1][sIdx] && arrTime < bestOverall[sIdx] {
							tau[k+1][sIdx] = arrTime
							bestOverall[sIdx] = arrTime
							newMarked[sIdx] = true
							labels[k+1][sIdx] = raptorLabel{
								arrivalSec:    arrTime,
								boardStopIdx:  boardStopIdx,
								boardPos:      boardPos,
								alightStopIdx: sIdx,
								routeIdx:      routeIdx,
								globalTripIdx: currentTripIdx,
							}
						}
					}
				}
			}
		}

		// Apply transfers (pedestrian walks from improved stops)
		for sIdx := 0; sIdx < numStops; sIdx++ {
			if !newMarked[sIdx] {
				continue
			}
			for _, transfer := range tt.Transfers[sIdx] {
				arrTime := tau[k+1][sIdx] + transfer.WalkTimeSec
				if arrTime < tau[k+1][transfer.ToStopIdx] && arrTime < bestOverall[transfer.ToStopIdx] {
					tau[k+1][transfer.ToStopIdx] = arrTime
					bestOverall[transfer.ToStopIdx] = arrTime
					newMarked[transfer.ToStopIdx] = true
					labels[k+1][transfer.ToStopIdx] = raptorLabel{
						arrivalSec:    arrTime,
						isTransfer:    true,
						fromStopIdx:   sIdx,
						alightStopIdx: transfer.ToStopIdx,
						walkTimeSec:   transfer.WalkTimeSec,
						walkDistM:     transfer.DistanceM,
					}
				}
			}
		}

		// Update marked set
		marked = newMarked

		// Early termination: if no stops were improved, stop
		anyMarked := false
		for _, m := range marked {
			if m {
				anyMarked = true
				break
			}
		}
		if !anyMarked {
			break
		}
	}

	// Extract Pareto-optimal journeys at destination stops
	var journeys []models.Journey

	// For each destination stop, find best arrival per round (Pareto: time vs transfers)
	bestArrival := infinity
	for k := 0; k <= maxTransfers+1; k++ {
		for _, destIdx := range destStops {
			if tau[k][destIdx] >= infinity {
				continue
			}

			// Add walk time from stop to actual destination
			walkDist := haversineDistance(tt.StopLats[destIdx], tt.StopLons[destIdx], toLat, toLon)
			walkTime := int(walkDist / 1.4)
			totalArrival := tau[k][destIdx] + walkTime

			// Pareto filter: only keep if arrival is better than any previous with fewer transfers
			if totalArrival >= bestArrival {
				continue
			}
			bestArrival = totalArrival

			// Reconstruct journey
			journey := r.reconstructJourney(tt, labels, tau, k, destIdx, departureTimeSec, toLat, toLon)
			if journey != nil {
				journeys = append(journeys, *journey)
			}
		}
	}

	if len(journeys) == 0 {
		return nil, fmt.Errorf("no journeys found")
	}

	return journeys, nil
}

func (r *RaptorRouter) reconstructJourney(tt *graph.Timetable, labels [][]raptorLabel, tau [][]int, round, destStopIdx, depTimeSec int, destLat, destLon float64) *models.Journey {
	var legs []models.Leg

	// Add final walk to destination
	walkDist := haversineDistance(tt.StopLats[destStopIdx], tt.StopLons[destStopIdx], destLat, destLon)
	if walkDist > 10 { // only add if meaningful
		walkTime := int(walkDist / 1.4)
		legs = append(legs, models.Leg{
			Type:          models.EdgeWalk,
			FromStop:      tt.StopIDs[destStopIdx],
			FromStopName:  tt.StopNames[destStopIdx],
			ToStop:        "destination",
			ToStopName:    "Destination",
			DepartureTime: tau[round][destStopIdx],
			ArrivalTime:   tau[round][destStopIdx] + walkTime,
			Duration:      walkTime,
			Distance:      int(walkDist),
		})
	}

	// Trace back through labels
	currentStop := destStopIdx
	currentRound := round

	for currentRound > 0 {
		label := labels[currentRound][currentStop]
		if label.arrivalSec == 0 && !label.isTransfer && label.routeIdx == 0 && label.globalTripIdx == 0 {
			break // no label
		}

		if label.isTransfer {
			// Walk leg
			if label.fromStopIdx >= 0 {
				legs = append(legs, models.Leg{
					Type:          models.EdgeWalk,
					FromStop:      tt.StopIDs[label.fromStopIdx],
					FromStopName:  tt.StopNames[label.fromStopIdx],
					ToStop:        tt.StopIDs[label.alightStopIdx],
					ToStopName:    tt.StopNames[label.alightStopIdx],
					DepartureTime: label.arrivalSec - label.walkTimeSec,
					ArrivalTime:   label.arrivalSec,
					Duration:      label.walkTimeSec,
					Distance:      label.walkDistM,
				})
			}
			// Walk doesn't consume a round — find the ride that preceded it
			currentStop = label.fromStopIdx
			if currentStop < 0 {
				break // reached origin
			}
			continue
		}

		// Ride leg: board at boardStopIdx, alight at currentStop
		stTimes := tt.StopTimes[label.globalTripIdx]
		rIdx := label.routeIdx
		routeStops := tt.RouteStops[rIdx]

		// Find positions of board and alight stops
		boardPos := label.boardPos
		var alightPos int
		for p, sIdx := range routeStops {
			if sIdx == currentStop {
				alightPos = p
				break
			}
		}

		// Collect intermediate stops
		var stops []models.StopInfo
		numStops := 0
		var depTime, arrTime int
		if boardPos < len(stTimes) {
			depTime = stTimes[boardPos].DepartureSec
		}
		if alightPos < len(stTimes) {
			arrTime = stTimes[alightPos].ArrivalSec
		}

		for p := boardPos; p <= alightPos && p < len(routeStops) && p < len(stTimes); p++ {
			sIdx := routeStops[p]
			stops = append(stops, models.StopInfo{
				ID:   tt.StopIDs[sIdx],
				Name: tt.StopNames[sIdx],
			})
			numStops++
		}

		// Calculate wait time (time between arriving at board stop and departure)
		waitTime := 0
		if currentRound > 1 || (currentRound == 1 && tau[currentRound-1][label.boardStopIdx] < infinity) {
			prevArr := tau[currentRound-1][label.boardStopIdx]
			if depTime > prevArr {
				waitTime = depTime - prevArr
			}
		}

		tripID := ""
		if label.globalTripIdx >= 0 && label.globalTripIdx < len(tt.StopTimes) {
			for _, trips := range tt.Trips[rIdx] {
				if trips.GlobalTripIdx == label.globalTripIdx {
					tripID = trips.TripID
					break
				}
			}
		}

		legs = append(legs, models.Leg{
			Type:          models.EdgeRide,
			FromStop:      tt.StopIDs[label.boardStopIdx],
			FromStopName:  tt.StopNames[label.boardStopIdx],
			ToStop:        tt.StopIDs[currentStop],
			ToStopName:    tt.StopNames[currentStop],
			Route:         tt.RouteIDs[rIdx],
			RouteName:     tt.RouteNames[rIdx],
			Mode:          tt.RouteModes[rIdx],
			AgencyName:    agencyName(tt.AgencyIDs[rIdx]),
			DepartureTime: depTime,
			ArrivalTime:   arrTime,
			Duration:      arrTime - depTime,
			NumStops:      numStops,
			Stops:         stops,
			TripID:        tripID,
			WaitTime:      waitTime,
		})

		// Move to the board stop of this ride, previous round
		currentStop = label.boardStopIdx
		currentRound--
	}

	// Add initial walk from origin
	if currentRound == 0 && labels[0][currentStop].arrivalSec > 0 {
		label := labels[0][currentStop]
		if label.walkDistM > 10 {
			legs = append(legs, models.Leg{
				Type:          models.EdgeWalk,
				FromStop:      "origin",
				FromStopName:  "Origine",
				ToStop:        tt.StopIDs[currentStop],
				ToStopName:    tt.StopNames[currentStop],
				DepartureTime: depTimeSec,
				ArrivalTime:   label.arrivalSec,
				Duration:      label.walkTimeSec,
				Distance:      label.walkDistM,
			})
		}
	}

	// Reverse legs (we built them back-to-front)
	for i, j := 0, len(legs)-1; i < j; i, j = i+1, j-1 {
		legs[i], legs[j] = legs[j], legs[i]
	}

	if len(legs) == 0 {
		return nil
	}

	// Compute journey metrics
	firstDep := legs[0].DepartureTime
	lastArr := legs[len(legs)-1].ArrivalTime
	totalWalk := 0
	transfers := 0
	lastRoute := ""
	for _, leg := range legs {
		totalWalk += leg.Distance
		if leg.Type == models.EdgeRide {
			if lastRoute != "" && leg.Route != lastRoute {
				transfers++
			}
			lastRoute = leg.Route
		}
	}

	return &models.Journey{
		Legs:          legs,
		DepartureTime: firstDep,
		ArrivalTime:   lastArr,
		Duration:      lastArr - firstDep,
		Transfers:     transfers,
		WalkDistance:   totalWalk,
	}
}

func agencyName(agencyID string) string {
	switch {
	case contains(agencyID, "AFTU") || contains(agencyID, "aftu"):
		return "AFTU"
	case contains(agencyID, "DDD") || contains(agencyID, "dem") || contains(agencyID, "DEM"):
		return "Dem Dikk"
	case contains(agencyID, "BRT") || contains(agencyID, "brt"):
		return "BRT Dakar"
	case contains(agencyID, "TER") || contains(agencyID, "ter"):
		return "TER"
	default:
		return agencyID
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsCI(s, sub))
}

func containsCI(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			sc := s[i+j]
			tc := sub[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if tc >= 'A' && tc <= 'Z' {
				tc += 32
			}
			if sc != tc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

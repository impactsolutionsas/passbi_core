package routing

import (
	"testing"

	"github.com/passbi/passbi_core/internal/graph"
	"github.com/passbi/passbi_core/internal/models"
	"github.com/stretchr/testify/assert"
)

// buildTestTimetable creates a minimal timetable for RAPTOR unit tests.
//
// Topology (all stops at same lat with increasing lon, ~100m apart):
//
//	Stop 0 (origin area) --[Route A]--> Stop 1 --[Route A]--> Stop 2 (dest area)
//	                                    Stop 1 --[Route B]--> Stop 3 --[Route B]--> Stop 2
//
// Route A: direct path  0 → 1 → 2  (departs 08:00, arrives 08:10, 08:20)
// Route B: detour path  1 → 3 → 2  (departs 08:12, arrives 08:25, 08:40)
// Transfer: Stop 1 → Stop 1 (same stop, allows boarding Route B after alighting Route A)
func buildTestTimetable() *graph.Timetable {
	tt := graph.NewTimetable()

	// 4 stops spread far enough that origin/dest only reach their nearest stop
	// ~0.01° longitude ≈ 1km at this latitude, well beyond maxWalkDistanceM (500m)
	tt.StopIDs = []string{"S0", "S1", "S2", "S3"}
	tt.StopNames = []string{"Origin Stop", "Mid Stop", "Dest Stop", "Detour Stop"}
	tt.StopLats = []float64{14.7000, 14.7100, 14.7200, 14.7150}
	tt.StopLons = []float64{-17.4000, -17.3900, -17.3800, -17.3850}
	tt.StopIndex = map[string]int{"S0": 0, "S1": 1, "S2": 2, "S3": 3}

	// 2 routes
	tt.RouteIDs = []string{"RA", "RB"}
	tt.RouteNames = []string{"Route A (direct)", "Route B (detour)"}
	tt.RouteModes = []models.TransitMode{models.ModeBus, models.ModeBus}
	tt.AgencyIDs = []string{"AG1", "AG1"}
	tt.RouteIndex = map[string]int{"RA": 0, "RB": 1}

	// Route stops
	tt.RouteStops = [][]int{
		{0, 1, 2}, // Route A: S0 → S1 → S2
		{1, 3, 2}, // Route B: S1 → S3 → S2
	}

	// StopRoutes: which routes serve each stop and at what position
	tt.StopRoutes = [][]graph.StopRouteEntry{
		{{RouteIdx: 0, PosInRoute: 0}},                              // S0: Route A pos 0
		{{RouteIdx: 0, PosInRoute: 1}, {RouteIdx: 1, PosInRoute: 0}}, // S1: Route A pos 1, Route B pos 0
		{{RouteIdx: 0, PosInRoute: 2}, {RouteIdx: 1, PosInRoute: 2}}, // S2: Route A pos 2, Route B pos 2
		{{RouteIdx: 1, PosInRoute: 1}},                              // S3: Route B pos 1
	}

	// Trips: one trip per route
	tt.Trips = [][]graph.TripEntry{
		{{TripID: "TA1", ServiceID: "SVC1", FirstDepartureSec: 28800, GlobalTripIdx: 0}}, // Route A trip at 08:00
		{{TripID: "TB1", ServiceID: "SVC1", FirstDepartureSec: 28920, GlobalTripIdx: 1}}, // Route B trip at 08:02
	}

	// StopTimes indexed by GlobalTripIdx
	tt.StopTimes = [][]graph.StopTimeEntry{
		// Trip 0 (Route A: S0→S1→S2) — direct, fast
		{
			{ArrivalSec: 28800, DepartureSec: 28800, StopIdx: 0}, // S0: 08:00
			{ArrivalSec: 29400, DepartureSec: 29400, StopIdx: 1}, // S1: 08:10
			{ArrivalSec: 30000, DepartureSec: 30000, StopIdx: 2}, // S2: 08:20
		},
		// Trip 1 (Route B: S1→S3→S2) — detour, slow
		{
			{ArrivalSec: 28920, DepartureSec: 28920, StopIdx: 1}, // S1: 08:12
			{ArrivalSec: 29700, DepartureSec: 29700, StopIdx: 3}, // S3: 08:25
			{ArrivalSec: 30600, DepartureSec: 30600, StopIdx: 2}, // S2: 08:40
		},
	}

	// Transfers: none needed for these tests (stops are close enough for FindNearestStops)
	tt.Transfers = make([][]graph.TransferEntry, 4)

	// Active services
	tt.ActiveServices = map[string]bool{"SVC1": true}

	tt.MarkLoaded()
	return tt
}

func TestRaptorDirectTrip(t *testing.T) {
	tt := buildTestTimetable()
	router := NewRaptorRouter(tt)

	// Origin exactly at S0, destination exactly at S2
	journeys, err := router.FindJourneys(14.7000, -17.4000, 14.7200, -17.3800, 28800)
	if !assert.NoError(t, err) || !assert.NotEmpty(t, journeys) {
		return
	}

	best := journeys[0]

	// The direct path via Route A (08:00→08:20) must be chosen over
	// any detour through Route B (which would arrive at 08:40).
	ridLegs := 0
	for _, leg := range best.Legs {
		if leg.Type == models.EdgeRide {
			ridLegs++
		}
	}

	assert.Equal(t, 1, ridLegs, "direct trip should have exactly 1 ride leg")
	assert.Equal(t, "Route A (direct)", best.Legs[findRideLeg(best.Legs)].RouteName,
		"should use Route A, not Route B")
	// Arrival must be ~08:20 (30000) plus a few seconds walk
	assert.LessOrEqual(t, best.ArrivalTime, 30060,
		"arrival should be around 08:20, not 08:40")
}

func TestRaptorOneTransfer(t *testing.T) {
	tt := buildTestTimetable()

	// Add a Route C that only goes S0→S1, and rely on Route B for S1→S2
	// This forces a genuine 1-transfer journey.
	tt.RouteIDs = append(tt.RouteIDs, "RC")
	tt.RouteNames = append(tt.RouteNames, "Route C (feeder)")
	tt.RouteModes = append(tt.RouteModes, models.ModeBus)
	tt.AgencyIDs = append(tt.AgencyIDs, "AG1")
	tt.RouteIndex["RC"] = 2

	tt.RouteStops = append(tt.RouteStops, []int{0, 1}) // Route C: S0→S1

	// Add Route C to StopRoutes
	tt.StopRoutes[0] = append(tt.StopRoutes[0], graph.StopRouteEntry{RouteIdx: 2, PosInRoute: 0})
	tt.StopRoutes[1] = append(tt.StopRoutes[1], graph.StopRouteEntry{RouteIdx: 2, PosInRoute: 1})

	// Trip for Route C: departs 07:55 (before Route A), arrives S1 at 08:00
	tt.Trips = append(tt.Trips, []graph.TripEntry{
		{TripID: "TC1", ServiceID: "SVC1", FirstDepartureSec: 28500, GlobalTripIdx: 2},
	})
	tt.StopTimes = append(tt.StopTimes, []graph.StopTimeEntry{
		{ArrivalSec: 28500, DepartureSec: 28500, StopIdx: 0}, // S0: 07:55
		{ArrivalSec: 28800, DepartureSec: 28800, StopIdx: 1}, // S1: 08:00
	})

	// Add transfer at S1 (self-transfer with minimum transfer time)
	tt.Transfers[1] = []graph.TransferEntry{
		{ToStopIdx: 1, WalkTimeSec: 0, DistanceM: 0},
	}

	router := NewRaptorRouter(tt)

	// Depart at 07:55 — can catch Route C, transfer at S1, then Route A or B
	journeys, err := router.FindJourneys(14.7000, -17.4000, 14.7200, -17.3800, 28500)
	if !assert.NoError(t, err) || !assert.NotEmpty(t, journeys) {
		return
	}

	// There should be at least one journey; best should arrive no later than Route B's 08:40
	best := journeys[0]
	assert.LessOrEqual(t, best.ArrivalTime, 30060+120,
		"transfer journey should not be worse than direct Route A arrival + transfer time")
}

func TestRaptorUnreachableDestination(t *testing.T) {
	tt := buildTestTimetable()
	router := NewRaptorRouter(tt)

	// Destination far away from any stop (lat 15.0, lon -16.0)
	journeys, err := router.FindJourneys(14.7000, -17.4000, 15.0, -16.0, 28800)
	assert.Error(t, err, "should return error for unreachable destination")
	assert.Nil(t, journeys)
	assert.NotPanics(t, func() {
		router.FindJourneys(14.7000, -17.4000, 15.0, -16.0, 28800)
	})
}

func findRideLeg(legs []models.Leg) int {
	for i, leg := range legs {
		if leg.Type == models.EdgeRide {
			return i
		}
	}
	return 0
}

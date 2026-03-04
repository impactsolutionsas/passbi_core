package graph

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/models"
)

// StopRouteEntry maps a stop to a route and its position within that route
type StopRouteEntry struct {
	RouteIdx    int
	PosInRoute  int // position of stop in route's stop sequence
}

// TripEntry represents a single trip on a route
type TripEntry struct {
	TripID            string
	ServiceID         string
	FirstDepartureSec int // departure from first stop (for binary search)
	GlobalTripIdx     int // index into Timetable.StopTimes
}

// StopTimeEntry represents arrival/departure at a single stop within a trip
type StopTimeEntry struct {
	ArrivalSec   int
	DepartureSec int
	StopIdx      int
}

// TransferEntry represents a pedestrian transfer between two stops
type TransferEntry struct {
	ToStopIdx   int
	WalkTimeSec int
	DistanceM   int
}

// Timetable holds the RAPTOR-optimized index for time-dependent routing
type Timetable struct {
	// Stop mapping
	StopIDs   []string       // stopIdx → stop_id
	StopIndex map[string]int // stop_id → stopIdx
	StopNames []string       // stopIdx → stop name
	StopLats  []float64      // stopIdx → latitude
	StopLons  []float64      // stopIdx → longitude

	// Route mapping
	RouteIDs   []string       // routeIdx → route_id
	RouteIndex map[string]int // route_id → routeIdx
	RouteNames []string       // routeIdx → display name
	RouteModes []models.TransitMode // routeIdx → mode
	AgencyIDs  []string       // routeIdx → agency_id

	// Route-Stop associations
	RouteStops [][]int            // routeIdx → []stopIdx (ordered stop sequence)
	StopRoutes [][]StopRouteEntry // stopIdx → routes serving this stop

	// Trips and StopTimes
	Trips     [][]TripEntry     // routeIdx → []TripEntry (sorted by FirstDepartureSec)
	StopTimes [][]StopTimeEntry // globalTripIdx → []StopTimeEntry (sorted by sequence)

	// Transfers (pedestrian)
	Transfers [][]TransferEntry // stopIdx → []TransferEntry

	// Service calendar
	ActiveServices map[string]bool // set of active service_ids for today

	loaded bool
}

// NewTimetable creates an empty timetable
func NewTimetable() *Timetable {
	return &Timetable{
		StopIndex:      make(map[string]int),
		RouteIndex:     make(map[string]int),
		ActiveServices: make(map[string]bool),
	}
}

// IsLoaded returns true if the timetable has data
func (tt *Timetable) IsLoaded() bool {
	return tt.loaded && len(tt.StopIDs) > 0 && len(tt.Trips) > 0
}

// LoadFromDB loads the timetable from PostgreSQL for RAPTOR routing
func (tt *Timetable) LoadFromDB(ctx context.Context, db *pgxpool.Pool) error {
	startTime := time.Now()
	log.Println("Loading RAPTOR timetable...")

	// Step 1: Determine active services for today
	if err := tt.loadActiveServices(ctx, db); err != nil {
		log.Printf("Warning: failed to load active services: %v (will use all trips)", err)
		// Continue — we'll use all trips as fallback
	}

	// Step 2: Load stops
	if err := tt.loadStops(ctx, db); err != nil {
		return fmt.Errorf("load stops: %w", err)
	}

	// Step 3: Load routes
	if err := tt.loadRoutes(ctx, db); err != nil {
		return fmt.Errorf("load routes: %w", err)
	}

	// Step 4: Load trips and stop_times, build RouteStops
	if err := tt.loadTripsAndStopTimes(ctx, db); err != nil {
		return fmt.Errorf("load trips: %w", err)
	}

	// Step 5: Build StopRoutes reverse index
	tt.buildStopRoutes()

	// Step 6: Build transfers from walk edges
	tt.buildTransfers(ctx, db)

	tt.loaded = true
	log.Printf("RAPTOR timetable loaded in %v (%d stops, %d routes, %d trips)",
		time.Since(startTime), len(tt.StopIDs), len(tt.RouteIDs), len(tt.StopTimes))

	return nil
}

func (tt *Timetable) loadActiveServices(ctx context.Context, db *pgxpool.Pool) error {
	today := time.Now()
	dayOfWeek := today.Weekday()
	dateStr := today.Format("2006-01-02")

	// Day column for calendar query
	dayCol := map[time.Weekday]string{
		time.Sunday: "sunday", time.Monday: "monday", time.Tuesday: "tuesday",
		time.Wednesday: "wednesday", time.Thursday: "thursday",
		time.Friday: "friday", time.Saturday: "saturday",
	}[dayOfWeek]

	// Get services active by calendar
	query := fmt.Sprintf(`
		SELECT service_id FROM calendar
		WHERE %s = true AND start_date <= $1 AND end_date >= $1
	`, dayCol)

	rows, err := db.Query(ctx, query, dateStr)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var sid string
		if err := rows.Scan(&sid); err != nil {
			continue
		}
		tt.ActiveServices[sid] = true
	}

	// Apply calendar_dates exceptions
	exRows, err := db.Query(ctx, `
		SELECT service_id, exception_type FROM calendar_date WHERE date = $1
	`, dateStr)
	if err != nil {
		return err
	}
	defer exRows.Close()

	for exRows.Next() {
		var sid string
		var exType int
		if err := exRows.Scan(&sid, &exType); err != nil {
			continue
		}
		if exType == 1 {
			tt.ActiveServices[sid] = true // added
		} else if exType == 2 {
			delete(tt.ActiveServices, sid) // removed
		}
	}

	log.Printf("  Active services today (%s): %d", dateStr, len(tt.ActiveServices))

	// If no calendar data, treat all services as active
	if len(tt.ActiveServices) == 0 {
		log.Println("  Warning: no active services found, will use all trips")
	}

	return nil
}

func (tt *Timetable) loadStops(ctx context.Context, db *pgxpool.Pool) error {
	rows, err := db.Query(ctx, `SELECT id, name, lat, lon FROM stop ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name string
		var lat, lon float64
		if err := rows.Scan(&id, &name, &lat, &lon); err != nil {
			continue
		}
		idx := len(tt.StopIDs)
		tt.StopIDs = append(tt.StopIDs, id)
		tt.StopNames = append(tt.StopNames, name)
		tt.StopLats = append(tt.StopLats, lat)
		tt.StopLons = append(tt.StopLons, lon)
		tt.StopIndex[id] = idx
	}

	log.Printf("  Loaded %d stops", len(tt.StopIDs))
	return nil
}

func (tt *Timetable) loadRoutes(ctx context.Context, db *pgxpool.Pool) error {
	rows, err := db.Query(ctx, `
		SELECT id, COALESCE(short_name, long_name, id), mode, agency_id
		FROM route ORDER BY id
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, mode, agency string
		if err := rows.Scan(&id, &name, &mode, &agency); err != nil {
			continue
		}
		idx := len(tt.RouteIDs)
		tt.RouteIDs = append(tt.RouteIDs, id)
		tt.RouteNames = append(tt.RouteNames, name)
		tt.RouteModes = append(tt.RouteModes, models.TransitMode(mode))
		tt.AgencyIDs = append(tt.AgencyIDs, agency)
		tt.RouteIndex[id] = idx
	}

	log.Printf("  Loaded %d routes", len(tt.RouteIDs))
	return nil
}

func (tt *Timetable) loadTripsAndStopTimes(ctx context.Context, db *pgxpool.Pool) error {
	// Load all stop_times joined with trips, ordered for route pattern detection
	query := `
		SELECT t.trip_id, t.route_id, t.service_id,
		       st.stop_id, st.stop_sequence,
		       COALESCE(st.departure_seconds, 0),
		       COALESCE(st.arrival_seconds, 0)
		FROM stop_time st
		JOIN trip t ON t.trip_id = st.trip_id AND t.agency_id = st.agency_id
		ORDER BY t.route_id, t.trip_id, st.stop_sequence
	`

	rows, err := db.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Temporary structures to group by route
	type rawStopTime struct {
		stopID    string
		seq       int
		arrSec    int
		depSec    int
	}
	type rawTrip struct {
		tripID    string
		routeID   string
		serviceID string
		stopTimes []rawStopTime
	}

	var currentTrip *rawTrip
	var allTrips []rawTrip

	for rows.Next() {
		var tripID, routeID, serviceID, stopID string
		var seq, depSec, arrSec int
		if err := rows.Scan(&tripID, &routeID, &serviceID, &stopID, &seq, &depSec, &arrSec); err != nil {
			continue
		}

		if currentTrip == nil || currentTrip.tripID != tripID || currentTrip.routeID != routeID {
			if currentTrip != nil && len(currentTrip.stopTimes) >= 2 {
				allTrips = append(allTrips, *currentTrip)
			}
			currentTrip = &rawTrip{
				tripID:    tripID,
				routeID:   routeID,
				serviceID: serviceID,
			}
		}
		currentTrip.stopTimes = append(currentTrip.stopTimes, rawStopTime{
			stopID: stopID, seq: seq, arrSec: arrSec, depSec: depSec,
		})
	}
	if currentTrip != nil && len(currentTrip.stopTimes) >= 2 {
		allTrips = append(allTrips, *currentTrip)
	}

	log.Printf("  Loaded %d trips from DB", len(allTrips))

	// Group trips by route and build stop patterns
	routeTrips := make(map[string][]rawTrip)
	for _, trip := range allTrips {
		routeTrips[trip.routeID] = append(routeTrips[trip.routeID], trip)
	}

	// Build RouteStops: use the longest trip per route as the canonical stop pattern
	tt.RouteStops = make([][]int, len(tt.RouteIDs))
	tt.Trips = make([][]TripEntry, len(tt.RouteIDs))
	globalTripIdx := 0

	for routeID, trips := range routeTrips {
		rIdx, ok := tt.RouteIndex[routeID]
		if !ok {
			continue
		}

		// Find the canonical stop pattern (longest trip)
		var longestTrip rawTrip
		for _, t := range trips {
			if len(t.stopTimes) > len(longestTrip.stopTimes) {
				longestTrip = t
			}
		}

		// Build RouteStops for this route
		stopPattern := make([]int, 0, len(longestTrip.stopTimes))
		stopPosMap := make(map[string]int) // stopID → position in pattern
		for i, st := range longestTrip.stopTimes {
			sIdx, ok := tt.StopIndex[st.stopID]
			if !ok {
				continue
			}
			stopPattern = append(stopPattern, sIdx)
			stopPosMap[st.stopID] = i
		}
		tt.RouteStops[rIdx] = stopPattern

		// Build trips for this route (filter by active services if available)
		for _, trip := range trips {
			if len(tt.ActiveServices) > 0 && !tt.ActiveServices[trip.serviceID] {
				continue
			}
			if len(trip.stopTimes) < 2 {
				continue
			}

			// Build StopTimeEntries
			stEntries := make([]StopTimeEntry, 0, len(trip.stopTimes))
			for _, st := range trip.stopTimes {
				sIdx, ok := tt.StopIndex[st.stopID]
				if !ok {
					continue
				}
				stEntries = append(stEntries, StopTimeEntry{
					ArrivalSec:   st.arrSec,
					DepartureSec: st.depSec,
					StopIdx:      sIdx,
				})
			}

			if len(stEntries) < 2 {
				continue
			}

			entry := TripEntry{
				TripID:            trip.tripID,
				ServiceID:         trip.serviceID,
				FirstDepartureSec: stEntries[0].DepartureSec,
				GlobalTripIdx:     globalTripIdx,
			}
			tt.Trips[rIdx] = append(tt.Trips[rIdx], entry)
			tt.StopTimes = append(tt.StopTimes, stEntries)
			globalTripIdx++
		}

		// Sort trips by first departure time (required for binary search)
		sort.Slice(tt.Trips[rIdx], func(i, j int) bool {
			return tt.Trips[rIdx][i].FirstDepartureSec < tt.Trips[rIdx][j].FirstDepartureSec
		})
	}

	log.Printf("  Indexed %d trips for RAPTOR", globalTripIdx)
	return nil
}

func (tt *Timetable) buildStopRoutes() {
	tt.StopRoutes = make([][]StopRouteEntry, len(tt.StopIDs))

	for rIdx, stops := range tt.RouteStops {
		for pos, sIdx := range stops {
			tt.StopRoutes[sIdx] = append(tt.StopRoutes[sIdx], StopRouteEntry{
				RouteIdx:   rIdx,
				PosInRoute: pos,
			})
		}
	}
}

func (tt *Timetable) buildTransfers(ctx context.Context, db *pgxpool.Pool) {
	tt.Transfers = make([][]TransferEntry, len(tt.StopIDs))

	// Build transfers from walk edges in the edge table
	rows, err := db.Query(ctx, `
		SELECT DISTINCT n1.stop_id, n2.stop_id, e.cost_time, e.cost_walk
		FROM edge e
		JOIN node n1 ON n1.id = e.from_node_id
		JOIN node n2 ON n2.id = e.to_node_id
		WHERE e.type = 'WALK' AND e.cost_walk <= 500
	`)
	if err != nil {
		log.Printf("Warning: failed to load transfers: %v", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var fromStop, toStop string
		var costTime, costWalk int
		if err := rows.Scan(&fromStop, &toStop, &costTime, &costWalk); err != nil {
			continue
		}
		fromIdx, ok1 := tt.StopIndex[fromStop]
		toIdx, ok2 := tt.StopIndex[toStop]
		if !ok1 || !ok2 || fromIdx == toIdx {
			continue
		}
		tt.Transfers[fromIdx] = append(tt.Transfers[fromIdx], TransferEntry{
			ToStopIdx:   toIdx,
			WalkTimeSec: costTime,
			DistanceM:   costWalk,
		})
		count++
	}

	log.Printf("  Loaded %d transfer links", count)
}

// FindNearestStops finds stops within maxDist meters of the given coordinates
// Returns []stopIdx sorted by distance
func (tt *Timetable) FindNearestStops(lat, lon float64, maxDist float64, limit int) []int {
	type candidate struct {
		stopIdx int
		dist    float64
	}
	var candidates []candidate

	for i := range tt.StopIDs {
		d := haversineDistanceFast(lat, lon, tt.StopLats[i], tt.StopLons[i])
		if d <= maxDist {
			candidates = append(candidates, candidate{i, d})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].dist < candidates[j].dist
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	result := make([]int, len(candidates))
	for i, c := range candidates {
		result[i] = c.stopIdx
	}
	return result
}

// EarliestTrip finds the earliest trip on a route departing from stopPos at or after depSec
// Returns the trip entry and the global trip index, or nil if none found
func (tt *Timetable) EarliestTrip(routeIdx, stopPos, depSec int) (*TripEntry, int) {
	trips := tt.Trips[routeIdx]
	if len(trips) == 0 {
		return nil, -1
	}

	// Binary search for first trip that could depart at or after depSec
	// We search by FirstDepartureSec as an approximation, then verify
	lo := sort.Search(len(trips), func(i int) bool {
		// Conservative: trip's first departure + some offset
		// We need to check the actual stop time
		return trips[i].FirstDepartureSec >= depSec-7200 // 2h buffer
	})

	for i := lo; i < len(trips); i++ {
		trip := &trips[i]
		stTimes := tt.StopTimes[trip.GlobalTripIdx]
		if stopPos >= len(stTimes) {
			continue
		}
		if stTimes[stopPos].DepartureSec >= depSec {
			return trip, trip.GlobalTripIdx
		}
	}

	return nil, -1
}

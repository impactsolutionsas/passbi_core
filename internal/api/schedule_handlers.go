package api

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/passbi/passbi_core/internal/cache"
	"github.com/passbi/passbi_core/internal/db"
)

// --- Response types ---

// DepartureInfo represents a single upcoming departure at a stop
type DepartureInfo struct {
	RouteID       string `json:"route_id"`
	RouteName     string `json:"route_name"`
	Mode          string `json:"mode"`
	AgencyID      string `json:"agency_id"`
	AgencyName    string `json:"agency_name"`
	Headsign      string `json:"headsign"`
	Direction     int    `json:"direction"`
	DepartureTime string `json:"departure_time"`
	DepartureSecs int    `json:"departure_seconds"`
	MinutesUntil  int    `json:"minutes_until"`
	TripID        string `json:"trip_id"`
	ServiceID     string `json:"service_id"`
	ServiceActive bool   `json:"service_active"`
}

// DeparturesResponse is the response for the departures endpoint
type DeparturesResponse struct {
	Stop        StopBasic       `json:"stop"`
	Departures  []DepartureInfo `json:"departures"`
	CurrentTime string          `json:"current_time"`
	Date        string          `json:"date"`
	Total       int             `json:"total"`
}

// StopBasic represents minimal stop info
type StopBasic struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// ScheduleService represents a service pattern for a route
type ScheduleService struct {
	ServiceID string   `json:"service_id"`
	Days      []string `json:"days"`
	StartDate string   `json:"start_date,omitempty"`
	EndDate   string   `json:"end_date,omitempty"`
}

// ScheduleStop represents a stop in the timetable
type ScheduleStop struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sequence int    `json:"sequence"`
}

// ScheduleTrip represents a trip row in the timetable
type ScheduleTrip struct {
	TripID    string   `json:"trip_id"`
	ServiceID string   `json:"service_id"`
	Headsign  string   `json:"headsign"`
	Direction int      `json:"direction"`
	Times     []string `json:"times"`
}

// ScheduleResponse is the response for the schedule endpoint
type ScheduleResponse struct {
	Route    RouteBasic        `json:"route"`
	Services []ScheduleService `json:"services"`
	Stops    []ScheduleStop    `json:"stops"`
	Trips    []ScheduleTrip    `json:"trips"`
	Total    int               `json:"total_trips"`
}

// RouteBasic represents minimal route info
type RouteBasic struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Mode     string `json:"mode"`
	AgencyID string `json:"agency_id"`
}

// TripDetail represents a trip with its stop times
type TripDetail struct {
	TripID    string         `json:"trip_id"`
	ServiceID string         `json:"service_id"`
	Headsign  string         `json:"headsign"`
	Direction int            `json:"direction"`
	Stops     []TripStopTime `json:"stops"`
}

// TripStopTime represents a stop time within a trip
type TripStopTime struct {
	StopID        string `json:"stop_id"`
	StopName      string `json:"stop_name"`
	Sequence      int    `json:"sequence"`
	ArrivalTime   string `json:"arrival_time"`
	DepartureTime string `json:"departure_time"`
}

// TripsResponse is the response for the trips endpoint
type TripsResponse struct {
	Route  RouteBasic   `json:"route"`
	Trips  []TripDetail `json:"trips"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// --- Handlers ---

// StopDepartures handles GET /v2/stops/:id/departures
func StopDepartures(c *fiber.Ctx) error {
	stopID := c.Params("id")
	if stopID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "stop ID is required"})
	}

	// Dakar timezone = UTC+0
	now := time.Now().UTC()

	// Parse time parameter (default: current time)
	timeStr := c.Query("time")
	var timeSecs int
	if timeStr != "" {
		parsed, err := parseTimeStr(timeStr)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("invalid time format (use HH:MM): %v", err)})
		}
		timeSecs = parsed
	} else {
		timeSecs = now.Hour()*3600 + now.Minute()*60 + now.Second()
		timeStr = now.Format("15:04:05")
	}

	// Parse date parameter (default: today)
	dateStr := c.Query("date")
	var date time.Time
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid date format (use YYYY-MM-DD)"})
		}
		date = parsed
	} else {
		date = now
		dateStr = now.Format("2006-01-02")
	}

	// Parse limit
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	// Check cache
	cacheKey := cache.DeparturesKey(stopID, dateStr, timeSecs)
	var cachedResp DeparturesResponse
	if err := cache.GetJSON(c.Context(), cacheKey, &cachedResp); err == nil {
		return c.JSON(cachedResp)
	}

	// Get DB
	pool, err := db.GetDB()
	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}

	ctx := c.Context()

	// Get stop info
	var stop StopBasic
	err = pool.QueryRow(ctx, `SELECT id, name, lat, lon FROM stop WHERE id = $1`, stopID).
		Scan(&stop.ID, &stop.Name, &stop.Lat, &stop.Lon)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "stop not found"})
	}

	// Query departures with active service detection
	// Map Go's Weekday() to the calendar column name
	dayColumns := [7]string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}
	dayCol := dayColumns[date.Weekday()]

	query := fmt.Sprintf(`
		WITH active_services AS (
			SELECT DISTINCT c.service_id, c.agency_id
			FROM calendar c
			WHERE $2::date BETWEEN c.start_date AND c.end_date
			  AND c.%s = true
			  AND NOT EXISTS (
				SELECT 1 FROM calendar_date cd
				WHERE cd.service_id = c.service_id
				  AND cd.agency_id = c.agency_id
				  AND cd.date = $2::date
				  AND cd.exception_type = 2
			  )

			UNION

			SELECT cd.service_id, cd.agency_id
			FROM calendar_date cd
			WHERE cd.date = $2::date
			  AND cd.exception_type = 1
		)
		SELECT
			st.departure_time,
			st.departure_seconds,
			t.trip_id,
			t.service_id,
			COALESCE(t.headsign, '') AS headsign,
			t.direction,
			r.id AS route_id,
			COALESCE(r.short_name, r.long_name, r.id) AS route_name,
			r.mode,
			r.agency_id,
			CASE WHEN a.service_id IS NOT NULL THEN true ELSE false END AS service_active
		FROM stop_time st
		JOIN trip t ON st.trip_id = t.trip_id AND st.agency_id = t.agency_id
		JOIN route r ON t.route_id = r.id
		LEFT JOIN active_services a ON t.service_id = a.service_id AND t.agency_id = a.agency_id
		WHERE st.stop_id = $1
		  AND st.departure_seconds >= $3
		  AND st.departure_seconds < $3 + 7200
		ORDER BY
			CASE WHEN a.service_id IS NOT NULL THEN 0 ELSE 1 END,
			st.departure_seconds
		LIMIT $4
	`, dayCol)

	rows, err := pool.Query(ctx, query, stopID, date, timeSecs, limit)
	if err != nil {
		log.Printf("Departures query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer rows.Close()

	var departures []DepartureInfo
	for rows.Next() {
		var d DepartureInfo
		if err := rows.Scan(
			&d.DepartureTime, &d.DepartureSecs,
			&d.TripID, &d.ServiceID, &d.Headsign, &d.Direction,
			&d.RouteID, &d.RouteName, &d.Mode, &d.AgencyID,
			&d.ServiceActive,
		); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		d.AgencyName = agencyDisplayName(d.AgencyID)
		d.MinutesUntil = (d.DepartureSecs - timeSecs) / 60
		if d.MinutesUntil < 0 {
			d.MinutesUntil = 0
		}
		departures = append(departures, d)
	}

	if departures == nil {
		departures = []DepartureInfo{}
	}

	resp := DeparturesResponse{
		Stop:        stop,
		Departures:  departures,
		CurrentTime: timeStr,
		Date:        dateStr,
		Total:       len(departures),
	}

	// Cache for 60 seconds
	if err := cache.SetJSON(c.Context(), cacheKey, resp, 60*time.Second); err != nil {
		log.Printf("Cache set error: %v", err)
	}

	return c.JSON(resp)
}

// RouteSchedule handles GET /v2/routes/:id/schedule
func RouteSchedule(c *fiber.Ctx) error {
	routeID := c.Params("id")
	if routeID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "route ID is required"})
	}

	direction := c.Query("direction", "all")
	serviceFilter := c.Query("service", "")

	// Check cache
	cacheKey := cache.ScheduleKey(routeID, direction, serviceFilter)
	var cachedResp ScheduleResponse
	if err := cache.GetJSON(c.Context(), cacheKey, &cachedResp); err == nil {
		return c.JSON(cachedResp)
	}

	pool, err := db.GetDB()
	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}

	ctx := c.Context()

	// Get route info
	var route RouteBasic
	err = pool.QueryRow(ctx, `
		SELECT id, COALESCE(short_name, long_name, id), mode, agency_id
		FROM route WHERE id = $1
	`, routeID).Scan(&route.ID, &route.Name, &route.Mode, &route.AgencyID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "route not found"})
	}

	// Get services for this route
	serviceRows, err := pool.Query(ctx, `
		SELECT DISTINCT t.service_id,
			c.monday, c.tuesday, c.wednesday, c.thursday, c.friday, c.saturday, c.sunday,
			c.start_date, c.end_date
		FROM trip t
		LEFT JOIN calendar c ON c.service_id = t.service_id AND c.agency_id = t.agency_id
		WHERE t.route_id = $1
		ORDER BY t.service_id
	`, routeID)
	if err != nil {
		log.Printf("Services query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer serviceRows.Close()

	var services []ScheduleService
	for serviceRows.Next() {
		var svc ScheduleService
		var mon, tue, wed, thu, fri, sat, sun *bool
		var startDate, endDate *time.Time

		if err := serviceRows.Scan(&svc.ServiceID,
			&mon, &tue, &wed, &thu, &fri, &sat, &sun,
			&startDate, &endDate); err != nil {
			log.Printf("Service scan error: %v", err)
			continue
		}

		var days []string
		if mon != nil && *mon {
			days = append(days, "monday")
		}
		if tue != nil && *tue {
			days = append(days, "tuesday")
		}
		if wed != nil && *wed {
			days = append(days, "wednesday")
		}
		if thu != nil && *thu {
			days = append(days, "thursday")
		}
		if fri != nil && *fri {
			days = append(days, "friday")
		}
		if sat != nil && *sat {
			days = append(days, "saturday")
		}
		if sun != nil && *sun {
			days = append(days, "sunday")
		}
		svc.Days = days
		if startDate != nil {
			svc.StartDate = startDate.Format("2006-01-02")
		}
		if endDate != nil {
			svc.EndDate = endDate.Format("2006-01-02")
		}

		services = append(services, svc)
	}
	if services == nil {
		services = []ScheduleService{}
	}

	// Get stop sequence from a representative trip
	stopQuery := `
		SELECT DISTINCT ON (st.stop_sequence) st.stop_id, s.name, st.stop_sequence
		FROM stop_time st
		JOIN trip t ON st.trip_id = t.trip_id AND st.agency_id = t.agency_id
		JOIN stop s ON s.id = st.stop_id
		WHERE t.route_id = $1
	`
	stopArgs := []interface{}{routeID}
	argIdx := 1

	if direction != "all" {
		argIdx++
		stopQuery += fmt.Sprintf(" AND t.direction = $%d", argIdx)
		dir, _ := strconv.Atoi(direction)
		stopArgs = append(stopArgs, dir)
	}
	if serviceFilter != "" {
		argIdx++
		stopQuery += fmt.Sprintf(" AND t.service_id = $%d", argIdx)
		stopArgs = append(stopArgs, serviceFilter)
	}

	stopQuery += " ORDER BY st.stop_sequence LIMIT 100"

	stopRows, err := pool.Query(ctx, stopQuery, stopArgs...)
	if err != nil {
		log.Printf("Stops query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer stopRows.Close()

	var stops []ScheduleStop
	for stopRows.Next() {
		var s ScheduleStop
		if err := stopRows.Scan(&s.ID, &s.Name, &s.Sequence); err != nil {
			log.Printf("Stop scan error: %v", err)
			continue
		}
		stops = append(stops, s)
	}
	if stops == nil {
		stops = []ScheduleStop{}
	}

	// Get trips with first departure time for ordering
	tripQuery := `
		SELECT t.trip_id, t.service_id, COALESCE(t.headsign, ''), t.direction,
			(SELECT st2.departure_time FROM stop_time st2
			 WHERE st2.trip_id = t.trip_id AND st2.agency_id = t.agency_id
			 ORDER BY st2.stop_sequence LIMIT 1) AS first_dep
		FROM trip t
		WHERE t.route_id = $1
	`
	tripArgs := []interface{}{routeID}
	tripArgIdx := 1

	if direction != "all" {
		tripArgIdx++
		tripQuery += fmt.Sprintf(" AND t.direction = $%d", tripArgIdx)
		dir, _ := strconv.Atoi(direction)
		tripArgs = append(tripArgs, dir)
	}
	if serviceFilter != "" {
		tripArgIdx++
		tripQuery += fmt.Sprintf(" AND t.service_id = $%d", tripArgIdx)
		tripArgs = append(tripArgs, serviceFilter)
	}

	tripQuery += " ORDER BY first_dep LIMIT 50"

	tripRows, err := pool.Query(ctx, tripQuery, tripArgs...)
	if err != nil {
		log.Printf("Trips query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer tripRows.Close()

	var trips []ScheduleTrip
	for tripRows.Next() {
		var t ScheduleTrip
		var firstDep *string
		if err := tripRows.Scan(&t.TripID, &t.ServiceID, &t.Headsign, &t.Direction, &firstDep); err != nil {
			log.Printf("Trip scan error: %v", err)
			continue
		}

		// Get departure times at each stop for this trip
		timeRows, err := pool.Query(ctx, `
			SELECT COALESCE(departure_time, '') FROM stop_time
			WHERE trip_id = $1 AND agency_id = (SELECT agency_id FROM trip WHERE trip_id = $1 LIMIT 1)
			ORDER BY stop_sequence
		`, t.TripID)
		if err != nil {
			log.Printf("Trip times query error: %v", err)
			continue
		}

		var times []string
		for timeRows.Next() {
			var tm string
			if err := timeRows.Scan(&tm); err == nil {
				times = append(times, tm)
			}
		}
		timeRows.Close()

		t.Times = times
		trips = append(trips, t)
	}
	if trips == nil {
		trips = []ScheduleTrip{}
	}

	resp := ScheduleResponse{
		Route:    route,
		Services: services,
		Stops:    stops,
		Trips:    trips,
		Total:    len(trips),
	}

	// Cache for 1 hour
	if err := cache.SetJSON(c.Context(), cacheKey, resp, time.Hour); err != nil {
		log.Printf("Cache set error: %v", err)
	}

	return c.JSON(resp)
}

// RouteTrips handles GET /v2/routes/:id/trips
func RouteTrips(c *fiber.Ctx) error {
	routeID := c.Params("id")
	if routeID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "route ID is required"})
	}

	serviceFilter := c.Query("service", "")
	directionFilter := c.Query("direction", "")
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	pool, err := db.GetDB()
	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}

	ctx := c.Context()

	// Get route info
	var route RouteBasic
	err = pool.QueryRow(ctx, `
		SELECT id, COALESCE(short_name, long_name, id), mode, agency_id
		FROM route WHERE id = $1
	`, routeID).Scan(&route.ID, &route.Name, &route.Mode, &route.AgencyID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "route not found"})
	}

	// Count total trips
	countQuery := `SELECT COUNT(*) FROM trip WHERE route_id = $1`
	countArgs := []interface{}{routeID}
	countArgIdx := 1

	if serviceFilter != "" {
		countArgIdx++
		countQuery += fmt.Sprintf(" AND service_id = $%d", countArgIdx)
		countArgs = append(countArgs, serviceFilter)
	}
	if directionFilter != "" {
		countArgIdx++
		countQuery += fmt.Sprintf(" AND direction = $%d", countArgIdx)
		dir, _ := strconv.Atoi(directionFilter)
		countArgs = append(countArgs, dir)
	}

	var total int
	pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)

	// Get trips
	tripQuery := `
		SELECT trip_id, agency_id, service_id, COALESCE(headsign, ''), direction
		FROM trip WHERE route_id = $1
	`
	tripArgs := []interface{}{routeID}
	tripArgIdx := 1

	if serviceFilter != "" {
		tripArgIdx++
		tripQuery += fmt.Sprintf(" AND service_id = $%d", tripArgIdx)
		tripArgs = append(tripArgs, serviceFilter)
	}
	if directionFilter != "" {
		tripArgIdx++
		tripQuery += fmt.Sprintf(" AND direction = $%d", tripArgIdx)
		dir, _ := strconv.Atoi(directionFilter)
		tripArgs = append(tripArgs, dir)
	}

	tripArgIdx++
	tripQuery += fmt.Sprintf(" ORDER BY trip_id LIMIT $%d", tripArgIdx)
	tripArgs = append(tripArgs, limit)

	tripArgIdx++
	tripQuery += fmt.Sprintf(" OFFSET $%d", tripArgIdx)
	tripArgs = append(tripArgs, offset)

	tripRows, err := pool.Query(ctx, tripQuery, tripArgs...)
	if err != nil {
		log.Printf("Trips query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer tripRows.Close()

	var trips []TripDetail
	for tripRows.Next() {
		var t TripDetail
		var agencyID string
		if err := tripRows.Scan(&t.TripID, &agencyID, &t.ServiceID, &t.Headsign, &t.Direction); err != nil {
			log.Printf("Trip scan error: %v", err)
			continue
		}

		// Get stop times for this trip
		stRows, err := pool.Query(ctx, `
			SELECT st.stop_id, s.name, st.stop_sequence,
				COALESCE(st.arrival_time, ''), COALESCE(st.departure_time, '')
			FROM stop_time st
			JOIN stop s ON s.id = st.stop_id
			WHERE st.trip_id = $1 AND st.agency_id = $2
			ORDER BY st.stop_sequence
		`, t.TripID, agencyID)
		if err != nil {
			log.Printf("Stop times query error: %v", err)
			continue
		}

		var stops []TripStopTime
		for stRows.Next() {
			var s TripStopTime
			if err := stRows.Scan(&s.StopID, &s.StopName, &s.Sequence, &s.ArrivalTime, &s.DepartureTime); err == nil {
				stops = append(stops, s)
			}
		}
		stRows.Close()

		if stops == nil {
			stops = []TripStopTime{}
		}
		t.Stops = stops
		trips = append(trips, t)
	}

	if trips == nil {
		trips = []TripDetail{}
	}

	return c.JSON(TripsResponse{
		Route:  route,
		Trips:  trips,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// parseTimeStr parses "HH:MM" or "HH:MM:SS" to seconds since midnight
func parseTimeStr(s string) (int, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("expected HH:MM or HH:MM:SS")
	}

	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}

	secs := 0
	if len(parts) == 3 {
		secs, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, err
		}
	}

	return h*3600 + m*60 + secs, nil
}

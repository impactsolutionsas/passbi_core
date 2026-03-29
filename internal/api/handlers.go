package api

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/passbi/passbi_core/internal/cache"
	"github.com/passbi/passbi_core/internal/db"
	"github.com/passbi/passbi_core/internal/graph"
	"github.com/passbi/passbi_core/internal/impact"
	"github.com/passbi/passbi_core/internal/models"
	"github.com/passbi/passbi_core/internal/routing"
)

// NetworkImpactDTO summarizes the best journey's climate impact across all options
type NetworkImpactDTO struct {
	TotalCO2SavedGrams int64  `json:"total_co2_saved_grams_if_best_chosen"`
	BestJourneyLabel   string `json:"best_journey_label"`
	TicketIsDigital    bool   `json:"ticket_is_digital"`
	CashAvoided        bool   `json:"cash_avoided"`
}

// RouteSearchResponse is the API response structure (unified format)
type RouteSearchResponse struct {
	Journeys      []JourneyResult  `json:"journeys"`
	DepartureTime string           `json:"departure_time"`
	Engine        string           `json:"engine"`
	NetworkImpact NetworkImpactDTO `json:"network_impact"`
}

// JourneyResult represents a RAPTOR-computed journey option
type JourneyResult struct {
	DepartureTime   string               `json:"departure_time"`
	ArrivalTime     string               `json:"arrival_time"`
	DurationSeconds int                  `json:"duration_seconds"`
	WalkDistanceM   int                  `json:"walk_distance_meters"`
	Transfers       int                  `json:"transfers"`
	Fare            *models.FareInfo     `json:"fare,omitempty"`
	Impact          impact.JourneyImpact `json:"impact"`
	Legs            []LegResult          `json:"legs"`
}

// LegResult represents one segment of a journey in the API response
type LegResult struct {
	Type          string          `json:"type"`
	FromStop      string          `json:"from_stop"`
	FromStopName  string          `json:"from_stop_name"`
	ToStop        string          `json:"to_stop"`
	ToStopName    string          `json:"to_stop_name"`
	DepartureTime string          `json:"departure_time"`
	ArrivalTime   string          `json:"arrival_time"`
	DurationSecs  int             `json:"duration_seconds"`
	WaitTimeSecs  int             `json:"wait_time_seconds,omitempty"`
	DistanceM     int             `json:"distance_meters,omitempty"`
	Route         string          `json:"route,omitempty"`
	RouteName     string          `json:"route_name,omitempty"`
	Mode          string          `json:"mode,omitempty"`
	AgencyName    string          `json:"agency_name,omitempty"`
	NumStops      int             `json:"num_stops,omitempty"`
	Stops         []models.StopInfo `json:"stops,omitempty"`
	TripID        string          `json:"trip_id,omitempty"`
}

// RouteSearch handles the /v2/route-search endpoint
// Uses RAPTOR (time-dependent) as primary engine, A* as fallback
func RouteSearch(c *fiber.Ctx) error {
	// Parse query parameters
	fromStr := c.Query("from")
	toStr := c.Query("to")

	if fromStr == "" || toStr == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "missing required parameters: from and to",
		})
	}

	// Parse coordinates
	fromLat, fromLon, err := parseCoordinates(fromStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": fmt.Sprintf("invalid 'from' coordinates: %v", err),
		})
	}

	toLat, toLon, err := parseCoordinates(toStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": fmt.Sprintf("invalid 'to' coordinates: %v", err),
		})
	}

	// Parse departure time (default: now, Dakar = UTC+0)
	now := time.Now().UTC()
	var baseTimeSecs int
	timeStr := c.Query("time")
	if timeStr != "" {
		parts := strings.Split(timeStr, ":")
		if len(parts) >= 2 {
			h, _ := strconv.Atoi(parts[0])
			m, _ := strconv.Atoi(parts[1])
			baseTimeSecs = h*3600 + m*60
		}
	} else {
		baseTimeSecs = now.Hour()*3600 + now.Minute()*60 + now.Second()
		timeStr = now.Format("15:04")
	}

	// Engine selection: ?engine=raptor|astar (default: raptor with astar fallback)
	engineParam := strings.ToLower(c.Query("engine"))
	// Accept common aliases
	if engineParam == "a*" || engineParam == "astra" || engineParam == "a-star" {
		engineParam = "astar"
	}
	if engineParam != "" && engineParam != "raptor" && engineParam != "astar" {
		return c.Status(400).JSON(fiber.Map{
			"error": fmt.Sprintf("unknown engine %q, use 'raptor' or 'astar'", engineParam),
		})
	}
	g := graph.GetGraph()

	if engineParam != "astar" {
		// Try RAPTOR (time-dependent, Pareto-optimal)
		if g.TT != nil && g.TT.IsLoaded() {
			journeys, err := computeRaptorRoute(fromLat, fromLon, toLat, toLon, baseTimeSecs, g.TT)
			if err == nil && len(journeys) > 0 {
				return c.JSON(RouteSearchResponse{
					Journeys:      journeys,
					DepartureTime: timeStr,
					Engine:        "raptor",
					NetworkImpact: computeNetworkImpact(journeys),
				})
			}
			if err != nil {
				log.Printf("RAPTOR failed: %v", err)
			}
			// If engine explicitly set to raptor, don't fallback
			if engineParam == "raptor" {
				return c.Status(404).JSON(fiber.Map{
					"error": "no journeys found (raptor)",
				})
			}
		} else if engineParam == "raptor" {
			return c.Status(503).JSON(fiber.Map{
				"error": "raptor engine not available (timetable not loaded)",
			})
		}
	}

	// A* engine — results converted to unified journey format
	ctx := c.Context()
	journeys, err := computeAStarJourneys(ctx, fromLat, fromLon, toLat, toLon, baseTimeSecs)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "no routes found between the specified locations",
		})
	}

	return c.JSON(RouteSearchResponse{
		Journeys:      journeys,
		DepartureTime: timeStr,
		Engine:        "astar",
		NetworkImpact: computeNetworkImpact(journeys),
	})
}

// computeRaptorRoute runs RAPTOR and formats results
func computeRaptorRoute(fromLat, fromLon, toLat, toLon float64, depTimeSec int, tt *graph.Timetable) ([]JourneyResult, error) {
	raptorRouter := routing.NewRaptorRouter(tt)
	journeys, err := raptorRouter.FindJourneys(fromLat, fromLon, toLat, toLon, depTimeSec)
	if err != nil {
		return nil, err
	}

	var results []JourneyResult
	for i := range journeys {
		j := &journeys[i]

		// Estimate fare
		j.Fare = routing.EstimateFare(j)

		// Convert legs to API format
		var legs []LegResult
		for _, leg := range j.Legs {
			legs = append(legs, LegResult{
				Type:          string(leg.Type),
				FromStop:      leg.FromStop,
				FromStopName:  leg.FromStopName,
				ToStop:        leg.ToStop,
				ToStopName:    leg.ToStopName,
				DepartureTime: formatSecondsToTime(leg.DepartureTime),
				ArrivalTime:   formatSecondsToTime(leg.ArrivalTime),
				DurationSecs:  leg.Duration,
				WaitTimeSecs:  leg.WaitTime,
				DistanceM:     leg.Distance,
				Route:         leg.Route,
				RouteName:     leg.RouteName,
				Mode:          string(leg.Mode),
				AgencyName:    leg.AgencyName,
				NumStops:      leg.NumStops,
				Stops:         leg.Stops,
				TripID:        leg.TripID,
			})
		}

		impactLegs := make([]impact.Leg, len(j.Legs))
		for i, leg := range j.Legs {
			impactLegs[i] = impact.Leg{
				Type:           string(leg.Type),
				Mode:           string(leg.Mode),
				DistanceMeters: float64(leg.Distance),
			}
		}

		results = append(results, JourneyResult{
			DepartureTime:   formatSecondsToTime(j.DepartureTime),
			ArrivalTime:     formatSecondsToTime(j.ArrivalTime),
			DurationSeconds: j.Duration,
			WalkDistanceM:   j.WalkDistance,
			Transfers:       j.Transfers,
			Fare:            j.Fare,
			Impact:          impact.Compute(impactLegs),
			Legs:            legs,
		})
	}

	return results, nil
}

// computeAStarJourneys runs A* strategies and converts to the unified journey format
func computeAStarJourneys(ctx context.Context, fromLat, fromLon, toLat, toLon float64, baseTimeSecs int) ([]JourneyResult, error) {
	strategies := routing.GetAllStrategies()

	type stratResult struct {
		strategy string
		path     *models.Path
		err      error
	}

	resultChan := make(chan stratResult, len(strategies))
	var wg sync.WaitGroup

	for _, strategy := range strategies {
		wg.Add(1)
		go func(strat routing.Strategy) {
			defer wg.Done()
			path, err := computeRoute(ctx, fromLat, fromLon, toLat, toLon, strat)
			resultChan <- stratResult{
				strategy: strat.Name(),
				path:     path,
				err:      err,
			}
		}(strategy)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var journeys []JourneyResult
	for result := range resultChan {
		if result.err != nil {
			log.Printf("Route computation failed for strategy %s: %v", result.strategy, result.err)
			continue
		}
		if result.path == nil {
			continue
		}

		enrichStepsWithTimes(result.path.Steps, baseTimeSecs)
		arrivalSecs := baseTimeSecs + result.path.TotalTime

		// Convert steps to legs
		var legs []LegResult
		for _, step := range result.path.Steps {
			legs = append(legs, LegResult{
				Type:          string(step.Type),
				FromStop:      step.FromStop,
				FromStopName:  step.FromStopName,
				ToStop:        step.ToStop,
				ToStopName:    step.ToStopName,
				DepartureTime: step.DepartureTime,
				ArrivalTime:   step.ArrivalTime,
				DurationSecs:  step.Duration,
				DistanceM:     step.Distance,
				Route:         step.Route,
				RouteName:     step.RouteName,
				Mode:          string(step.Mode),
				AgencyName:    step.AgencyName,
				NumStops:      step.NumStops,
				Stops:         step.Stops,
			})
		}

		impactLegs := make([]impact.Leg, len(legs))
		for i, leg := range legs {
			impactLegs[i] = impact.Leg{
				Type:           leg.Type,
				Mode:           leg.Mode,
				DistanceMeters: float64(leg.DistanceM),
			}
		}

		journeys = append(journeys, JourneyResult{
			DepartureTime:   formatSecondsToTime(baseTimeSecs),
			ArrivalTime:     formatSecondsToTime(arrivalSecs),
			DurationSeconds: result.path.TotalTime,
			WalkDistanceM:   result.path.TotalWalk,
			Transfers:       result.path.Transfers,
			Impact:          impact.Compute(impactLegs),
			Legs:            legs,
		})
	}

	if len(journeys) == 0 {
		return nil, fmt.Errorf("no routes found")
	}

	return journeys, nil
}

// computeRoute computes a route with caching
func computeRoute(ctx context.Context, fromLat, fromLon, toLat, toLon float64, strategy routing.Strategy) (*models.Path, error) {
	// Generate cache key
	cacheKey := cache.RouteKey(fromLat, fromLon, toLat, toLon, strategy.Name())
	lockKey := cache.LockKey(cacheKey)

	// Try to get from cache
	cachedPath, err := cache.GetRoute(ctx, cacheKey)
	if err == nil && cachedPath != nil {
		return cachedPath, nil
	}

	// Try to acquire lock
	acquired, err := cache.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		log.Printf("Failed to acquire lock: %v", err)
		// Continue without lock (degrade gracefully)
	} else if !acquired {
		// Another request is computing this route, wait for it
		cachedPath, err := cache.WaitForLock(ctx, cacheKey, 3*time.Second)
		if err == nil && cachedPath != nil {
			return cachedPath, nil
		}
		// If waiting failed, compute anyway
	}

	// Ensure lock is released
	defer func() {
		if acquired {
			cache.ReleaseLock(ctx, lockKey)
		}
	}()

	// Compute route using in-memory graph (no database queries during routing)
	router := routing.NewRouter()
	path, err := router.FindPath(ctx, fromLat, fromLon, toLat, toLon, strategy)
	if err != nil {
		return nil, err
	}

	// Cache result
	cacheTTL := 10 * time.Minute
	if err := cache.SetRoute(ctx, cacheKey, path, cacheTTL); err != nil {
		log.Printf("Failed to cache route: %v", err)
	}

	return path, nil
}

// computeNetworkImpact picks the best journey (highest CO₂ saved) for the summary
func computeNetworkImpact(journeys []JourneyResult) NetworkImpactDTO {
	if len(journeys) == 0 {
		return NetworkImpactDTO{}
	}
	best := journeys[0]
	for _, j := range journeys {
		if j.Impact.CO2SavedGrams > best.Impact.CO2SavedGrams {
			best = j
		}
	}
	return NetworkImpactDTO{
		TotalCO2SavedGrams: best.Impact.CO2SavedGrams,
		BestJourneyLabel:   best.Impact.EquivalentLabel,
		TicketIsDigital:    true,
		CashAvoided:        true,
	}
}

// Health handles the /health endpoint
func Health(c *fiber.Ctx) error {
	ctx := c.Context()

	// Check database
	dbErr := db.HealthCheck(ctx)
	dbStatus := "ok"
	if dbErr != nil {
		dbStatus = dbErr.Error()
	}

	// Check Redis
	redisErr := cache.HealthCheck(ctx)
	redisStatus := "ok"
	if redisErr != nil {
		redisStatus = redisErr.Error()
	}

	// Overall status
	status := "healthy"
	httpStatus := 200
	if dbErr != nil || redisErr != nil {
		status = "unhealthy"
		httpStatus = 503
	}

	return c.Status(httpStatus).JSON(fiber.Map{
		"status": status,
		"checks": fiber.Map{
			"database": dbStatus,
			"redis":    redisStatus,
		},
	})
}

// parseCoordinates parses "lat,lon" string into floats
func parseCoordinates(coordStr string) (lat, lon float64, err error) {
	parts := strings.Split(coordStr, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected format: lat,lon")
	}

	lat, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude: %w", err)
	}

	lon, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude: %w", err)
	}

	// Validate ranges
	if lat < -90 || lat > 90 {
		return 0, 0, fmt.Errorf("latitude must be between -90 and 90")
	}
	if lon < -180 || lon > 180 {
		return 0, 0, fmt.Errorf("longitude must be between -180 and 180")
	}

	return lat, lon, nil
}

// NearbyStopsResponse represents the response for nearby stops
type NearbyStopsResponse struct {
	Stops []NearbyStop `json:"stops"`
}

// NearbyRouteInfo represents a route serving a nearby stop
type NearbyRouteInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Mode       string `json:"mode"`
	AgencyID   string `json:"agency_id"`
	AgencyName string `json:"agency_name"`
}

// NearbyStop represents a nearby stop with its routes
type NearbyStop struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Lat           float64           `json:"lat"`
	Lon           float64           `json:"lon"`
	DistanceM     int               `json:"distance_meters"`
	Modes         []string          `json:"modes"`
	Routes        []NearbyRouteInfo `json:"routes"`
	RoutesCount   int               `json:"routes_count"`
}

// StopsNearby handles the /v2/stops/nearby endpoint
func StopsNearby(c *fiber.Ctx) error {
	// Parse query parameters
	latStr := c.Query("lat")
	lonStr := c.Query("lon")
	radiusStr := c.Query("radius", "500") // Default 500m

	if latStr == "" || lonStr == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "missing required parameters: lat and lon",
		})
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "invalid latitude",
		})
	}

	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "invalid longitude",
		})
	}

	radius, err := strconv.Atoi(radiusStr)
	if err != nil || radius < 0 || radius > 5000 {
		return c.Status(400).JSON(fiber.Map{
			"error": "invalid radius (must be between 0 and 5000 meters)",
		})
	}

	// Get database connection
	pool, err := db.GetDB()
	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "internal server error",
		})
	}

	ctx := c.Context()

	// Query nearby stops with their routes, modes, and agency info
	query := `
		WITH stop_distances AS (
			SELECT
				s.id,
				s.name,
				s.lat,
				s.lon,
				ROUND(
					6371000 * acos(
						LEAST(1.0, GREATEST(-1.0,
							cos(radians($2)) * cos(radians(s.lat)) *
							cos(radians(s.lon) - radians($1)) +
							sin(radians($2)) * sin(radians(s.lat))
						))
					)
				) AS distance
			FROM stop s
			WHERE (
				6371000 * acos(
					LEAST(1.0, GREATEST(-1.0,
						cos(radians($2)) * cos(radians(s.lat)) *
						cos(radians(s.lon) - radians($1)) +
						sin(radians($2)) * sin(radians(s.lat))
					))
				)
			) <= $3
		)
		SELECT
			sd.id,
			sd.name,
			sd.lat,
			sd.lon,
			sd.distance,
			r.id AS route_id,
			COALESCE(r.short_name, r.long_name, r.id) AS route_name,
			r.mode,
			r.agency_id
		FROM stop_distances sd
		LEFT JOIN node n ON n.stop_id = sd.id
		LEFT JOIN route r ON r.id = n.route_id
		ORDER BY sd.distance, r.mode, r.id
	`

	rows, err := pool.Query(ctx, query, lon, lat, radius)
	if err != nil {
		log.Printf("Query error: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "internal server error",
		})
	}
	defer rows.Close()

	// Group results by stop
	type stopRow struct {
		id, name                         string
		lat, lon                         float64
		distanceM                        int
		routeID, routeName, mode, agency *string
	}

	stopOrder := []string{}
	stopMap := make(map[string]*NearbyStop)

	for rows.Next() {
		var r stopRow
		if err := rows.Scan(&r.id, &r.name, &r.lat, &r.lon, &r.distanceM,
			&r.routeID, &r.routeName, &r.mode, &r.agency); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}

		stop, exists := stopMap[r.id]
		if !exists {
			stop = &NearbyStop{
				ID:        r.id,
				Name:      r.name,
				Lat:       r.lat,
				Lon:       r.lon,
				DistanceM: r.distanceM,
				Routes:    []NearbyRouteInfo{},
				Modes:     []string{},
			}
			stopMap[r.id] = stop
			stopOrder = append(stopOrder, r.id)
		}

		if r.routeID != nil {
			agencyName := agencyDisplayName(*r.agency)
			stop.Routes = append(stop.Routes, NearbyRouteInfo{
				ID:         *r.routeID,
				Name:       *r.routeName,
				Mode:       *r.mode,
				AgencyID:   *r.agency,
				AgencyName: agencyName,
			})
			// Track unique modes
			modeStr := *r.mode
			found := false
			for _, m := range stop.Modes {
				if m == modeStr {
					found = true
					break
				}
			}
			if !found {
				stop.Modes = append(stop.Modes, modeStr)
			}
		}
	}

	// Build ordered result (limit 20 stops)
	var stops []NearbyStop
	for i, id := range stopOrder {
		if i >= 20 {
			break
		}
		s := stopMap[id]
		s.RoutesCount = len(s.Routes)
		stops = append(stops, *s)
	}

	if stops == nil {
		stops = []NearbyStop{}
	}

	return c.JSON(NearbyStopsResponse{
		Stops: stops,
	})
}

// RoutesListResponse represents the response for routes list
type RoutesListResponse struct {
	Routes []RouteInfo `json:"routes"`
	Total  int         `json:"total"`
}

// RouteInfo represents route information
type RouteInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Mode       string `json:"mode"`
	AgencyID   string `json:"agency_id"`
	StopsCount int    `json:"stops_count"`
}

// RoutesList handles the /v2/routes/list endpoint
func RoutesList(c *fiber.Ctx) error {
	// Parse query parameters
	mode := c.Query("mode")        // Optional: filter by mode (BUS, BRT, TER)
	agency := c.Query("agency")    // Optional: filter by agency
	limitStr := c.Query("limit")   // Optional: limit number of results

	// Parse limit with default value
	limit := 100 // Default limit
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 1000 {
				limit = 1000 // Max limit to prevent abuse
			}
		}
	}

	// Get database connection
	pool, err := db.GetDB()
	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "internal server error",
		})
	}

	ctx := c.Context()

	// Build query with optional filters
	query := `
		SELECT
			r.id,
			COALESCE(r.short_name, r.long_name, r.id) AS name,
			r.mode,
			r.agency_id,
			COUNT(DISTINCT n.stop_id) AS stops_count
		FROM route r
		LEFT JOIN node n ON n.route_id = r.id
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 0

	if mode != "" {
		argCount++
		query += fmt.Sprintf(" AND UPPER(r.mode) = UPPER($%d)", argCount)
		args = append(args, mode)
	}

	if agency != "" {
		argCount++
		query += fmt.Sprintf(" AND r.agency_id = $%d", argCount)
		args = append(args, agency)
	}

	query += `
		GROUP BY r.id, r.short_name, r.long_name, r.mode, r.agency_id
		ORDER BY r.id
	`

	// Add limit
	argCount++
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, limit)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		log.Printf("Query error: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "internal server error",
		})
	}
	defer rows.Close()

	var routes []RouteInfo
	for rows.Next() {
		var route RouteInfo

		if err := rows.Scan(&route.ID, &route.Name, &route.Mode, &route.AgencyID, &route.StopsCount); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}

		routes = append(routes, route)
	}

	if routes == nil {
		routes = []RouteInfo{}
	}

	return c.JSON(RoutesListResponse{
		Routes: routes,
		Total:  len(routes),
	})
}

// StopSearchResult represents a stop in search results
type StopSearchResult struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// StopsSearch handles GET /v2/stops/search?q=petersen&limit=10
func StopsSearch(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" || len(query) < 2 {
		return c.Status(400).JSON(fiber.Map{
			"error": "query parameter 'q' is required (minimum 2 characters)",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	// Escape ILIKE special characters
	sanitized := strings.ReplaceAll(query, "%", "\\%")
	sanitized = strings.ReplaceAll(sanitized, "_", "\\_")
	pattern := "%" + sanitized + "%"

	pool, err := db.GetDB()
	if err != nil {
		log.Printf("Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}

	rows, err := pool.Query(c.Context(), `
		SELECT id, name, lat, lon
		FROM stop
		WHERE name ILIKE $1
		ORDER BY
			CASE WHEN lower(name) = lower($2) THEN 0
				 WHEN lower(name) LIKE lower($2) || '%' THEN 1
				 ELSE 2
			END,
			name
		LIMIT $3
	`, pattern, query, limit)
	if err != nil {
		log.Printf("Stop search query error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer rows.Close()

	var stops []StopSearchResult
	for rows.Next() {
		var s StopSearchResult
		if err := rows.Scan(&s.ID, &s.Name, &s.Lat, &s.Lon); err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		stops = append(stops, s)
	}

	if stops == nil {
		stops = []StopSearchResult{}
	}

	return c.JSON(fiber.Map{
		"stops": stops,
		"query": query,
		"total": len(stops),
	})
}

// enrichStepsWithTimes adds departure/arrival timestamps and agency names to steps
func enrichStepsWithTimes(steps []models.Step, baseTimeSecs int) {
	currentSecs := baseTimeSecs
	for i := range steps {
		steps[i].DepartureTime = formatSecondsToTime(currentSecs)
		arrivalSecs := currentSecs + steps[i].Duration
		steps[i].ArrivalTime = formatSecondsToTime(arrivalSecs)
		if steps[i].Type == models.EdgeRide && steps[i].Route != "" {
			steps[i].AgencyName = inferAgencyFromRoute(steps[i].Route)
		}
		currentSecs = arrivalSecs
	}
}

// formatSecondsToTime converts seconds since midnight to "HH:MM" string
func formatSecondsToTime(secs int) string {
	secs = secs % 86400
	h := secs / 3600
	m := (secs % 3600) / 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

// inferAgencyFromRoute derives agency name from route ID patterns
func inferAgencyFromRoute(routeID string) string {
	upper := strings.ToUpper(routeID)
	switch {
	case strings.Contains(upper, "AFTU"):
		return "AFTU"
	case strings.Contains(upper, "DDD") || strings.Contains(upper, "DEM"):
		return "Dem Dikk"
	case strings.HasPrefix(upper, "B") && len(routeID) <= 3:
		return "BRT Dakar"
	default:
		return agencyDisplayName(routeID)
	}
}

// agencyDisplayName maps agency_id patterns to human-readable names
func agencyDisplayName(agencyID string) string {
	upper := strings.ToUpper(agencyID)
	switch {
	case strings.Contains(upper, "AFTU"):
		return "AFTU"
	case strings.Contains(upper, "DDD") || strings.Contains(upper, "DEM"):
		return "Dem Dikk"
	case strings.Contains(upper, "BRT"):
		return "BRT Dakar"
	case strings.Contains(upper, "TER"):
		return "TER (Train Express Régional)"
	default:
		return agencyID
	}
}

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
	"github.com/passbi/passbi_core/internal/models"
	"github.com/passbi/passbi_core/internal/routing"
)

// RouteSearchResponse is the API response structure
type RouteSearchResponse struct {
	Routes map[string]*RouteResult `json:"routes"`
}

// RouteResult represents a single route option
type RouteResult struct {
	DurationSeconds int           `json:"duration_seconds"`
	WalkDistanceM   int           `json:"walk_distance_meters"`
	Transfers       int           `json:"transfers"`
	Steps           []models.Step `json:"steps"`
}

// RouteSearch handles the /v2/route-search endpoint
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

	// Compute all 4 routes in parallel using in-memory graph
	ctx := c.Context()
	strategies := routing.GetAllStrategies()

	type routeResult struct {
		strategy string
		path     *models.Path
		err      error
	}

	resultChan := make(chan routeResult, len(strategies))
	var wg sync.WaitGroup

	for _, strategy := range strategies {
		wg.Add(1)
		go func(strat routing.Strategy) {
			defer wg.Done()
			path, err := computeRoute(ctx, fromLat, fromLon, toLat, toLon, strat)
			resultChan <- routeResult{
				strategy: strat.Name(),
				path:     path,
				err:      err,
			}
		}(strategy)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	routes := make(map[string]*RouteResult)
	for result := range resultChan {
		if result.err != nil {
			log.Printf("Route computation failed for strategy %s: %v", result.strategy, result.err)
			// Still continue with other strategies
			continue
		}

		if result.path != nil {
			routes[result.strategy] = &RouteResult{
				DurationSeconds: result.path.TotalTime,
				WalkDistanceM:   result.path.TotalWalk,
				Transfers:       result.path.Transfers,
				Steps:           result.path.Steps,
			}
		}
	}

	// Check if we got at least one route
	if len(routes) == 0 {
		return c.Status(404).JSON(fiber.Map{
			"error": "no routes found between the specified locations",
		})
	}

	return c.JSON(RouteSearchResponse{
		Routes: routes,
	})
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

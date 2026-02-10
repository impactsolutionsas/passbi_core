package gtfs

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/models"
)

// InferMode determines the transit mode from a GTFS route
// Priority: route_type field, then keyword matching, default to BUS
func InferMode(route models.GTFSRoute) models.TransitMode {
	// First check for keyword matches (more specific)
	routeName := strings.ToUpper(route.ShortName + " " + route.LongName)

	if strings.Contains(routeName, "BRT") || strings.Contains(routeName, "RAPID") {
		return models.ModeBRT
	}
	if strings.Contains(routeName, "TER") || strings.Contains(routeName, "TRAIN") || strings.Contains(routeName, "RAIL") {
		return models.ModeTER
	}
	if strings.Contains(routeName, "FERRY") || strings.Contains(routeName, "BOAT") {
		return models.ModeFerry
	}
	if strings.Contains(routeName, "TRAM") {
		return models.ModeTram
	}

	// Then check GTFS route_type mapping
	// https://developers.google.com/transit/gtfs/reference#routestxt
	switch route.RouteType {
	case 0: // Tram, Streetcar, Light rail
		return models.ModeTram
	case 1: // Subway, Metro
		return models.ModeBRT // Map to BRT for now
	case 2: // Rail
		return models.ModeTER
	case 3: // Bus
		return models.ModeBus
	case 4: // Ferry
		return models.ModeFerry
	case 5: // Cable tram
		return models.ModeTram
	case 6: // Aerial lift, suspended cable car
		return models.ModeTram
	case 7: // Funicular
		return models.ModeTram
	}

	// Default to bus
	return models.ModeBus
}

// DeduplicateStops removes duplicate stops within a threshold distance
// Returns deduplicated stops and a mapping from old stop IDs to kept stop IDs
func DeduplicateStops(ctx context.Context, db *pgxpool.Pool, stops []models.GTFSStop, thresholdMeters float64) ([]models.GTFSStop, map[string]string, error) {
	if len(stops) == 0 {
		return stops, make(map[string]string), nil
	}

	// Simple distance-based deduplication
	// For each stop, check if there's a previous stop within threshold
	deduplicated := []models.GTFSStop{}
	skipIndices := make(map[int]bool)
	stopMapping := make(map[string]string) // old_id -> kept_id

	for i := 0; i < len(stops); i++ {
		if skipIndices[i] {
			continue
		}

		currentStop := stops[i]
		deduplicated = append(deduplicated, currentStop)
		stopMapping[currentStop.StopID] = currentStop.StopID // map to itself

		// Check remaining stops for duplicates
		for j := i + 1; j < len(stops); j++ {
			if skipIndices[j] {
				continue
			}

			distance := haversineDistance(
				currentStop.Lat, currentStop.Lon,
				stops[j].Lat, stops[j].Lon,
			)

			if distance < thresholdMeters {
				log.Printf("Deduplicating stop %s (duplicate of %s, distance: %.2fm)",
					stops[j].StopID, currentStop.StopID, distance)
				skipIndices[j] = true
				stopMapping[stops[j].StopID] = currentStop.StopID // map duplicate to original
			}
		}
	}

	log.Printf("Deduplicated %d stops to %d (removed %d duplicates)",
		len(stops), len(deduplicated), len(stops)-len(deduplicated))

	return deduplicated, stopMapping, nil
}

// haversineDistance calculates the distance between two points in meters
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000 // meters

	// Convert to radians
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	// Haversine formula
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

// ParseTimeToSeconds converts GTFS time format (HH:MM:SS) to seconds
// Handles times >= 24:00:00 (next day service)
func ParseTimeToSeconds(timeStr string) (int, error) {
	if timeStr == "" {
		return 0, fmt.Errorf("empty time string")
	}

	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid time format: %s", timeStr)
	}

	var hours, minutes, seconds int
	fmt.Sscanf(parts[0], "%d", &hours)
	fmt.Sscanf(parts[1], "%d", &minutes)
	fmt.Sscanf(parts[2], "%d", &seconds)

	return hours*3600 + minutes*60 + seconds, nil
}

// InterpolateStopTimes fills in missing arrival/departure times
// For trips with missing times, interpolate based on distance/speed
func InterpolateStopTimes(stopTimes []models.GTFSStopTime) []models.GTFSStopTime {
	if len(stopTimes) == 0 {
		return stopTimes
	}

	// Group by trip
	tripGroups := make(map[string][]models.GTFSStopTime)
	for _, st := range stopTimes {
		tripGroups[st.TripID] = append(tripGroups[st.TripID], st)
	}

	interpolated := []models.GTFSStopTime{}

	for tripID, times := range tripGroups {
		// Find first and last valid times
		firstValid := -1
		lastValid := -1

		for i, st := range times {
			if st.ArrivalTime != "" && st.DepartureTime != "" {
				if firstValid == -1 {
					firstValid = i
				}
				lastValid = i
			}
		}

		if firstValid == -1 || lastValid == -1 {
			log.Printf("Warning: trip %s has no valid times, skipping interpolation", tripID)
			interpolated = append(interpolated, times...)
			continue
		}

		// Simple linear interpolation between valid times
		for i := range times {
			if times[i].ArrivalTime == "" {
				// Interpolate
				if i < firstValid {
					times[i].ArrivalTime = times[firstValid].ArrivalTime
					times[i].DepartureTime = times[firstValid].DepartureTime
				} else if i > lastValid {
					times[i].ArrivalTime = times[lastValid].ArrivalTime
					times[i].DepartureTime = times[lastValid].DepartureTime
				} else {
					// Linear interpolation between surrounding valid times
					prevValid := firstValid
					for j := i - 1; j >= firstValid; j-- {
						if times[j].ArrivalTime != "" {
							prevValid = j
							break
						}
					}

					nextValid := lastValid
					for j := i + 1; j <= lastValid; j++ {
						if times[j].ArrivalTime != "" {
							nextValid = j
							break
						}
					}

					if prevValid != nextValid {
						times[i].ArrivalTime = times[prevValid].DepartureTime
						times[i].DepartureTime = times[prevValid].DepartureTime
					}
				}
			}

			interpolated = append(interpolated, times[i])
		}
	}

	return interpolated
}

// ValidateAndCleanStops removes stops with invalid coordinates
func ValidateAndCleanStops(stops []models.GTFSStop) []models.GTFSStop {
	cleaned := []models.GTFSStop{}

	for _, stop := range stops {
		// Check for valid coordinates
		if stop.Lat < -90 || stop.Lat > 90 {
			log.Printf("Warning: invalid latitude for stop %s: %f", stop.StopID, stop.Lat)
			continue
		}
		if stop.Lon < -180 || stop.Lon > 180 {
			log.Printf("Warning: invalid longitude for stop %s: %f", stop.StopID, stop.Lon)
			continue
		}
		if stop.Lat == 0 && stop.Lon == 0 {
			log.Printf("Warning: stop %s has null island coordinates, skipping", stop.StopID)
			continue
		}

		cleaned = append(cleaned, stop)
	}

	if len(cleaned) < len(stops) {
		log.Printf("Cleaned stops: removed %d invalid stops", len(stops)-len(cleaned))
	}

	return cleaned
}

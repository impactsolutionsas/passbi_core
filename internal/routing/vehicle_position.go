package routing

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/models"
)

// VehiclePositionEstimator estimates vehicle positions on routes
type VehiclePositionEstimator struct {
	db *pgxpool.Pool
}

// NewVehiclePositionEstimator creates a new estimator
func NewVehiclePositionEstimator(db *pgxpool.Pool) *VehiclePositionEstimator {
	return &VehiclePositionEstimator{db: db}
}

// EstimatePosition estimates the current position of a vehicle on a route
// based on elapsed time since the start of the journey
func (e *VehiclePositionEstimator) EstimatePosition(ctx context.Context, path *models.Path, elapsedSeconds int) (lat, lon float64, err error) {
	if len(path.Nodes) == 0 {
		return 0, 0, fmt.Errorf("path has no nodes")
	}

	if elapsedSeconds <= 0 {
		// At start position
		return path.Nodes[0].Lat, path.Nodes[0].Lon, nil
	}

	if elapsedSeconds >= path.TotalTime {
		// At end position
		lastNode := path.Nodes[len(path.Nodes)-1]
		return lastNode.Lat, lastNode.Lon, nil
	}

	// Find which segment the vehicle is currently on
	cumulativeTime := 0
	for i, edge := range path.Edges {
		segmentEndTime := cumulativeTime + edge.CostTime

		if elapsedSeconds >= cumulativeTime && elapsedSeconds < segmentEndTime {
			// Vehicle is on this segment
			progress := float64(elapsedSeconds-cumulativeTime) / float64(edge.CostTime)

			// Get start and end nodes
			startNode := path.Nodes[i]
			endNode := path.Nodes[i+1]

			// Interpolate position
			return e.interpolatePosition(ctx, startNode, endNode, progress)
		}

		cumulativeTime = segmentEndTime
	}

	// Fallback to end position
	lastNode := path.Nodes[len(path.Nodes)-1]
	return lastNode.Lat, lastNode.Lon, nil
}

// interpolatePosition interpolates between two nodes using PostGIS
func (e *VehiclePositionEstimator) interpolatePosition(ctx context.Context, start, end models.Node, progress float64) (lat, lon float64, err error) {
	// Clamp progress to [0, 1]
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	// Use PostGIS ST_LineInterpolatePoint for accurate geospatial interpolation
	query := `
		SELECT
			ST_Y(point) AS lat,
			ST_X(point) AS lon
		FROM (
			SELECT ST_LineInterpolatePoint(
				ST_MakeLine(
					ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
					ST_SetSRID(ST_MakePoint($3, $4), 4326)::geography
				)::geometry,
				$5
			) AS point
		) sub
	`

	err = e.db.QueryRow(ctx, query, start.Lon, start.Lat, end.Lon, end.Lat, progress).Scan(&lat, &lon)
	if err != nil {
		// Fallback to simple linear interpolation if PostGIS fails
		lat, lon = linearInterpolate(start.Lat, start.Lon, end.Lat, end.Lon, progress)
		return lat, lon, nil
	}

	return lat, lon, nil
}

// linearInterpolate performs simple linear interpolation between two points
func linearInterpolate(lat1, lon1, lat2, lon2, progress float64) (lat, lon float64) {
	lat = lat1 + (lat2-lat1)*progress
	lon = lon1 + (lon2-lon1)*progress
	return lat, lon
}

// EstimateArrivalTime estimates the arrival time at a specific stop in the path
func (e *VehiclePositionEstimator) EstimateArrivalTime(path *models.Path, stopIndex int) (int, error) {
	if stopIndex < 0 || stopIndex >= len(path.Nodes) {
		return 0, fmt.Errorf("invalid stop index: %d", stopIndex)
	}

	if stopIndex == 0 {
		return 0, nil // Already at start
	}

	// Sum edge times up to this stop
	totalTime := 0
	for i := 0; i < stopIndex; i++ {
		totalTime += path.Edges[i].CostTime
	}

	return totalTime, nil
}

// EstimateProgress calculates the progress along a route as a percentage
func EstimateProgress(elapsedSeconds, totalSeconds int) float64 {
	if totalSeconds <= 0 {
		return 0
	}

	progress := float64(elapsedSeconds) / float64(totalSeconds)

	// Clamp to [0, 1]
	return math.Max(0, math.Min(1, progress))
}

// DistanceAlongPath calculates the distance traveled along a path up to a given time
func (e *VehiclePositionEstimator) DistanceAlongPath(path *models.Path, elapsedSeconds int) int {
	if elapsedSeconds <= 0 {
		return 0
	}

	if elapsedSeconds >= path.TotalTime {
		return path.TotalWalk
	}

	cumulativeTime := 0
	cumulativeDistance := 0

	for _, edge := range path.Edges {
		segmentEndTime := cumulativeTime + edge.CostTime

		if elapsedSeconds >= segmentEndTime {
			// Completed this segment
			cumulativeDistance += edge.CostWalk
			cumulativeTime = segmentEndTime
		} else {
			// Partial segment
			progress := float64(elapsedSeconds-cumulativeTime) / float64(edge.CostTime)
			cumulativeDistance += int(float64(edge.CostWalk) * progress)
			break
		}
	}

	return cumulativeDistance
}

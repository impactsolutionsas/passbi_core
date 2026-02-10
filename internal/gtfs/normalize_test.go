package gtfs

import (
	"testing"

	"github.com/passbi/passbi_core/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestInferMode(t *testing.T) {
	tests := []struct {
		name     string
		route    models.GTFSRoute
		expected models.TransitMode
	}{
		{
			name: "Bus from route type",
			route: models.GTFSRoute{
				RouteID:   "1",
				RouteType: 3,
			},
			expected: models.ModeBus,
		},
		{
			name: "BRT from keyword",
			route: models.GTFSRoute{
				RouteID:   "2",
				ShortName: "BRT Line 1",
				RouteType: 3,
			},
			expected: models.ModeBRT,
		},
		{
			name: "Train from route type",
			route: models.GTFSRoute{
				RouteID:   "3",
				RouteType: 2,
			},
			expected: models.ModeTER,
		},
		{
			name: "Ferry from route type",
			route: models.GTFSRoute{
				RouteID:   "4",
				RouteType: 4,
			},
			expected: models.ModeFerry,
		},
		{
			name: "Default to bus",
			route: models.GTFSRoute{
				RouteID:   "5",
				RouteType: 999,
			},
			expected: models.ModeBus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InferMode(tt.route)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHaversineDistance(t *testing.T) {
	tests := []struct {
		name     string
		lat1     float64
		lon1     float64
		lat2     float64
		lon2     float64
		expected float64
		delta    float64
	}{
		{
			name:     "Zero distance",
			lat1:     14.7167,
			lon1:     -17.4677,
			lat2:     14.7167,
			lon2:     -17.4677,
			expected: 0,
			delta:    1,
		},
		{
			name:     "Approximately 1km",
			lat1:     14.7167,
			lon1:     -17.4677,
			lat2:     14.7257,
			lon2:     -17.4677,
			expected: 1000,
			delta:    100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := haversineDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			assert.InDelta(t, tt.expected, result, tt.delta)
		})
	}
}

func TestParseTimeToSeconds(t *testing.T) {
	tests := []struct {
		name     string
		timeStr  string
		expected int
		hasError bool
	}{
		{
			name:     "Valid time",
			timeStr:  "12:30:00",
			expected: 12*3600 + 30*60,
			hasError: false,
		},
		{
			name:     "Midnight",
			timeStr:  "00:00:00",
			expected: 0,
			hasError: false,
		},
		{
			name:     "Next day service",
			timeStr:  "25:30:00",
			expected: 25*3600 + 30*60,
			hasError: false,
		},
		{
			name:     "Invalid format",
			timeStr:  "12:30",
			expected: 0,
			hasError: true,
		},
		{
			name:     "Empty string",
			timeStr:  "",
			expected: 0,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTimeToSeconds(tt.timeStr)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestValidateAndCleanStops(t *testing.T) {
	tests := []struct {
		name     string
		stops    []models.GTFSStop
		expected int
	}{
		{
			name: "All valid stops",
			stops: []models.GTFSStop{
				{StopID: "1", Lat: 14.7, Lon: -17.4},
				{StopID: "2", Lat: 14.8, Lon: -17.5},
			},
			expected: 2,
		},
		{
			name: "Filter invalid latitude",
			stops: []models.GTFSStop{
				{StopID: "1", Lat: 14.7, Lon: -17.4},
				{StopID: "2", Lat: 95.0, Lon: -17.5},
			},
			expected: 1,
		},
		{
			name: "Filter null island",
			stops: []models.GTFSStop{
				{StopID: "1", Lat: 14.7, Lon: -17.4},
				{StopID: "2", Lat: 0.0, Lon: 0.0},
			},
			expected: 1,
		},
		{
			name: "Filter invalid longitude",
			stops: []models.GTFSStop{
				{StopID: "1", Lat: 14.7, Lon: -17.4},
				{StopID: "2", Lat: 14.8, Lon: 200.0},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAndCleanStops(tt.stops)
			assert.Equal(t, tt.expected, len(result))
		})
	}
}

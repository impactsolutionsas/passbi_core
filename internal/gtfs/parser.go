package gtfs

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/passbi/passbi_core/internal/models"
)

// GTFSFeed represents a parsed GTFS feed
type GTFSFeed struct {
	Agencies  []models.GTFSAgency
	Stops     []models.GTFSStop
	Routes    []models.GTFSRoute
	Trips     []models.GTFSTrip
	StopTimes []models.GTFSStopTime
}

// ParseGTFSZip extracts and parses a GTFS ZIP file
func ParseGTFSZip(zipPath string) (*GTFSFeed, error) {
	// Create temp directory for extraction
	tempDir, err := os.MkdirTemp("", "gtfs-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract ZIP
	if err := extractZip(zipPath, tempDir); err != nil {
		return nil, fmt.Errorf("failed to extract zip: %w", err)
	}

	// Parse GTFS files
	feed := &GTFSFeed{}

	// Parse agencies (optional)
	if agencies, err := ParseAgencies(filepath.Join(tempDir, "agency.txt")); err == nil {
		feed.Agencies = agencies
		log.Printf("Parsed %d agencies", len(agencies))
	} else {
		log.Printf("Warning: failed to parse agencies: %v", err)
	}

	// Parse stops (required)
	stops, err := ParseStops(filepath.Join(tempDir, "stops.txt"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse stops (required): %w", err)
	}
	feed.Stops = stops
	log.Printf("Parsed %d stops", len(stops))

	// Parse routes (required)
	routes, err := ParseRoutes(filepath.Join(tempDir, "routes.txt"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse routes (required): %w", err)
	}
	feed.Routes = routes
	log.Printf("Parsed %d routes", len(routes))

	// Parse trips (required)
	trips, err := ParseTrips(filepath.Join(tempDir, "trips.txt"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse trips (required): %w", err)
	}
	feed.Trips = trips
	log.Printf("Parsed %d trips", len(trips))

	// Parse stop_times (required)
	stopTimes, err := ParseStopTimes(filepath.Join(tempDir, "stop_times.txt"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse stop_times (required): %w", err)
	}
	feed.StopTimes = stopTimes
	log.Printf("Parsed %d stop_times", len(stopTimes))

	return feed, nil
}

// ParseAgencies parses agency.txt
func ParseAgencies(filePath string) ([]models.GTFSAgency, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseAgenciesFromReader(file)
}

func parseAgenciesFromReader(reader io.Reader) ([]models.GTFSAgency, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true

	// Read header
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	colMap := makeColumnMap(header)
	var agencies []models.GTFSAgency

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Warning: skipping malformed agency row: %v", err)
			continue
		}

		agency := models.GTFSAgency{
			AgencyID:   getField(record, colMap, "agency_id"),
			AgencyName: getField(record, colMap, "agency_name"),
			AgencyURL:  getField(record, colMap, "agency_url"),
			Timezone:   getField(record, colMap, "agency_timezone"),
		}

		agencies = append(agencies, agency)
	}

	return agencies, nil
}

// ParseStops parses stops.txt
func ParseStops(filePath string) ([]models.GTFSStop, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseStopsFromReader(file)
}

func parseStopsFromReader(reader io.Reader) ([]models.GTFSStop, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true

	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	colMap := makeColumnMap(header)
	var stops []models.GTFSStop

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Warning: skipping malformed stop row: %v", err)
			continue
		}

		stopID := getField(record, colMap, "stop_id")
		stopName := getField(record, colMap, "stop_name")
		latStr := getField(record, colMap, "stop_lat")
		lonStr := getField(record, colMap, "stop_lon")

		// Skip stops without required fields
		if stopID == "" || latStr == "" || lonStr == "" {
			log.Printf("Warning: skipping stop with missing required fields: %s", stopID)
			continue
		}

		lat, err := strconv.ParseFloat(latStr, 64)
		if err != nil {
			log.Printf("Warning: invalid latitude for stop %s: %v", stopID, err)
			continue
		}

		lon, err := strconv.ParseFloat(lonStr, 64)
		if err != nil {
			log.Printf("Warning: invalid longitude for stop %s: %v", stopID, err)
			continue
		}

		stop := models.GTFSStop{
			StopID:   stopID,
			StopName: stopName,
			Lat:      lat,
			Lon:      lon,
		}

		stops = append(stops, stop)
	}

	return stops, nil
}

// ParseRoutes parses routes.txt
func ParseRoutes(filePath string) ([]models.GTFSRoute, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseRoutesFromReader(file)
}

func parseRoutesFromReader(reader io.Reader) ([]models.GTFSRoute, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true

	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	colMap := makeColumnMap(header)
	var routes []models.GTFSRoute

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Warning: skipping malformed route row: %v", err)
			continue
		}

		routeID := getField(record, colMap, "route_id")
		if routeID == "" {
			continue
		}

		routeTypeStr := getField(record, colMap, "route_type")
		routeType, _ := strconv.Atoi(routeTypeStr)

		route := models.GTFSRoute{
			RouteID:    routeID,
			AgencyID:   getField(record, colMap, "agency_id"),
			ShortName:  getField(record, colMap, "route_short_name"),
			LongName:   getField(record, colMap, "route_long_name"),
			RouteType:  routeType,
			RouteColor: getField(record, colMap, "route_color"),
		}

		routes = append(routes, route)
	}

	return routes, nil
}

// ParseTrips parses trips.txt
func ParseTrips(filePath string) ([]models.GTFSTrip, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseTripsFromReader(file)
}

func parseTripsFromReader(reader io.Reader) ([]models.GTFSTrip, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true

	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	colMap := makeColumnMap(header)
	var trips []models.GTFSTrip

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Warning: skipping malformed trip row: %v", err)
			continue
		}

		tripID := getField(record, colMap, "trip_id")
		routeID := getField(record, colMap, "route_id")

		if tripID == "" || routeID == "" {
			continue
		}

		directionStr := getField(record, colMap, "direction_id")
		direction, _ := strconv.Atoi(directionStr)

		trip := models.GTFSTrip{
			RouteID:   routeID,
			ServiceID: getField(record, colMap, "service_id"),
			TripID:    tripID,
			Headsign:  getField(record, colMap, "trip_headsign"),
			Direction: direction,
		}

		trips = append(trips, trip)
	}

	return trips, nil
}

// ParseStopTimes parses stop_times.txt
func ParseStopTimes(filePath string) ([]models.GTFSStopTime, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseStopTimesFromReader(file)
}

func parseStopTimesFromReader(reader io.Reader) ([]models.GTFSStopTime, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true

	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	colMap := makeColumnMap(header)
	var stopTimes []models.GTFSStopTime

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Warning: skipping malformed stop_time row: %v", err)
			continue
		}

		tripID := getField(record, colMap, "trip_id")
		stopID := getField(record, colMap, "stop_id")
		seqStr := getField(record, colMap, "stop_sequence")

		if tripID == "" || stopID == "" || seqStr == "" {
			continue
		}

		sequence, err := strconv.Atoi(seqStr)
		if err != nil {
			log.Printf("Warning: invalid sequence for trip %s: %v", tripID, err)
			continue
		}

		stopTime := models.GTFSStopTime{
			TripID:        tripID,
			ArrivalTime:   getField(record, colMap, "arrival_time"),
			DepartureTime: getField(record, colMap, "departure_time"),
			StopID:        stopID,
			StopSequence:  sequence,
		}

		stopTimes = append(stopTimes, stopTime)
	}

	return stopTimes, nil
}

// Helper functions

func makeColumnMap(header []string) map[string]int {
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(col)] = i
	}
	return colMap
}

func getField(record []string, colMap map[string]int, fieldName string) string {
	if idx, ok := colMap[fieldName]; ok && idx < len(record) {
		return strings.TrimSpace(record[idx])
	}
	return ""
}

func extractZip(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// Open file in zip
		rc, err := file.Open()
		if err != nil {
			return err
		}

		// Create destination file
		destPath := filepath.Join(destDir, filepath.Base(file.Name))
		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return err
		}

		// Copy contents
		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/passbi/passbi_core/internal/db"
	"github.com/passbi/passbi_core/internal/graph"
	"github.com/passbi/passbi_core/internal/gtfs"
	"github.com/passbi/passbi_core/internal/models"
)

func main() {
	// Command-line flags
	agencyID := flag.String("agency-id", "", "Agency ID for this GTFS feed (required)")
	gtfsPath := flag.String("gtfs", "", "Path to GTFS ZIP file (required)")
	rebuildGraph := flag.Bool("rebuild-graph", false, "Rebuild graph after import")
	dedupeThreshold := flag.Float64("dedupe-threshold", 30.0, "Stop deduplication threshold in meters")

	flag.Parse()

	// Validate required flags
	if *agencyID == "" || *gtfsPath == "" {
		fmt.Println("Usage: passbi-import --agency-id=<id> --gtfs=<path.zip> [--rebuild-graph] [--dedupe-threshold=30]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Validate file exists
	if _, err := os.Stat(*gtfsPath); os.IsNotExist(err) {
		log.Fatalf("GTFS file not found: %s", *gtfsPath)
	}

	log.Println("Starting GTFS import...")
	log.Printf("Agency ID: %s", *agencyID)
	log.Printf("GTFS file: %s", *gtfsPath)

	// Initialize database connection
	pool, err := db.GetDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create import log entry
	importLogID, err := createImportLog(ctx, pool, *agencyID)
	if err != nil {
		log.Fatalf("Failed to create import log: %v", err)
	}

	// Run import in transaction
	if err := runImport(ctx, pool, *agencyID, *gtfsPath, *dedupeThreshold, *rebuildGraph, importLogID); err != nil {
		// Update log as failed
		updateImportLog(ctx, pool, importLogID, "failed", 0, 0, 0, 0, err.Error())
		log.Fatalf("Import failed: %v", err)
	}

	log.Println("Import completed successfully!")
	os.Exit(0)
}

func runImport(ctx context.Context, pool *pgxpool.Pool, agencyID, gtfsPath string, dedupeThreshold float64, rebuildGraph bool, logID int64) error {
	startTime := time.Now()

	// Parse GTFS feed
	log.Println("Step 1/5: Parsing GTFS feed...")
	feed, err := gtfs.ParseGTFSZip(gtfsPath)
	if err != nil {
		return fmt.Errorf("failed to parse GTFS: %w", err)
	}

	// Validate and clean stops
	log.Println("Step 2/5: Validating and cleaning stops...")
	feed.Stops = gtfs.ValidateAndCleanStops(feed.Stops)

	// Deduplicate stops
	log.Println("Step 3/5: Deduplicating stops...")
	var stopMapping map[string]string
	feed.Stops, stopMapping, err = gtfs.DeduplicateStops(ctx, pool, feed.Stops, dedupeThreshold)
	if err != nil {
		return fmt.Errorf("failed to deduplicate stops: %w", err)
	}

	// Remap stop IDs in stop_times to use deduplicated stops
	for i := range feed.StopTimes {
		if newID, ok := stopMapping[feed.StopTimes[i].StopID]; ok {
			feed.StopTimes[i].StopID = newID
		}
	}

	// Begin transaction
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Import stops
	log.Println("Step 4/5: Importing stops and routes to database...")
	if err := importStops(ctx, tx, agencyID, feed.Stops); err != nil {
		return fmt.Errorf("failed to import stops: %w", err)
	}

	// Import routes
	if err := importRoutes(ctx, tx, agencyID, feed.Routes); err != nil {
		return fmt.Errorf("failed to import routes: %w", err)
	}

	// Import trips
	if err := importTrips(ctx, tx, agencyID, feed.Trips); err != nil {
		return fmt.Errorf("failed to import trips: %w", err)
	}

	// Import calendar
	if err := importCalendar(ctx, tx, agencyID, feed.Calendars); err != nil {
		return fmt.Errorf("failed to import calendar: %w", err)
	}

	// Import calendar_dates
	if err := importCalendarDates(ctx, tx, agencyID, feed.CalendarDates); err != nil {
		return fmt.Errorf("failed to import calendar_dates: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Import stop_times in separate chunked transactions (too large for single tx)
	log.Printf("Step 4b/5: Importing %d stop_times...", len(feed.StopTimes))
	if err := importStopTimesChunked(ctx, pool, agencyID, feed.StopTimes); err != nil {
		return fmt.Errorf("failed to import stop_times: %w", err)
	}

	// Build graph (if requested)
	nodeCount := 0
	edgeCount := 0

	if rebuildGraph {
		log.Println("Step 5/5: Building routing graph...")
		builder := graph.NewBuilder(pool)
		if err := builder.BuildGraph(ctx, feed); err != nil {
			return fmt.Errorf("failed to build graph: %w", err)
		}

		// Count nodes and edges
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM node").Scan(&nodeCount); err != nil {
			log.Printf("Warning: failed to count nodes: %v", err)
		}
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM edge").Scan(&edgeCount); err != nil {
			log.Printf("Warning: failed to count edges: %v", err)
		}
	} else {
		log.Println("Step 5/5: Skipping graph build (use --rebuild-graph to enable)")
	}

	// Update import log
	duration := time.Since(startTime)
	log.Printf("Import completed in %s", duration)

	return updateImportLog(ctx, pool, logID, "success",
		len(feed.Stops), len(feed.Routes), nodeCount, edgeCount, "")
}

func createImportLog(ctx context.Context, pool *pgxpool.Pool, agencyID string) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO import_log (agency_id, status)
		VALUES ($1, 'running')
		RETURNING id
	`, agencyID).Scan(&id)

	return id, err
}

func updateImportLog(ctx context.Context, pool *pgxpool.Pool, id int64, status string, stops, routes, nodes, edges int, errMsg string) error {
	// Build message with stats
	message := errMsg
	if status == "success" {
		message = fmt.Sprintf("Imported %d stops, %d routes, %d nodes, %d edges", stops, routes, nodes, edges)
	}

	_, err := pool.Exec(ctx, `
		UPDATE import_log
		SET completed_at = NOW(),
		    status = $2,
		    message = $3
		WHERE id = $1
	`, id, status, message)

	return err
}

func importStops(ctx context.Context, tx pgx.Tx, agencyID string, stops []models.GTFSStop) error {
	batch := &pgx.Batch{}

	for _, stop := range stops {
		batch.Queue(`
			INSERT INTO stop (id, name, lat, lon, agency_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE
			SET name = EXCLUDED.name,
			    lat = EXCLUDED.lat,
			    lon = EXCLUDED.lon,
			    agency_id = EXCLUDED.agency_id
		`, stop.StopID, stop.StopName, stop.Lat, stop.Lon, agencyID)
	}

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	for i := 0; i < batch.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("failed to insert stop %d: %w", i, err)
		}
	}

	log.Printf("Imported %d stops", len(stops))
	return nil
}

func importRoutes(ctx context.Context, tx pgx.Tx, agencyID string, routes []models.GTFSRoute) error {
	batch := &pgx.Batch{}

	for _, route := range routes {
		mode := gtfs.InferMode(route)

		batch.Queue(`
			INSERT INTO route (id, agency_id, short_name, long_name, mode)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE
			SET agency_id = EXCLUDED.agency_id,
			    short_name = EXCLUDED.short_name,
			    long_name = EXCLUDED.long_name,
			    mode = EXCLUDED.mode
		`, route.RouteID, agencyID, route.ShortName, route.LongName, mode)
	}

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	for i := 0; i < batch.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("failed to insert route %d: %w", i, err)
		}
	}

	log.Printf("Imported %d routes", len(routes))
	return nil
}

func importTrips(ctx context.Context, tx pgx.Tx, agencyID string, trips []models.GTFSTrip) error {
	if len(trips) == 0 {
		log.Println("No trips to import")
		return nil
	}

	batch := &pgx.Batch{}
	count := 0

	for _, trip := range trips {
		batch.Queue(`
			INSERT INTO trip (trip_id, agency_id, route_id, service_id, headsign, direction)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (agency_id, trip_id) DO UPDATE
			SET route_id = EXCLUDED.route_id,
			    service_id = EXCLUDED.service_id,
			    headsign = EXCLUDED.headsign,
			    direction = EXCLUDED.direction
		`, trip.TripID, agencyID, trip.RouteID, trip.ServiceID, trip.Headsign, trip.Direction)

		count++
		if batch.Len() >= 1000 {
			results := tx.SendBatch(ctx, batch)
			for i := 0; i < batch.Len(); i++ {
				if _, err := results.Exec(); err != nil {
					results.Close()
					return fmt.Errorf("failed to insert trip batch at %d: %w", count, err)
				}
			}
			results.Close()
			batch = &pgx.Batch{}
		}
	}

	if batch.Len() > 0 {
		results := tx.SendBatch(ctx, batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err := results.Exec(); err != nil {
				results.Close()
				return fmt.Errorf("failed to insert trip final batch: %w", err)
			}
		}
		results.Close()
	}

	log.Printf("Imported %d trips", count)
	return nil
}

func importStopTimesChunked(ctx context.Context, pool *pgxpool.Pool, agencyID string, stopTimes []models.GTFSStopTime) error {
	if len(stopTimes) == 0 {
		log.Println("No stop_times to import")
		return nil
	}

	chunkSize := 50000
	total := len(stopTimes)

	for start := 0; start < total; start += chunkSize {
		end := start + chunkSize
		if end > total {
			end = total
		}
		chunk := stopTimes[start:end]

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin tx at offset %d: %w", start, err)
		}

		batch := &pgx.Batch{}
		for _, st := range chunk {
			arrSec, _ := gtfs.ParseTimeToSeconds(st.ArrivalTime)
			depSec, _ := gtfs.ParseTimeToSeconds(st.DepartureTime)

			batch.Queue(`
				INSERT INTO stop_time (trip_id, agency_id, stop_id, stop_sequence,
					arrival_time, departure_time, arrival_seconds, departure_seconds)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (agency_id, trip_id, stop_sequence) DO UPDATE
				SET stop_id = EXCLUDED.stop_id,
				    arrival_time = EXCLUDED.arrival_time,
				    departure_time = EXCLUDED.departure_time,
				    arrival_seconds = EXCLUDED.arrival_seconds,
				    departure_seconds = EXCLUDED.departure_seconds
			`, st.TripID, agencyID, st.StopID, st.StopSequence,
				st.ArrivalTime, st.DepartureTime, arrSec, depSec)

			if batch.Len() >= 1000 {
				results := tx.SendBatch(ctx, batch)
				for i := 0; i < batch.Len(); i++ {
					if _, err := results.Exec(); err != nil {
						results.Close()
						tx.Rollback(ctx)
						return fmt.Errorf("failed to insert stop_time batch: %w", err)
					}
				}
				results.Close()
				batch = &pgx.Batch{}
			}
		}

		if batch.Len() > 0 {
			results := tx.SendBatch(ctx, batch)
			for i := 0; i < batch.Len(); i++ {
				if _, err := results.Exec(); err != nil {
					results.Close()
					tx.Rollback(ctx)
					return fmt.Errorf("failed to insert stop_time final batch: %w", err)
				}
			}
			results.Close()
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit stop_times chunk at %d: %w", start, err)
		}

		log.Printf("  Imported stop_times %d-%d / %d", start+1, end, total)
	}

	log.Printf("Imported %d stop_times total", total)
	return nil
}

func importCalendar(ctx context.Context, tx pgx.Tx, agencyID string, calendars []models.GTFSCalendar) error {
	if len(calendars) == 0 {
		log.Println("No calendar entries to import")
		return nil
	}

	batch := &pgx.Batch{}

	for _, cal := range calendars {
		startDate := parseGTFSDate(cal.StartDate)
		endDate := parseGTFSDate(cal.EndDate)

		batch.Queue(`
			INSERT INTO calendar (service_id, agency_id, monday, tuesday, wednesday,
				thursday, friday, saturday, sunday, start_date, end_date)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (agency_id, service_id) DO UPDATE
			SET monday = EXCLUDED.monday, tuesday = EXCLUDED.tuesday,
			    wednesday = EXCLUDED.wednesday, thursday = EXCLUDED.thursday,
			    friday = EXCLUDED.friday, saturday = EXCLUDED.saturday,
			    sunday = EXCLUDED.sunday, start_date = EXCLUDED.start_date,
			    end_date = EXCLUDED.end_date
		`, cal.ServiceID, agencyID,
			cal.Monday, cal.Tuesday, cal.Wednesday, cal.Thursday,
			cal.Friday, cal.Saturday, cal.Sunday, startDate, endDate)
	}

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	for i := 0; i < batch.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("failed to insert calendar %d: %w", i, err)
		}
	}

	log.Printf("Imported %d calendar entries", len(calendars))
	return nil
}

func importCalendarDates(ctx context.Context, tx pgx.Tx, agencyID string, calDates []models.GTFSCalendarDate) error {
	if len(calDates) == 0 {
		log.Println("No calendar_dates to import")
		return nil
	}

	batch := &pgx.Batch{}

	for _, cd := range calDates {
		date := parseGTFSDate(cd.Date)

		batch.Queue(`
			INSERT INTO calendar_date (service_id, agency_id, date, exception_type)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (agency_id, service_id, date) DO UPDATE
			SET exception_type = EXCLUDED.exception_type
		`, cd.ServiceID, agencyID, date, cd.ExceptionType)
	}

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	for i := 0; i < batch.Len(); i++ {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("failed to insert calendar_date %d: %w", i, err)
		}
	}

	log.Printf("Imported %d calendar_dates", len(calDates))
	return nil
}

func parseGTFSDate(dateStr string) time.Time {
	t, err := time.Parse("20060102", dateStr)
	if err != nil {
		return time.Time{}
	}
	return t
}

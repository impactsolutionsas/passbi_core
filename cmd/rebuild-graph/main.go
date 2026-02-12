package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/passbi/passbi_core/internal/db"
	"github.com/passbi/passbi_core/internal/graph"
)

func main() {
	log.Println("🔄 PassBi Core - Graph Rebuild Tool")
	log.Println("===================================")

	// Connect to database
	log.Println("📡 Connecting to database...")
	dbPool, err := db.GetDB()
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}
	defer db.Close()

	log.Println("✅ Database connected")

	ctx := context.Background()

	// Check data availability
	var stopCount, routeCount, tripCount int
	err = dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM stop").Scan(&stopCount)
	if err != nil {
		log.Fatalf("❌ Failed to count stops: %v", err)
	}
	err = dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM route").Scan(&routeCount)
	if err != nil {
		log.Fatalf("❌ Failed to count routes: %v", err)
	}
	err = dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM trip").Scan(&tripCount)
	if err != nil {
		log.Fatalf("❌ Failed to count trips: %v", err)
	}

	log.Printf("📊 Database statistics:")
	log.Printf("   Stops: %d", stopCount)
	log.Printf("   Routes: %d", routeCount)
	log.Printf("   Trips: %d", tripCount)

	if stopCount == 0 || routeCount == 0 || tripCount == 0 {
		log.Fatalf("❌ No data found in database. Import GTFS data first!")
	}

	// Confirm rebuild
	fmt.Println()
	fmt.Println("⚠️  This will DELETE all existing nodes and edges!")
	fmt.Print("Continue? (yes/no): ")
	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "yes" && confirm != "y" {
		log.Println("❌ Rebuild cancelled")
		os.Exit(0)
	}

	// Rebuild graph
	fmt.Println()
	log.Println("🔄 Starting graph rebuild...")
	startTime := time.Now()

	builder := graph.NewBuilder(dbPool)
	err = builder.BuildGraphFromDB(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to rebuild graph: %v", err)
	}

	duration := time.Since(startTime)

	// Show results
	var nodeCount, edgeCount int
	err = dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM node").Scan(&nodeCount)
	if err != nil {
		log.Printf("⚠️  Failed to count nodes: %v", err)
	}
	err = dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM edge").Scan(&edgeCount)
	if err != nil {
		log.Printf("⚠️  Failed to count edges: %v", err)
	}

	fmt.Println()
	log.Println("✅ Graph rebuild completed!")
	log.Printf("⏱️  Duration: %v", duration)
	log.Printf("📊 Graph statistics:")
	log.Printf("   Nodes: %d", nodeCount)
	log.Printf("   Edges: %d", edgeCount)

	// Check coverage
	var stopsWithNodes int
	err = dbPool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT stop_id) FROM node
	`).Scan(&stopsWithNodes)
	if err == nil {
		coverage := float64(stopsWithNodes) / float64(stopCount) * 100
		log.Printf("   Stop coverage: %d/%d (%.1f%%)", stopsWithNodes, stopCount, coverage)
	}

	fmt.Println()
	log.Println("🚀 Graph is ready for routing!")
}

package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	// Build connection string from env
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	sslmode := os.Getenv("DB_SSLMODE")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	fmt.Println("🔗 Testing Supabase connection...")
	fmt.Printf("   Host: %s:%s\n", host, port)
	fmt.Printf("   User: %s\n", user)
	fmt.Printf("   Database: %s\n\n", dbname)

	// Test connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("❌ Failed to create connection: %v\n", err)
	}
	defer db.Close()

	// Ping database
	if err := db.Ping(); err != nil {
		log.Fatalf("❌ Failed to ping database: %v\n", err)
	}

	fmt.Println("✅ Connection successful!\n")

	// Check PostgreSQL version
	var pgVersion string
	err = db.QueryRow("SELECT version()").Scan(&pgVersion)
	if err != nil {
		log.Printf("⚠️  Could not get PostgreSQL version: %v\n", err)
	} else {
		fmt.Printf("📊 PostgreSQL Version:\n   %s\n\n", pgVersion)
	}

	// Check PostGIS
	var postgisVersion string
	err = db.QueryRow("SELECT PostGIS_Version()").Scan(&postgisVersion)
	if err != nil {
		fmt.Println("⚠️  PostGIS NOT enabled")
		fmt.Println("   → Please enable PostGIS extension in Supabase Dashboard:")
		fmt.Println("   → https://app.supabase.com/project/xlvuggzprjjkzolonbuh/database/extensions")
	} else {
		fmt.Printf("✅ PostGIS Version: %s\n\n", postgisVersion)
	}

	// Check existing tables
	fmt.Println("📋 Checking existing tables...")
	rows, err := db.Query(`
		SELECT tablename
		FROM pg_tables
		WHERE schemaname = 'public'
		ORDER BY tablename
	`)
	if err != nil {
		log.Printf("⚠️  Could not list tables: %v\n", err)
	} else {
		defer rows.Close()
		tableCount := 0
		for rows.Next() {
			var tablename string
			if err := rows.Scan(&tablename); err != nil {
				continue
			}
			fmt.Printf("   - %s\n", tablename)
			tableCount++
		}
		if tableCount == 0 {
			fmt.Println("   (no tables found - migrations need to be run)")
		}
		fmt.Printf("\n   Total: %d tables\n", tableCount)
	}

	fmt.Println("\n✅ Connection test completed successfully!")
}

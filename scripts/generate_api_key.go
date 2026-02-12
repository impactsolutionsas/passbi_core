package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
)

func main() {
	env := flag.String("env", "test", "Environment: test or live")
	flag.Parse()

	if *env != "test" && *env != "live" {
		fmt.Println("Error: env must be 'test' or 'live'")
		os.Exit(1)
	}

	key, hash, prefix := generateAPIKey(*env)

	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("🔑 API Key Generated")
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Printf("Environment:  %s\n", *env)
	fmt.Printf("\nAPI Key (show ONLY ONCE):\n%s\n", key)
	fmt.Printf("\nHash (store in database):\n%s\n", hash)
	fmt.Printf("\nPrefix (for display):\n%s\n", prefix)
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("\n⚠️  Save the API key now! You won't be able to see it again.")
	fmt.Println("\nTo insert into database:")
	fmt.Printf("INSERT INTO api_key (partner_id, key_hash, key_prefix, name, scopes)\n")
	fmt.Printf("VALUES ('PARTNER_ID', '%s', '%s', 'Key Name', ARRAY['read:routes']);\n", hash, prefix)
	fmt.Println("═══════════════════════════════════════════════════")
}

// generateAPIKey generates a new API key with hash and prefix
func generateAPIKey(env string) (key, hash, prefix string) {
	// Generate 32 random bytes
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		panic(err)
	}
	randomStr := hex.EncodeToString(randomBytes)

	// Generate checksum (first 2 bytes of hash)
	checksumBytes := sha256.Sum256([]byte(randomStr))
	checksum := hex.EncodeToString(checksumBytes[:2])

	// Construct the key
	key = fmt.Sprintf("pk_%s_%s_%s", env, randomStr, checksum)

	// Hash for storage
	hashBytes := sha256.Sum256([]byte(key))
	hash = hex.EncodeToString(hashBytes[:])

	// Prefix for display (first 12 chars after pk_env_)
	prefix = fmt.Sprintf("pk_%s_%s...", env, randomStr[:8])

	return
}

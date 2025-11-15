package main

import (
	"fmt"
	"log"
	"os"

	"github.com/palbooo/push-receiver-go/pkg/register"
)

// Example showing only the registration flow without listening
func main() {
	// Get auth token from environment or command line
	authToken := os.Getenv("RUST_AUTH_TOKEN")
	if authToken == "" && len(os.Args) > 1 {
		authToken = os.Args[1]
	}

	if authToken == "" {
		log.Fatal("Please provide RUST_AUTH_TOKEN environment variable or pass it as first argument")
	}

	fmt.Println("=== Rust+ FCM Registration ===\n")

	// Option 1: Register with JWT (extracts Steam ID automatically)
	result, err := register.RegisterWithJWT(authToken)
	if err != nil {
		log.Fatalf("Registration failed: %v", err)
	}

	// Option 2: If you already have the Steam ID
	// steamID := "76561198880712723"
	// result, err := register.Register(steamID, authToken)

	fmt.Println("\n=== Registration Successful ===")
	fmt.Printf("Steam ID: %s\n", result.SteamID)
	fmt.Printf("Android ID: %s\n", result.FCMCredentials.GCM.AndroidID)
	fmt.Printf("Security Token: %s\n", result.FCMCredentials.GCM.SecurityToken)
	fmt.Printf("FCM Token: %s\n", result.FCMCredentials.FCM.Token)
	fmt.Printf("Expo Push Token: %s\n", result.ExpoPushToken)
	fmt.Printf("Updated Auth Token: %s\n", result.AuthToken)

	// Save credentials to JSON file
	jsonOutput, err := result.ToJSONIndent()
	if err != nil {
		log.Fatalf("Failed to convert to JSON: %v", err)
	}

	filename := "fcm_credentials.json"
	if err := os.WriteFile(filename, []byte(jsonOutput), 0644); err != nil {
		log.Fatalf("Failed to write to file: %v", err)
	}

	fmt.Printf("\n✅ Credentials saved to %s\n", filename)
	fmt.Println("\nYou can now use these credentials with the FCM listener:")
	fmt.Printf("  androidID: %s\n", result.FCMCredentials.GCM.AndroidID)
	fmt.Printf("  securityToken: %s\n", result.FCMCredentials.GCM.SecurityToken)
}

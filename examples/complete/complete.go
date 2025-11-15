package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/palbooo/push-receiver-go/pkg/client"
	"github.com/palbooo/push-receiver-go/pkg/register"
)

// Example showing complete registration flow with FCM and listening for notifications
func main() {
	// Get auth token from environment or command line
	authToken := os.Getenv("RUST_AUTH_TOKEN")
	if authToken == "" && len(os.Args) > 1 {
		authToken = os.Args[1]
	}

	if authToken == "" {
		log.Fatal("Please provide RUST_AUTH_TOKEN environment variable or pass it as first argument")
	}

	fmt.Println("=== Starting Rust+ FCM Registration ===\n")

	// Step 1: Register with FCM and Rust+ API
	fmt.Println("Registering with FCM and Rust+ API...")
	result, err := register.RegisterWithJWT(authToken)
	if err != nil {
		log.Fatalf("Registration failed: %v", err)
	}

	fmt.Println("\n=== Registration Successful ===")
	fmt.Printf("Steam ID: %s\n", result.SteamID)
	fmt.Printf("Android ID: %s\n", result.FCMCredentials.GCM.AndroidID)
	fmt.Printf("Security Token: %s\n", result.FCMCredentials.GCM.SecurityToken)
	fmt.Printf("FCM Token: %s\n", result.FCMCredentials.FCM.Token)
	fmt.Printf("Expo Push Token: %s\n", result.ExpoPushToken)
	fmt.Printf("Updated Auth Token: %s\n", result.AuthToken)

	// Optional: Save credentials to JSON
	jsonOutput, err := result.ToJSONIndent()
	if err == nil {
		fmt.Println("\n=== Registration Result (JSON) ===")
		fmt.Println(jsonOutput)
	}

	// Step 2: Start listening for notifications
	fmt.Println("\n=== Starting FCM Listener ===")
	fcmClient := client.NewClient(
		result.FCMCredentials.GCM.AndroidID,
		result.FCMCredentials.GCM.SecurityToken,
		nil,
	)

	if err := fcmClient.Connect(); err != nil {
		log.Fatalf("Failed to connect to FCM: %v", err)
	}

	fmt.Println("🎧 Listening for Rust+ notifications...")
	fmt.Println("Press Ctrl+C to stop\n")

	// Handle incoming messages
	go func() {
		for event := range fcmClient.Events() {
			switch event.Type {
			case client.EventConnect:
				fmt.Println("✅ Connected to FCM")

			case client.EventDisconnect:
				fmt.Println("❌ Disconnected from FCM")

			case client.EventDataReceived:
				handleRustPlusMessage(event.Data)

			case client.EventNotificationReceived:
				handleRustPlusNotification(event.Data)

			case client.EventError:
				fmt.Printf("❌ Error: %v\n", event.Data)
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n\nShutting down...")
	fcmClient.Close()
}

func handleRustPlusMessage(data interface{}) {
	msg, ok := data.(map[string]interface{})
	if !ok {
		return
	}

	appData, _ := msg["appData"].(map[string]string)

	fmt.Printf("\n🎮 Rust+ Data Message\n")
	fmt.Printf("├─ Category: %v\n", msg["category"])
	fmt.Printf("├─ From: %v\n", msg["from"])

	if title, ok := appData["title"]; ok {
		fmt.Printf("├─ Title: %s\n", title)
	}

	if body, ok := appData["body"]; ok {
		fmt.Printf("├─ Body: %s\n", body)
	}

	// Check for additional app data
	for key, value := range appData {
		if key != "title" && key != "body" {
			fmt.Printf("├─ %s: %s\n", key, value)
		}
	}

	if rawData, ok := msg["rawData"].([]byte); ok && len(rawData) > 0 {
		fmt.Printf("└─ Raw Data: %d bytes\n", len(rawData))
	} else {
		fmt.Println("└─ (end)")
	}
}

func handleRustPlusNotification(data interface{}) {
	msg, ok := data.(map[string]interface{})
	if !ok {
		return
	}

	appData, _ := msg["appData"].(map[string]string)

	fmt.Printf("\n🔔 Rust+ Notification (Encrypted)\n")
	fmt.Printf("├─ Category: %v\n", msg["category"])
	fmt.Printf("├─ From: %v\n", msg["from"])

	// Encrypted notifications contain crypto-key
	if cryptoKey, ok := appData["crypto-key"]; ok {
		fmt.Printf("├─ Crypto Key: %s...\n", cryptoKey[:20])
	}

	if rawData, ok := msg["rawData"].([]byte); ok && len(rawData) > 0 {
		fmt.Printf("└─ Encrypted Data: %d bytes\n", len(rawData))
	} else {
		fmt.Println("└─ (end)")
	}
}

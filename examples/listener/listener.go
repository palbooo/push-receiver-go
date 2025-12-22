package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/palbooo/push-receiver-go/pkg/client"
	"github.com/palbooo/push-receiver-go/pkg/register"
)

// Example showing only the FCM listener with existing credentials
// Use this if you already have androidID and securityToken from a previous registration
func main() {
	var androidID, securityToken string

	// Try to load from JSON file first (created by register_only.go)
	if _, err := os.Stat("fcm_credentials.json"); err == nil {
		fmt.Println("Loading credentials from fcm_credentials.json...")
		data, err := os.ReadFile("fcm_credentials.json")
		if err == nil {
			var result register.RegistrationResult
			if err := json.Unmarshal(data, &result); err == nil {
				androidID = result.FCMCredentials.GCM.AndroidID
				securityToken = result.FCMCredentials.GCM.SecurityToken
				fmt.Printf("✅ Loaded credentials for Steam ID: %s\n\n", result.SteamID)
			}
		}
	}

	// If not loaded from file, get from command line
	if androidID == "" {
		androidID = os.Getenv("ANDROID_ID")
		securityToken = os.Getenv("SECURITY_TOKEN")

		if androidID == "" && len(os.Args) > 2 {
			androidID = os.Args[1]
			securityToken = os.Args[2]
		}

		if androidID == "" || securityToken == "" {
			fmt.Println("Usage:")
			fmt.Println("  Option 1: Place fcm_credentials.json in current directory (from register_only.go)")
			fmt.Println("  Option 2: Set environment variables:")
			fmt.Println("    $env:ANDROID_ID=\"your-android-id\"")
			fmt.Println("    $env:SECURITY_TOKEN=\"your-security-token\"")
			fmt.Println("  Option 3: Pass as arguments:")
			fmt.Println("    go run listener_only.go <android-id> <security-token>")
			log.Fatal("\nPlease provide FCM credentials")
		}
	}

	fmt.Println("=== Starting FCM Listener ===")
	fmt.Printf("Android ID: %s\n", androidID)
	if len(securityToken) > 20 {
		fmt.Printf("Security Token: %s...\n\n", securityToken[:20])
	} else {
		fmt.Printf("Security Token: %s\n\n", securityToken)
	}

	// Create FCM client
	fcmClient := client.NewClient(androidID, securityToken, nil)

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
				fmt.Println("⚠️  Disconnected from FCM - attempting to reconnect...")

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

	fmt.Println("\n\n👋 Shutting down...")
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
	fmt.Printf("├─ Persistent ID: %v\n", msg["persistentId"])

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
	fmt.Printf("├─ Persistent ID: %v\n", msg["persistentId"])

	// Encrypted notifications contain crypto-key
	if cryptoKey, ok := appData["crypto-key"]; ok {
		fmt.Printf("├─ Crypto Key: %s...\n", cryptoKey[:min(20, len(cryptoKey))])
	}

	if salt, ok := appData["salt"]; ok {
		fmt.Printf("├─ Salt: %s...\n", salt[:min(20, len(salt))])
	}

	if encoding, ok := appData["encoding"]; ok {
		fmt.Printf("├─ Encoding: %s\n", encoding)
	}

	if rawData, ok := msg["rawData"].([]byte); ok && len(rawData) > 0 {
		fmt.Printf("└─ Encrypted Data: %d bytes\n", len(rawData))
	} else {
		fmt.Println("└─ (end)")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

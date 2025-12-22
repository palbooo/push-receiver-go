package register

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/palbooo/push-receiver-go/internal/utils"
)

// Service handles FCM registration operations
type Service struct {
	config *Config
}

// NewService creates a new FCM registration service
func NewService(config *Config) *Service {
	if config == nil {
		config = DefaultConfig()
	}
	return &Service{
		config: config,
	}
}

// Register is a convenience function that performs the complete FCM registration flow
// using the default configuration. This is the simplest way to use the package.
//
// For custom configuration, create a Service with NewService() instead.
//
// Example:
//
//	result, err := register.Register(steamID, authToken)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("FCM Token: %s\n", result.FCMCredentials.FCM.Token)
func Register(steamID, authToken string) (*RegistrationResult, error) {
	service := NewService(nil)
	return service.Register(steamID, authToken)
}

// RegisterWithJWT is a convenience function that extracts the Steam ID from the JWT
// and performs the complete FCM registration flow using default configuration.
// This is the absolute simplest way to use the package - just pass the auth token.
//
// Example:
//
//	authToken := "eyJzdGVhbUlkIjoiNzY1NjExOTg4ODA3MTI3MjMi..."
//	result, err := register.RegisterWithJWT(authToken)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Steam ID: %s\n", result.SteamID)
func RegisterWithJWT(authToken string) (*RegistrationResult, error) {
	steamID, err := ExtractSteamIDFromJWT(authToken)
	if err != nil {
		return nil, fmt.Errorf("failed to extract steam ID: %w", err)
	}
	return Register(steamID, authToken)
}

// ExpoPushToken exchanges an FCM token for an Expo push token
func (s *Service) ExpoPushToken(fcmToken string) (string, error) {
	requestBody := ExpoPushTokenRequest{
		Type:        "fcm",
		DeviceID:    uuid.New().String(),
		Development: false,
		AppID:       s.config.FCM.AndroidPackageName,
		DeviceToken: fcmToken,
		ProjectID:   "49451aca-a822-41e6-ad59-955718d0ff9c",
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	responseBody, err := utils.SimpleRequest(utils.RequestOptions{
		URL:    s.config.ExpoPushTokenURL,
		Method: "POST",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: bodyBytes,
	})
	if err != nil {
		return "", fmt.Errorf("expo push token request failed: %w", err)
	}

	var response ExpoPushTokenResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if response.Data.ExpoPushToken == "" {
		return "", fmt.Errorf("empty expo push token in response")
	}

	return response.Data.ExpoPushToken, nil
}

// RegisterRustPlus registers the device with the Rust Companion API
// Returns the updated authToken from the API response
func (s *Service) RegisterRustPlus(authToken, expoPushToken string) (string, error) {
	// Decode URL-encoded token before sending
	decodedToken, err := url.QueryUnescape(authToken)
	if err != nil {
		decodedToken = authToken // Use original if decode fails
	}

	fmt.Println("Registering with Rust Companion API...")

	requestBody := RustPlusRequest{
		AuthToken: decodedToken,
		DeviceID:  "rustplus.app",
		PushKind:  3,
		PushToken: expoPushToken,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", s.config.RustPlusAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("rustplus registration request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for 200 OK status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("rustplus registration failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	fmt.Printf("Rust Companion API responded with status: %d\n", resp.StatusCode)

	// Parse response to get the updated authToken
	var rustPlusResponse RustPlusResponse
	if err := json.Unmarshal(responseBody, &rustPlusResponse); err != nil {
		return "", fmt.Errorf("failed to parse rustplus response: %w", err)
	}

	if rustPlusResponse.AuthToken == "" {
		return "", fmt.Errorf("empty authToken in rustplus response")
	}

	fmt.Printf("Received updated authToken from Rust Companion API\n")
	return rustPlusResponse.AuthToken, nil
}

// Register performs the complete FCM registration flow
func (s *Service) Register(steamID, authToken string) (*RegistrationResult, error) {
	fmt.Printf("Starting FCM registration process for Steam ID: %s...\n", steamID)

	// Step 1: Register with FCM
	fmt.Println("Registering with FCM...")
	androidFCM := NewAndroidFCM(s.config)
	fcmCredentials, err := androidFCM.Register()
	if err != nil {
		return nil, fmt.Errorf("fcm registration failed: %w", err)
	}

	fmt.Println("FCM Registration successful")
	fmt.Printf("FCM Token: %s\n", fcmCredentials.FCM.Token)

	// Step 2: Get Expo push token
	fmt.Println("Fetching Expo Push Token...")
	expoPushToken, err := s.ExpoPushToken(fcmCredentials.FCM.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to get expo push token: %w", err)
	}

	fmt.Printf("Expo Push Token: %s\n", expoPushToken)

	// Step 3: Register with RustPlus
	updatedAuthToken, err := s.RegisterRustPlus(authToken, expoPushToken)
	if err != nil {
		return nil, fmt.Errorf("failed to register with rustplus: %w", err)
	}

	fmt.Println("Successfully registered with Rust Companion API")

	return &RegistrationResult{
		SteamID:        steamID,
		FCMCredentials: *fcmCredentials,
		ExpoPushToken:  expoPushToken,
		AuthToken:      updatedAuthToken,
		Success:        true,
	}, nil
}


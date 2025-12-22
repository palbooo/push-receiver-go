package register

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// JWTPayload represents the JWT payload structure from Rust Companion API
type JWTPayload struct {
	SteamID string `json:"steamId"`
	Version int    `json:"version"`
	Iss     int64  `json:"iss"`
	Exp     int64  `json:"exp"`
}

// ExtractSteamIDFromJWT extracts the Steam ID from a JWT auth token.
// The auth token is a JWT format token containing the steamId in the payload.
//
// Example:
//
//	authToken := "eyJzdGVhbUlkIjoiNzY1NjExOTg4ODA3MTI3MjMi..."
//	steamID, err := register.ExtractSteamIDFromJWT(authToken)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Steam ID: %s\n", steamID)
func ExtractSteamIDFromJWT(authToken string) (string, error) {
	// URL-decode the token first
	decodedToken, err := url.QueryUnescape(authToken)
	if err != nil {
		return "", fmt.Errorf("failed to URL-decode token: %w", err)
	}

	// JWT format: header.payload.signature
	parts := strings.Split(decodedToken, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid JWT format: expected at least 2 parts, got %d", len(parts))
	}

	// Decode the payload (first part is the header, second is payload)
	payload := parts[0]

	// Add padding if necessary for base64 decoding
	if padding := len(payload) % 4; padding > 0 {
		payload += strings.Repeat("=", 4-padding)
	}

	// Decode base64
	payloadBytes, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 payload: %w", err)
	}

	// Parse JSON
	var jwtPayload JWTPayload
	if err := json.Unmarshal(payloadBytes, &jwtPayload); err != nil {
		return "", fmt.Errorf("failed to parse JWT payload: %w", err)
	}

	if jwtPayload.SteamID == "" {
		return "", fmt.Errorf("steamId not found in JWT payload")
	}

	return jwtPayload.SteamID, nil
}

// ParseJWT parses a JWT auth token and returns the complete payload.
// This is useful if you need access to additional fields like version, iss, or exp.
//
// Example:
//
//	payload, err := register.ParseJWT(authToken)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Steam ID: %s\n", payload.SteamID)
//	fmt.Printf("Version: %d\n", payload.Version)
//	fmt.Printf("Expires: %d\n", payload.Exp)
func ParseJWT(authToken string) (*JWTPayload, error) {
	// URL-decode the token first
	decodedToken, err := url.QueryUnescape(authToken)
	if err != nil {
		return nil, fmt.Errorf("failed to URL-decode token: %w", err)
	}

	// JWT format: header.payload.signature
	parts := strings.Split(decodedToken, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid JWT format: expected at least 2 parts, got %d", len(parts))
	}

	// Decode the payload (first part)
	payload := parts[0]

	// Add padding if necessary for base64 decoding
	if padding := len(payload) % 4; padding > 0 {
		payload += strings.Repeat("=", 4-padding)
	}

	// Decode base64
	payloadBytes, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 payload: %w", err)
	}

	// Parse JSON
	var jwtPayload JWTPayload
	if err := json.Unmarshal(payloadBytes, &jwtPayload); err != nil {
		return nil, fmt.Errorf("failed to parse JWT payload: %w", err)
	}

	return &jwtPayload, nil
}


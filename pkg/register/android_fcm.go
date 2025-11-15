package register

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/palbooo/push-receiver-go/internal/gcm"
	"github.com/palbooo/push-receiver-go/internal/utils"
)

// AndroidFCM handles Android FCM registration
type AndroidFCM struct {
	config *Config
}

// NewAndroidFCM creates a new AndroidFCM instance
func NewAndroidFCM(config *Config) *AndroidFCM {
	return &AndroidFCM{
		config: config,
	}
}

// Register performs the complete FCM registration flow
func (a *AndroidFCM) Register() (*FCMCredentials, error) {
	// Step 1: Create Firebase installation
	installationAuthToken, err := a.installRequest()
	if err != nil {
		return nil, fmt.Errorf("installation request failed: %w", err)
	}

	// Wait for Firebase token to propagate (prevents PHONE_REGISTRATION_ERROR)
	fmt.Println("Waiting for Firebase token to activate...")
	time.Sleep(2 * time.Second)

	// Step 2: Check-in with GCM
	checkInResponse, err := gcm.CheckIn("", "")
	if err != nil {
		return nil, fmt.Errorf("gcm check-in failed: %w", err)
	}

	androidID := fmt.Sprintf("%d", *checkInResponse.AndroidId)
	securityToken := fmt.Sprintf("%d", *checkInResponse.SecurityToken)

	// Step 3: Register with GCM to get FCM token
	fcmToken, err := a.registerRequest(androidID, securityToken, installationAuthToken, 0)
	if err != nil {
		return nil, fmt.Errorf("fcm registration failed: %w", err)
	}

	return &FCMCredentials{
		GCM: GCMCredentials{
			AndroidID:     androidID,
			SecurityToken: securityToken,
		},
		FCM: FCMTokenCredentials{
			Token: fcmToken,
		},
	}, nil
}

func (a *AndroidFCM) installRequest() (string, error) {
	fid := generateFirebaseFID()

	requestBody := map[string]interface{}{
		"fid":         fid,
		"appId":       a.config.FCM.GMSAppID,
		"authVersion": "FIS_v2",
		"sdkVersion":  "a:17.0.0",
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	url := fmt.Sprintf("https://firebaseinstallations.googleapis.com/v1/projects/%s/installations",
		a.config.FCM.ProjectID)

	responseBody, err := utils.SimpleRequest(utils.RequestOptions{
		URL:    url,
		Method: "POST",
		Headers: map[string]string{
			"Accept":                     "application/json",
			"Content-Type":               "application/json",
			"X-Android-Package":          a.config.FCM.AndroidPackageName,
			"X-Android-Cert":             a.config.FCM.AndroidPackageCert,
			"x-firebase-client":          "android-min-sdk/23 fire-core/20.0.0 device-name/a21snnxx device-brand/samsung device-model/a21s android-installer/com.android.vending fire-android/30 fire-installations/17.0.0 fire-fcm/22.0.0 android-platform/ kotlin/1.9.23 android-target-sdk/34",
			"x-firebase-client-log-type": "3",
			"x-goog-api-key":             a.config.FCM.APIKey,
			"User-Agent":                 "Dalvik/2.1.0 (Linux; U; Android 11; SM-A217F Build/RP1A.200720.012)",
		},
		Body: bodyBytes,
	})
	if err != nil {
		return "", fmt.Errorf("installation request failed: %w", err)
	}

	var response InstallationResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if response.AuthToken.Token == "" {
		return "", fmt.Errorf("failed to get Firebase installation AuthToken")
	}

	return response.AuthToken.Token, nil
}

func (a *AndroidFCM) registerRequest(androidID, securityToken, installationAuthToken string, retryCount int) (string, error) {
	form := map[string]string{
		"device":                             androidID,
		"app":                                a.config.FCM.AndroidPackageName,
		"cert":                               a.config.FCM.AndroidPackageCert,
		"app_ver":                            "1",
		"X-subtype":                          a.config.FCM.GCMSenderID,
		"X-app_ver":                          "1",
		"X-osv":                              "29",
		"X-cliv":                             "fiid-21.1.1",
		"X-gmsv":                             "220217001",
		"X-scope":                            "*",
		"X-Goog-Firebase-Installations-Auth": installationAuthToken,
		"X-gms_app_id":                       a.config.FCM.GMSAppID,
		"X-Firebase-Client":                  "android-min-sdk/23 fire-core/20.0.0 device-name/a21snnxx device-brand/samsung device-model/a21s android-installer/com.android.vending fire-android/30 fire-installations/17.0.0 fire-fcm/22.0.0 android-platform/ kotlin/1.9.23 android-target-sdk/34",
		"X-Firebase-Client-Log-Type":         "1",
		"X-app_ver_name":                     "1",
		"target_ver":                         "31",
		"sender":                             a.config.FCM.GCMSenderID,
	}

	responseBody, err := utils.SimpleRequest(utils.RequestOptions{
		URL:    "https://android.clients.google.com/c2dm/register3",
		Method: "POST",
		Headers: map[string]string{
			"Authorization": fmt.Sprintf("AidLogin %s:%s", androidID, securityToken),
			"Content-Type":  "application/x-www-form-urlencoded",
		},
		Form: form,
	})
	if err != nil {
		return "", fmt.Errorf("register request failed: %w", err)
	}

	response := string(responseBody)
	if strings.Contains(response, "Error") {
		if retryCount >= 5 {
			return "", fmt.Errorf("GCM register failed after retries: %s", response)
		}

		// Exponential backoff: 2s, 4s, 6s, 8s, 10s
		waitTime := time.Duration(2*(retryCount+1)) * time.Second
		fmt.Printf("Register request failed with %s, retrying in %v... (attempt %d)\n", response, waitTime, retryCount+1)
		time.Sleep(waitTime)
		return a.registerRequest(androidID, securityToken, installationAuthToken, retryCount+1)
	}

	// Extract token from response (format: "token=<TOKEN>")
	parts := strings.Split(response, "=")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid register response format: %s", response)
	}

	return parts[1], nil
}

// generateFirebaseFID generates a Firebase Installation ID
// Based on: https://github.com/firebase/firebase-js-sdk/blob/master/packages/installations/src/helpers/generate-fid.ts
func generateFirebaseFID() string {
	buf := make([]byte, 17)
	rand.Read(buf)

	// Replace the first 4 bits with the constant FID header of 0b0111
	buf[0] = 0b01110000 | (buf[0] & 0b00001111)

	// Encode to base64 and remove padding
	return strings.TrimRight(utils.ToBase64(buf), "=")
}


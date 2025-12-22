package register

import "encoding/json"

// GCMCredentials contains GCM authentication credentials
type GCMCredentials struct {
	AndroidID     string `json:"androidId"`
	SecurityToken string `json:"securityToken"`
}

// FCMTokenCredentials contains FCM token information
type FCMTokenCredentials struct {
	Token string `json:"token"`
}

// FCMCredentials contains both GCM and FCM credentials
type FCMCredentials struct {
	GCM GCMCredentials      `json:"gcm"`
	FCM FCMTokenCredentials `json:"fcm"`
}

// RegistrationResult contains the complete FCM registration result
type RegistrationResult struct {
	SteamID        string         `json:"steamId"`
	FCMCredentials FCMCredentials `json:"fcmCredentials"`
	ExpoPushToken  string         `json:"expoPushToken"`
	AuthToken      string         `json:"authToken"`
	Success        bool           `json:"success"`
}

// ToJSON converts the RegistrationResult to a JSON string
func (r *RegistrationResult) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// ToJSONIndent converts the RegistrationResult to a pretty-printed JSON string
func (r *RegistrationResult) ToJSONIndent() (string, error) {
	jsonBytes, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// CheckInResponse represents the GCM check-in response
type CheckInResponse struct {
	AndroidID     string `json:"androidId"`
	SecurityToken string `json:"securityToken"`
}

// InstallationResponse represents the Firebase installation response
type InstallationResponse struct {
	Name      string    `json:"name"`
	FID       string    `json:"fid"`
	AuthToken AuthToken `json:"authToken"`
}

// AuthToken represents the authentication token from Firebase
type AuthToken struct {
	Token     string `json:"token"`
	ExpiresIn string `json:"expiresIn"`
}

// ExpoPushTokenRequest represents the request to get an Expo push token
type ExpoPushTokenRequest struct {
	Type        string `json:"type"`
	DeviceID    string `json:"deviceId"`
	Development bool   `json:"development"`
	AppID       string `json:"appId"`
	DeviceToken string `json:"deviceToken"`
	ProjectID   string `json:"projectId"`
}

// ExpoPushTokenResponse represents the response containing the Expo push token
type ExpoPushTokenResponse struct {
	Data struct {
		ExpoPushToken string `json:"expoPushToken"`
	} `json:"data"`
}

// RustPlusRequest represents the request to register with RustPlus API
type RustPlusRequest struct {
	AuthToken string `json:"AuthToken"`
	DeviceID  string `json:"DeviceId"`
	PushKind  int    `json:"PushKind"`
	PushToken string `json:"PushToken"`
}

// RustPlusResponse represents the response from RustPlus API registration
type RustPlusResponse struct {
	AuthToken string `json:"token"`
}


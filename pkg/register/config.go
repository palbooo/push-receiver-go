package register

// FCMConfig contains Firebase Cloud Messaging configuration
type FCMConfig struct {
	APIKey             string
	ProjectID          string
	GCMSenderID        string
	GMSAppID           string
	AndroidPackageName string
	AndroidPackageCert string
}

// Config holds all application configuration
type Config struct {
	FCM              FCMConfig
	RustPlusAPIURL   string
	ExpoPushTokenURL string

	// DeviceID identifies this application to the Rust Companion API.
	// Facepunch stores exactly one push token per (steamId, deviceId): two
	// applications registering with the same DeviceID overwrite each other's
	// registration, and the loser silently stops receiving notifications.
	// There is deliberately no default — pick a value unique to your
	// application (e.g. "my-bot.example.com").
	DeviceID string
}

// DefaultConfig returns the default configuration for Rust+ Companion.
// DeviceID is left empty and must be set before calling RegisterRustPlus.
func DefaultConfig() *Config {
	return &Config{
		FCM: FCMConfig{
			APIKey:             "AIzaSyB5y2y-Tzqb4-I4Qnlsh_9naYv_TD8pCvY",
			ProjectID:          "rust-companion-app",
			GCMSenderID:        "976529667804",
			GMSAppID:           "1:976529667804:android:d6f1ddeb4403b338fea619",
			AndroidPackageName: "com.facepunch.rust.companion",
			AndroidPackageCert: "E28D05345FB78A7A1A63D70F4A302DBF426CA5AD",
		},
		RustPlusAPIURL:   "https://companion-rust.facepunch.com:443/api/push/register",
		ExpoPushTokenURL: "https://exp.host/--/api/v2/push/getExpoPushToken",
	}
}

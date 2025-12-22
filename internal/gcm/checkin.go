package gcm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/palbooo/push-receiver-go/internal/utils"
	pb "github.com/palbooo/push-receiver-go/proto"
	protobuf "google.golang.org/protobuf/proto"
)

const (
	checkinURL  = "https://android.clients.google.com/checkin"
	registerURL = "https://android.clients.google.com/c2dm/register3"
)

// CheckIn performs a GCM check-in to get androidId and securityToken
func CheckIn(androidID, securityToken string) (*pb.AndroidCheckinResponse, error) {
	buffer, err := getCheckinRequest(androidID, securityToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkin request: %w", err)
	}

	body, err := utils.SimpleRequest(utils.RequestOptions{
		URL:    checkinURL,
		Method: "POST",
		Headers: map[string]string{
			"Content-Type": "application/x-protobuf",
		},
		Body: buffer,
	})
	if err != nil {
		return nil, fmt.Errorf("checkin request failed: %w", err)
	}

	response := &pb.AndroidCheckinResponse{}
	if err := protobuf.Unmarshal(body, response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkin response: %w", err)
	}

	return response, nil
}

// Register performs FCM registration with the given credentials
func Register(androidID, securityToken, appID string) (string, error) {
	serverKey := utils.ToBase64(ServerKey)

	form := map[string]string{
		"app":       "org.chromium.linux",
		"X-subtype": appID,
		"device":    androidID,
		"sender":    serverKey,
	}

	response, err := postRegister(androidID, securityToken, form, 0)
	if err != nil {
		return "", err
	}

	// Extract token from response (format: "token=<TOKEN>")
	parts := strings.Split(response, "=")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid register response format: %s", response)
	}

	return parts[1], nil
}

func postRegister(androidID, securityToken string, form map[string]string, retryCount int) (string, error) {
	body, err := utils.SimpleRequest(utils.RequestOptions{
		URL:    registerURL,
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

	response := string(body)
	if strings.Contains(response, "Error") {
		if retryCount >= 5 {
			return "", fmt.Errorf("GCM register failed after retries: %s", response)
		}

		fmt.Printf("Register request failed with %s, retrying... %d\n", response, retryCount+1)
		return postRegister(androidID, securityToken, form, retryCount+1)
	}

	return response, nil
}

func getCheckinRequest(androidID, securityToken string) ([]byte, error) {
	var androidIDVal *int64
	var securityTokenVal *uint64

	if androidID != "" {
		id, err := strconv.ParseInt(androidID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid androidId: %w", err)
		}
		androidIDVal = &id
	}

	if securityToken != "" {
		token, err := strconv.ParseUint(securityToken, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid securityToken: %w", err)
		}
		securityTokenVal = &token
	}

	checkinType := pb.DeviceType_DEVICE_CHROME_BROWSER
	platform := pb.ChromeBuildProto_PLATFORM_LINUX
	chromeVersion := "63.0.3234.0"
	channel := pb.ChromeBuildProto_CHANNEL_STABLE
	version := int32(3)
	userSerialNumber := int32(0)

	request := &pb.AndroidCheckinRequest{
		UserSerialNumber: &userSerialNumber,
		Checkin: &pb.AndroidCheckinProto{
			Type: checkinType.Enum(),
			ChromeBuild: &pb.ChromeBuildProto{
				Platform:      platform.Enum(),
				ChromeVersion: &chromeVersion,
				Channel:       channel.Enum(),
			},
		},
		Version:       &version,
		Id:            androidIDVal,
		SecurityToken: securityTokenVal,
	}

	return protobuf.Marshal(request)
}

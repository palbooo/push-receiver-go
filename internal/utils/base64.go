package utils

import "encoding/base64"

// ToBase64 encodes bytes to base64 string
func ToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// FromBase64 decodes base64 string to bytes
func FromBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}


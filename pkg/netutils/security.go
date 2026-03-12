// Package netutils provides security and networking utilities.
package netutils

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
)

// VerifyPSK verifies that the provided pre-shared key matches the expected one.
func VerifyPSK(provided, expected string) bool {
	return hmac.Equal([]byte(provided), []byte(expected))
}

// HashPSK creates a SHA256 hash of the PSK for logging/storage purposes.
// Note: Never log or store actual PSKs - only hashes!
func HashPSK(psk string) string {
	hash := sha256.Sum256([]byte(psk))
	return fmt.Sprintf("%x", hash)[:16] // First 16 chars for display
}

// ValidatePSKStrength checks if the PSK meets minimum security requirements.
func ValidatePSKStrength(psk string) error {
	if len(psk) < 8 {
		return fmt.Errorf("PSK must be at least 8 characters long")
	}
	
	hasUpper := false
	hasLower := false
	hasDigit := false
	
	for _, c := range psk {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit {
		return fmt.Errorf("PSK should contain uppercase, lowercase, and digits")
	}

	return nil
}

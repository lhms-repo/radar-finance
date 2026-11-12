package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// PINIterations is how many PBKDF2 rounds a new PIN hash uses; the stored hash keeps its own count, so
// raising it later still checks old PINs. Tests may lower it to stay fast.
var PINIterations = 600_000

// pinPrefix names the hash format stored in users.pin_hash.
const pinPrefix = "pbkdf2-sha256"

// ValidPIN reports whether the PIN has 4 to 6 digits and nothing else (RN-27).
func ValidPIN(pin string) bool {
	if len(pin) < 4 || len(pin) > 6 {
		return false
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// HashPIN returns the stored form of a PIN: format, rounds, salt and key, so the PIN itself is never kept (RN-27).
func HashPIN(pin string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pin, salt, PINIterations, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", pinPrefix, PINIterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// CheckPIN reports whether the PIN matches the stored hash, comparing in constant time.
func CheckPIN(stored, pin string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != pinPrefix {
		return false
	}
	rounds, err := strconv.Atoi(parts[1])
	if err != nil || rounds < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := enc.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pin, salt, rounds, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

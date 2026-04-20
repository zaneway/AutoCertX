package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const passwordAlgoVersion = 1

var errInvalidPasswordHash = errors.New("invalid password hash")

// PasswordHasher hashes and verifies password credentials.
type PasswordHasher struct {
	Iterations int
	SaltBytes  int
	KeyBytes   int
}

// DefaultPasswordHasher returns the PBKDF2 parameters used by T03.
func DefaultPasswordHasher() PasswordHasher {
	return PasswordHasher{
		Iterations: 120_000,
		SaltBytes:  16,
		KeyBytes:   32,
	}
}

// Hash derives a salted password hash and returns the encoded value plus its version.
func (h PasswordHasher) Hash(password string) (string, int, error) {
	if password == "" {
		return "", 0, errInvalidPasswordHash
	}

	salt := make([]byte, h.SaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", 0, fmt.Errorf("read salt: %w", err)
	}

	key := pbkdf2SHA256([]byte(password), salt, h.Iterations, h.KeyBytes)
	encoded := strings.Join([]string{
		"pbkdf2-sha256",
		strconv.Itoa(passwordAlgoVersion),
		strconv.Itoa(h.Iterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}, "$")

	return encoded, passwordAlgoVersion, nil
}

// Verify checks a cleartext password against the encoded hash.
func (h PasswordHasher) Verify(encoded string, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "pbkdf2-sha256" {
		return false, errInvalidPasswordHash
	}
	if parts[1] != strconv.Itoa(passwordAlgoVersion) {
		return false, errInvalidPasswordHash
	}

	iterations, err := strconv.Atoi(parts[2])
	if err != nil || iterations <= 0 {
		return false, errInvalidPasswordHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, errInvalidPasswordHash
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errInvalidPasswordHash
	}

	actual := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return false, nil
	}

	return true, nil
}

func pbkdf2SHA256(password []byte, salt []byte, iterations int, keyLen int) []byte {
	hLen := 32
	blocks := (keyLen + hLen - 1) / hLen
	output := make([]byte, 0, blocks*hLen)
	buffer := make([]byte, len(salt)+4)
	copy(buffer, salt)

	for block := 1; block <= blocks; block++ {
		buffer[len(salt)] = byte(block >> 24)
		buffer[len(salt)+1] = byte(block >> 16)
		buffer[len(salt)+2] = byte(block >> 8)
		buffer[len(salt)+3] = byte(block)

		u := hmacSHA256(password, buffer)
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iterations; i++ {
			u = hmacSHA256(password, u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		output = append(output, t...)
	}

	return output[:keyLen]
}

func hmacSHA256(key []byte, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

package security

import (
	"crypto/rand"
	"encoding/base64"
)

// GenerateInviteCode generates a cryptographically random conference invitation code.
//
// Returns:
//   - result 1: Cryptographically random invitation code.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func GenerateInviteCode() (string, error) {
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(random[:]), nil
}

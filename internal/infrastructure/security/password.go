package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory  = 64 * 1024
	argonTime    = 3
	argonThreads = 4
	saltLength   = 16
	keyLength    = 32
)

// PasswordHasher hashes and verifies passwords without storing plaintext credentials.
type PasswordHasher struct{}

// Hash hashes the password for safe credential storage.
//
// Parameters:
//   - password: Plaintext password accepted only for validation or hashing.
//
// Returns:
//   - result 1: Protected password hash or content digest, according to this component.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, keyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version,
		argonMemory, argonTime, argonThreads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify compares a plaintext password with the stored encoded hash using the bounded supported
// hash parameters.
//
// Parameters:
//   - password: Plaintext password accepted only for validation or hashing.
//   - encoded: Stored encoded password hash or token value.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (PasswordHasher) Verify(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// Fixed parameters and version checks prevent a corrupted hash from causing excessive memory
	// allocation.
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=4" {
		return false, fmt.Errorf("invalid password hash format")
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != saltLength {
		return false, fmt.Errorf("invalid password salt")
	}
	want, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(want) != keyLength {
		return false, fmt.Errorf("invalid password hash")
	}
	got := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, keyLength)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

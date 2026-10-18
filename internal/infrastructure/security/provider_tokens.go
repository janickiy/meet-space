package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
)

// ProviderTokens encrypts private values with AES-256-GCM; authenticated context prevents moving
// a token between accounts or devices.
type ProviderTokens struct{ aead cipher.AEAD }

// NewProviderTokens constructs the provider tokens with the dependencies and limits required for
// authentication and cryptographic bindings.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//
// Returns:
//   - result 1: The constructed or loaded provider tokens.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewProviderTokens(key string) (*ProviderTokens, error) {
	if key == "" {
		return nil, nil
	}
	b, err := base64.StdEncoding.Strict().DecodeString(key)
	if err != nil || len(b) != 32 {
		return nil, errors.New("provider encryption key must contain 32 base64-encoded bytes")
	}
	block, err := aes.NewCipher(b)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &ProviderTokens{aead: aead}, nil
}

// Encrypt encrypts a private provider value with a random nonce and authenticated owner binding.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//   - binding: Verified account, session and media-connection identity.
//
// Returns:
//   - result 1: Authenticated encrypted provider value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *ProviderTokens) Encrypt(value, binding string) (string, error) {
	if p == nil {
		return "", errors.New("provider encryption disabled")
	}
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := p.aead.Seal(nonce, nonce, []byte(value), []byte(binding))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt authenticates the stored ciphertext and owner binding before returning the private
// value.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//   - binding: Verified account, session and media-connection identity.
//
// Returns:
//   - result 1: Authenticated plaintext provider value for server-side use only.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *ProviderTokens) Decrypt(value, binding string) (string, error) {
	if p == nil {
		return "", errors.New("provider encryption disabled")
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(b) < p.aead.NonceSize() {
		return "", errors.New("invalid encrypted provider token")
	}
	n := p.aead.NonceSize()
	out, err := p.aead.Open(nil, b[:n], b[n:], []byte(binding))
	if err != nil {
		return "", errors.New("invalid encrypted provider token")
	}
	return string(out), nil
}

package security

import (
	"crypto/rand"
	"encoding/base64"
)

// GenerateInviteCode генерирует криптографически случайный код приглашения на конференцию.
//
// @returns:
//   - result1: Криптографически случайный код приглашения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func GenerateInviteCode() (string, error) {
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(random[:]), nil
}

// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
)

// ProviderTokens шифрует приватные значения с помощью AES-256-GCM; аутентифицированный контекст
// предотвращает перемещение токена между учетными записями или устройствами.
type ProviderTokens struct{ aead cipher.AEAD }

// NewProviderTokens создает токены провайдера с зависимостями и ограничениями, необходимыми для
// аутентификации и криптографических привязок.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//
// @returns:
//   - result1: Созданные или загруженные токены провайдера.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// Encrypt шифрует приватное значение провайдера с помощью случайного одноразового номера и
// аутентифицированной привязки владельца.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//   - binding: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
//
// @returns:
//   - result1: Аутентифицированное зашифрованное значение провайдера.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// Decrypt аутентифицирует сохраненный зашифрованный текст и привязку владельца, после чего
// возвращает приватное значение.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//   - binding: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
//
// @returns:
//   - result1: Аутентифицированное значение провайдера в виде открытого текста только для
//     использования на стороне сервера.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

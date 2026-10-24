// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package s3

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
)

// personalAvatarKey создает ключ для приватных объектов, принадлежащих серверу, для аватара
// личной группы.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func personalAvatarKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) == 4 && parts[0] == "avatars" && parts[1] == "conversations" && attachmentID(parts[2]) && attachmentID(parts[3])
}

// PutPersonalAvatar сохраняет нормализованные байты аватара под ключом для приватных объектов,
// принадлежащих серверу, для группового аватара.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//   - reader: Поток исходного контента или устройство чтения карт памяти, в зависимости от его
//     типа.
//   - size: Размер контента в байтах или запрошенная ограниченная емкость.
//   - contentType: Объявленный или обнаруженный тип контента MIME.
//   - checksum: Ожидаемый дайджест содержимого, используемый для проверки целостности передачи.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) PutPersonalAvatar(ctx context.Context, key string, reader io.Reader, size int64, contentType, checksum string) error {
	if !personalAvatarKey(key) || size < 1 || size > 2<<20 || (contentType != "image/jpeg" && contentType != "image/png") || len(checksum) != 64 {
		return fmt.Errorf("invalid personal avatar object")
	}
	_, err := c.minio.PutObject(ctx, c.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{"sha256": checksum}})
	return err
}

// OpenPersonalAsset никогда не создает публичный URL.  Вызывающая сторона владеет потоком
// чтения и закрывает его; Stat принудительно отправляет отложенный запрос MinIO до отправки
// каких-либо заголовков HTTP.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//
// @returns:
//   - result1: Результирующий считывающий модуль.
//   - result2: Возвращает количество, размер или числовое значение.
//   - result3: Обнаружен тип содержимого ресурса.
//   - result4: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) OpenPersonalAsset(ctx context.Context, key string) (io.ReadCloser, int64, string, error) {
	if !personalAvatarKey(key) && !attachmentKey(key) {
		return nil, 0, "", fmt.Errorf("invalid personal asset key")
	}
	object, err := c.minio.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, "", err
	}
	info, err := object.Stat()
	if err != nil {
		_ = object.Close()
		return nil, 0, "", err
	}
	return object, info.Size, info.ContentType, nil
}

// DeletePersonalAvatar удаляет только приватный объект, идентифицируемый по устаревшему ключу
// аватара.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) DeletePersonalAvatar(ctx context.Context, key string) error {
	if !personalAvatarKey(key) {
		return fmt.Errorf("invalid personal avatar key")
	}
	return c.minio.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{})
}

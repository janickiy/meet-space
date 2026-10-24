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
	"mime"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// CheckAttachmentPrivacy отклоняет настройки объектного хранилища, которые раскрывают приватные
// вложения чатов через публичную политику bucket.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) CheckAttachmentPrivacy(ctx context.Context) error {
	policy, err := c.minio.GetBucketPolicy(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("check private attachment bucket: %w", err)
	}
	if strings.TrimSpace(policy) != "" {
		return fmt.Errorf("attachments require a private bucket without a bucket policy")
	}
	return nil
}

// PutAttachment сохраняет байты вложений под сгенерированным сервером приватным ключом объекта.
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
func (c *Client) PutAttachment(ctx context.Context, key string, reader io.Reader, size int64, contentType, checksum string) error {
	if !attachmentKey(key) {
		return fmt.Errorf("invalid attachment key")
	}
	_, err := c.minio.PutObject(ctx, c.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{"sha256": checksum}})
	return err
}

// StatAttachment считывает фактический размер объекта и метаданные перед завершением загрузки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: Фактический тип хранимого содержимого объекта.
//   - result3: Сохраненный дайджест содержимого SHA-256, используемый для завершения
//     загрузки.
//   - result4: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) StatAttachment(ctx context.Context, key string) (int64, string, string, error) {
	if !attachmentKey(key) {
		return 0, "", "", fmt.Errorf("invalid attachment key")
	}
	object, err := c.minio.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return 0, "", "", err
	}
	return object.Size, object.ContentType, object.UserMetadata["Sha256"], nil
}

// AttachmentDownloadURL создает недолговечный подписанный URL, который запрашивает загрузку
// вложения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//   - filename: Безопасное имя файла для загрузки, указанное в заголовке Content-Disposition.
//   - expiry: Срок действия или интервал действительности, определяемый типом.
//
// @returns:
//   - result1: Недолговечный подписанный URL для загрузки вложения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) AttachmentDownloadURL(ctx context.Context, key, filename string, expiry time.Duration) (string, error) {
	if !attachmentKey(key) || expiry <= 0 || expiry > 5*time.Minute {
		return "", fmt.Errorf("invalid attachment download parameters")
	}
	client := c.minio
	if c.presignMinio != nil {
		client = c.presignMinio
	}
	params := url.Values{}
	params.Set("response-content-disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	params.Set("response-content-type", "application/octet-stream")
	params.Set("response-cache-control", "private, no-store")
	objectURL, err := client.PresignedGetObject(ctx, c.bucket, key, expiry, params)
	if err != nil {
		return "", err
	}
	return objectURL.String(), nil
}

// CleanAttachmentObjects удаляет только те объекты с префиксом вложения, которые не являются
// сохраненным финализированным объектом.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - prefix: Объект или префикс ключа, управляемый сервером.
//   - keep: Точный ключ финализированного объекта, который должен быть сохранен при очистке.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) CleanAttachmentObjects(ctx context.Context, prefix, keep string) error {
	parts := attachmentParts(strings.TrimSuffix(prefix, "/"))
	if len(parts) != 3 || parts[0] != "attachments" || !attachmentID(parts[1]) || !attachmentID(parts[2]) || !strings.HasSuffix(prefix, "/") || (keep != "" && (!attachmentKey(keep) || !strings.HasPrefix(keep, prefix))) {
		return fmt.Errorf("invalid attachment cleanup prefix")
	}
	// Синхронный итератор завершает работу вместе с вызвавшим его объектом.  Ошибка удаления не
	// должна приводить к блокировке производителя SDK на заполненном канале.
	for object := range c.minio.ListObjectsIter(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if object.Err != nil {
			return object.Err
		}
		if object.Key == keep {
			continue
		}
		if err := c.minio.RemoveObject(ctx, c.bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// attachmentID проверяет идентификатор вложения перед формированием пути к объекту.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func attachmentID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

// attachmentKey формирует принадлежащий серверу приватный ключ объекта для вложения.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func attachmentKey(key string) bool {
	parts := attachmentParts(key)
	return len(parts) == 4 && parts[0] == "attachments" && attachmentID(parts[1]) && attachmentID(parts[2]) && attachmentID(parts[3])
}

// attachmentParts проверяет и разбивает ключ объекта вложения на ожидаемые компоненты пути.
//
// Прямые файлы имеют отдельный префикс; для обеих областей по-прежнему требуется канонический
// UUIDs.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//
// @returns:
//   - result1: Собранные строковые значения.
func attachmentParts(key string) []string {
	parts := strings.Split(key, "/")
	if len(parts) > 1 && parts[1] == "direct" {
		parts = append(parts[:1], parts[2:]...)
	}
	return parts
}

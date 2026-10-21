// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package recorder

import (
	"context"
	"errors"
	"fmt"

	"github.com/janickiy/meet-space/internal/domain/records"
)

// ingestFailureRepository определяет заменяемый контракт зависимостей для записи заданий и их
// артефактов.
type ingestFailureRepository interface {
	// FindByUUID загружает запись по ее публичному идентификатору UUID.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//
	// @returns:
	//   - result1: Результирующая запись.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	FindByUUID(context.Context, string) (records.Record, error)
	// MarkFailed автоматически переводит запись в состояние сбоя, не перезаписывая новый
	// переход жизненного цикла.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//   - arg3: Основная ошибка, из-за которой операция завершилась неудачно.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	MarkFailed(context.Context, string, error) error
	// AddEvent добавляет событие диагностики жизненного цикла постоянной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//   - arg3: Ограниченная категория событий жизненного цикла записи.
	//   - arg4: Медиа или источник контента, связанный с текущей операцией.
	//   - arg5: Ограниченная степень серьезности диагностики.
	//   - arg6: Сохраненное или отправленное сообщение в чате.
	//   - arg7: Идентификатор работника, которому принадлежит эта операция.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	AddEvent(context.Context, string, string, string, string, string, string) error
}

// conferenceReleaser определяет заменяемый контракт зависимостей для записи заданий и их
// артефактов.
type conferenceReleaser interface {
	// Release — при освобождении ресурса записи освобождается только тот ресурс, который принадлежит
	// соответствующей операции или токену.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Внешняя запись UUID.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Release(context.Context, string, string) error
}

// FailIngest регистрирует сбой при приеме данных и освобождает только собственные ресурсы и
// блокировку записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - repository: Зависимость персистентности, используемая операцией приложения.
//   - locker: Блокировка распределенной записи конференции с учетом прав собственности.
//   - recordID: Внешняя запись UUID.
//   - workerID: Идентификатор работника, которому принадлежит эта операция.
//   - cause: Основная ошибка, из-за которой операция завершилась неудачно.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func FailIngest(ctx context.Context, repository ingestFailureRepository, locker conferenceReleaser, recordID, workerID string, cause error) error {
	return failRecord(ctx, repository, locker, recordID, workerID, "record.ingest.failed", cause)
}

// failRecord регистрирует сбой записи только в том случае, если состояние ее жизненного цикла
// не изменилось.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - repository: Зависимость персистентности, используемая операцией приложения.
//   - locker: Блокировка распределенной записи конференции с учетом прав собственности.
//   - recordID: Внешняя запись UUID.
//   - workerID: Идентификатор работника, которому принадлежит эта операция.
//   - eventType: Ограниченная категория событий жизненного цикла записи.
//   - cause: Основная ошибка, из-за которой операция завершилась неудачно.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func failRecord(ctx context.Context, repository ingestFailureRepository, locker conferenceReleaser, recordID, workerID, eventType string, cause error) error {
	record, err := repository.FindByUUID(ctx, recordID)
	if err != nil {
		return fmt.Errorf("find failed record: %w", err)
	}
	if records.IsTerminalStatus(record.Status) {
		return releaseRecordLock(ctx, locker, record)
	}
	if err := repository.MarkFailed(ctx, recordID, cause); err != nil {
		return fmt.Errorf("mark ingest failed: %w", err)
	}
	releaseErr := releaseRecordLock(ctx, locker, record)
	message := "WebRTC ingest failed"
	if cause != nil {
		message = cause.Error()
	}
	eventErr := repository.AddEvent(ctx, recordID, eventType, "worker", "error", message, workerID)
	return errors.Join(releaseErr, eventErr)
}

// releaseRecordLock снимает распределенную блокировку только в том случае, если она принадлежит
// этой записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - locker: Блокировка распределенной записи конференции с учетом прав собственности.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func releaseRecordLock(ctx context.Context, locker conferenceReleaser, record records.Record) error {
	if locker == nil {
		return nil
	}
	return locker.Release(ctx, record.ConferenceID, record.UUID)
}

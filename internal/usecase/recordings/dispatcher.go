// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package recordings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
)

// CommandRepository резервирует команды, уже зафиксированные в состоянии записи.
type CommandRepository interface {
	// ClaimCommand запрашивает одну команду исходящего пакета транзакций для записи вместе с
	// текущим состоянием записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: Результирующая команда из исходящего буфера.
	//   - result2: Результирующая запись.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ClaimCommand(context.Context) (records.OutboxCommand, records.Record, error)
	// CompleteCommand подтверждает команду исходящего пакета транзакций для записи только в том
	// случае, если запрос на запись актуален.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Значение команды из исходящего буфера, требуемое по контракту.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CompleteCommand(context.Context, records.OutboxCommand) error
	// RetryCommand повторно отправляет запрос на запись с использованием политики ограниченных
	// повторных попыток.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Значение команды из исходящего буфера, требуемое по контракту.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RetryCommand(context.Context, records.OutboxCommand) error
}

type Commander interface {
	// StartRecord публикует команду начала записи, которая подготавливает прием медиаданных.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//   - arg3: Запрошенная продолжительность сегмента в секундах.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	StartRecord(context.Context, string, int) error
	// StopRecord публикует команду остановки записи, которая запускает финализацию и публикацию
	// артефакта.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//   - arg3: Ограниченный жизненный цикл или причина сбоя.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	StopRecord(context.Context, string, string) error
}

// Locker — блокировщик резервирует запись перед ее доставкой.  Захват данных осуществляется работником.
type Locker interface {
	// Атомарная операция Acquire устанавливает блокировку записи конференции, владельцем
	// которой является указанная запись.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Внешняя запись UUID.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Acquire(context.Context, string, string) (bool, error)
}

// CommandDispatcher отвечает за опрос исходящих сообщений и упорядочивание их доставки,
// независимо от операций чтения/авторизации HTTP.  Вызывающая сторона отвечает за отмену и
// работу с общими клиентами.
type CommandDispatcher struct {
	repo     CommandRepository
	commands Commander
	locker   Locker
	events   Events
}

// NewCommandDispatcher создает диспетчер команд с зависимостями и ограничениями, необходимыми
// для управления жизненным циклом записи конференции.
//
// @Parameters:
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - commands: Передача команд записи.
//   - locker: Блокировка распределенной записи конференции с учетом прав собственности.
//   - events: Место назначения для зафиксированных событий жизненного цикла или чатов.
//
// @returns:
//   - result1: Созданный или загруженный диспетчер команд.
func NewCommandDispatcher(repo CommandRepository, commands Commander, locker Locker, events Events) *CommandDispatcher {
	return &CommandDispatcher{repo: repo, commands: commands, locker: locker, events: events}
}

// Run управляет жизненным циклом записи конференции до тех пор, пока контекст не будет отменен
// или пока не произойдет сбой при обработке.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (d *CommandDispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for range 16 {
			if err := d.dispatchOne(ctx); err != nil {
				break
			}
		}
	}
}

// dispatchOne выполняет аренду, публикацию, завершение и повторные попытки в рамках одной
// ограниченной операции.  Терминальная запись завершает только свою команду: повторная отправка
// не производится, что позволяет сохранить существующий контракт на восстановление и изоляцию.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (d *CommandDispatcher) dispatchOne(ctx context.Context) error {
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command, record, err := d.repo.ClaimCommand(op)
	if err != nil {
		return err
	}
	if records.IsTerminalStatus(record.Status) {
		return d.repo.CompleteCommand(op, command)
	}
	err = d.deliver(op, command, record)
	if err == nil {
		err = d.repo.CompleteCommand(op, command)
	}
	if err != nil {
		_ = d.repo.RetryCommand(op, command)
		if !errors.Is(err, context.Canceled) {
			slog.Warn("recording command retry", "recording_id", record.UUID, "command", command.CommandType)
		}
	}
	return err
}

// deliver проверяет текущие права доступа получателя непосредственно перед доставкой
// сохраненного события или команды.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (d *CommandDispatcher) deliver(ctx context.Context, command records.OutboxCommand, record records.Record) error {
	if command.CommandType != "record.start" {
		if err := d.commands.StopRecord(ctx, record.UUID, command.Reason); err != nil {
			return err
		}
		publish(d.events, ctx, record, "recording.stopping")
		return nil
	}
	if d.locker != nil {
		acquired, err := d.locker.Acquire(ctx, record.ConferenceID, record.UUID)
		if err != nil {
			return err
		}
		if !acquired {
			return apperrors.ErrConflict
		}
	}
	if err := d.commands.StartRecord(ctx, record.UUID, record.SegmentDurationSec); err != nil {
		return err
	}
	if record.Status == records.StatusStarting {
		publish(d.events, ctx, record, "recording.starting")
	}
	return nil
}

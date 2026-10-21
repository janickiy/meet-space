// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package ffmpeg

import (
	"context"
	"github.com/janickiy/meet-space/internal/operations"
	"os/exec"
	"syscall"
	"time"
)

// runBounded запускает внешний процесс с возможностью отмены и ограничением вывода
// диагностических данных.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - binary: Путь к настроенному внешнему исполняемому файлу.
//   - args: Аргументы командной строки, процесса или протокола.
//
// @returns:
//   - result1: Собранные байтовые значения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func runBounded(ctx context.Context, binary string, args ...string) ([]byte, error) {
	started := time.Now()
	operations.FFmpegActive(1)
	defer func() { operations.FFmpegActive(-1); operations.Observe("ffmpeg", time.Since(started).Seconds()) }()
	cmd := exec.CommandContext(ctx, binary, args...)
	output := &logTail{}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	err := cmd.Run()
	if err != nil {
		operations.Event("ffmpeg_failed")
	}
	return []byte(output.String()), err
}

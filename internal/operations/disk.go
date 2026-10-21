// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package operations

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

// ValidateStorage проверяет, доступен ли настроенный каталог для записи, перед началом
// обработки.
//
// @Parameters:
//   - path: Проверенный путь к локальной файловой системе или объекту.
//   - minimum: Минимально необходимый объем свободной памяти в байтах.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func ValidateStorage(path string, minimum uint64) error {
	if path == "" {
		return fmt.Errorf("STORAGE_PATH must not be empty")
	}
	if err := os.MkdirAll(path, 0750); err != nil {
		return fmt.Errorf("storage directory unavailable")
	}
	file, err := os.CreateTemp(path, ".readiness-")
	if err != nil {
		return fmt.Errorf("storage is not writable")
	}
	_ = file.Close()
	_ = os.Remove(file.Name())
	return DiskCheck(path, minimum)(context.Background())
}

// DiskCheck создает проверку готовности для настроенного резервирования свободного места в
// памяти.
//
// @Parameters:
//   - path: Проверенный путь к локальной файловой системе или объекту.
//   - minimum: Минимально необходимый объем свободной памяти в байтах.
//
// @returns:
//   - result1: Результат проверки.
func DiskCheck(path string, minimum uint64) Check {
	return func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var stat unix.Statfs_t
		if err := unix.Statfs(path, &stat); err != nil {
			return fmt.Errorf("storage stat failed")
		}
		free := uint64(stat.Bavail) * uint64(stat.Bsize)
		State("disk_free_bytes", float64(free))
		if free < minimum {
			Event("disk_low")
			return fmt.Errorf("storage free space below reserve")
		}
		return nil
	}
}

// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package local

import (
	"fmt"
	"os"
	"path/filepath"
)

// RemoveEmptyTrees удаляет пустые каталоги под проверенным корневым каталогом локальных
// записей.
//
// @Parameters:
//   - paths: Проверенные локальные пути, задействованные в этой операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func RemoveEmptyTrees(paths ...string) error {
	for _, path := range paths {
		if err := removeEmptyTree(path); err != nil {
			return err
		}
	}

	return nil
}

// removeEmptyTree просматривает локальное поддерево и удаляет каталоги только после того, как
// их содержимое становится пустым.
//
// @Parameters:
//   - path: Проверенный путь к локальной файловой системе или объекту.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func removeEmptyTree(path string) error {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read local storage dir %s: %w", path, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := removeEmptyTree(filepath.Join(path, entry.Name())); err != nil {
			return err
		}
	}
	entries, err = os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read local storage dir %s: %w", path, err)
	}
	if len(entries) > 0 {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove empty local storage dir %s: %w", path, err)
	}

	return nil
}

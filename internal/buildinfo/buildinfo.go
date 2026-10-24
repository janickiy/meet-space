// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Пакет buildinfo предоставляет только проверенные общедоступные метаданные артефакта.
package buildinfo

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
	"time"
)

// Version — версия, коммит и BuildTime указываются во время сборки с помощью -ldflags -X.
var Version, Commit, BuildTime string

// Info содержит информацию о версии релиза, коммите исходного кода и времени сборки, а не о
// значениях переменных среды или путях к файлам в файловой системе.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

// ValidVersion проверяет, можно ли безопасно указывать версию сборки в общедоступных метаданных.
//
// @Parameters:
//   - value: значение для проверки, нормализации или преобразования.
//
// Возвращаемое значение:
//   - result1: выполняется ли задокументированное условие операции или ее условное состояние
//     изменено успешно.
func ValidVersion(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for index, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || index > 0 && (char == '.' || char == '_' || char == '-' || char == '+') {
			continue
		}
		return false
	}
	return true
}

// Возвращает проверенные метаданные сборки без значений окружения или локальных путей.
//
// @returns:
//   - result1: итоговая информация.
func Current() Info {
	i := Info{Version: "dev", Commit: "unknown", BuildTime: "unknown"}
	if ValidVersion(Version) {
		i.Version = Version
	}
	commit := Commit
	if info, ok := debug.ReadBuildInfo(); ok && commit == "" {
		for _, value := range info.Settings {
			if value.Key == "vcs.revision" {
				commit = value.Value
			}
		}
	}
	valid := len(commit) == 40 || len(commit) == 64
	for _, char := range commit {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			valid = false
		}
	}
	if valid {
		i.Commit = commit
	}
	if stamp, err := time.Parse(time.RFC3339, BuildTime); err == nil {
		i.BuildTime = stamp.UTC().Format(time.RFC3339)
	}
	return i
}

// Handler предоставляет проверенные общедоступные метаданные сборки без секретных данных,
// флагов компилятора или внутренних адресов.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - r: Входящий запрос HTTP.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(Current())
}

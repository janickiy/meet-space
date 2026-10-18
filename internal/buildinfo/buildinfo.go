// Package buildinfo exposes only validated public artifact metadata.
package buildinfo

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
	"time"
)

// Version, Commit and BuildTime are injected at build time with -ldflags -X.
var Version, Commit, BuildTime string

// Info contains the release version, source commit and build time, never environment values or
// filesystem paths.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

// ValidVersion проверяет, можно ли безопасно указывать версию сборки в общедоступных метаданных.
//
// Параметры:
//   - value: значение для проверки, нормализации или преобразования.
//
// Возвращаемое значение:
//   - result 1: выполняется ли задокументированное условие операции или ее условное состояние
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
// Возвращает:
//   - результат 1: итоговая информация.
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

// Handler serves validated public build metadata without secrets, compiler flags or internal
// addresses.
//
// Parameters:
//   - w: Destination HTTP response writer.
//   - r: Incoming HTTP request.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(Current())
}

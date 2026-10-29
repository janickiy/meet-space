// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Платформа пакета определяет безопасные возможности продукта и сводные операционные данные
// только для чтения.
package platform

import "time"

// Capabilities описывает функции сервера без указания адресов провайдеров, токенов или
// секретных данных.
type Capabilities struct {
	RecordingModes []string `json:"recordingModes"`
}

// Failure — это ограниченное операционное событие без идентификаторов пользователя или
// конференции, а также без пользовательского контента.
type Failure struct {
	Kind string    `json:"kind"`
	Code string    `json:"code"`
	At   time.Time `json:"at"`
}

// Summary содержит сводный моментальный снимок для администратора с глобальными правами.
type Summary struct {
	AsOf                time.Time       `json:"asOf"`
	ActiveConferences   int64           `json:"activeConferences"`
	JoinedParticipants  int64           `json:"joinedParticipants"`
	ActiveRecordings    int64           `json:"activeRecordings"`
	QueuedJobs          int64           `json:"queuedJobs"`
	FailedJobs24h       int64           `json:"failedJobs24h"`
	FailedRecordings24h int64           `json:"failedRecordings24h"`
	APIReady            bool            `json:"apiReady"`
	MediaWorkerReady    bool            `json:"mediaWorkerReady"`
	Dependencies        map[string]bool `json:"dependencies"`
	RecentFailures      []Failure       `json:"recentFailures"`
}

// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package records

import "errors"

// ErrConferenceAlreadyRecording означает, что на конференции уже активна блокировка начала
// записи.
var ErrConferenceAlreadyRecording = errors.New("recording already started for conferenceId")

// ErrRecordStateChanged указывает на устаревшее обновление состояния или недопустимый порядок
// перехода между этапами жизненного цикла.
var ErrRecordStateChanged = errors.New("record status no longer allows this transition")

// ErrNoMedia означает, что попытка загрузки была прервана до создания медиафайла.  Оркестрация
// записи распознаёт этот результат независимо от адаптера загрузки.
var ErrNoMedia = errors.New("record media was not produced")

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

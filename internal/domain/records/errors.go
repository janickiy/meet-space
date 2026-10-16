package records

import "errors"

// ErrConferenceAlreadyRecording означает, что для conferenceId уже действует блокировка запуска записи.
var ErrConferenceAlreadyRecording = errors.New("recording already started for conferenceId")

// ErrRecordStateChanged обозначает устаревшее обновление состояния или нарушение порядка переходов.
var ErrRecordStateChanged = errors.New("record status no longer allows this transition")

// ErrNoMedia means an ingest attempt stopped before producing media. Recording
// orchestration recognizes this outcome independently of the ingest adapter.
var ErrNoMedia = errors.New("record media was not produced")

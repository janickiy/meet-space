package records

import "errors"

// ErrConferenceAlreadyRecording means the conference already has an active recording-start lock.
var ErrConferenceAlreadyRecording = errors.New("recording already started for conferenceId")

// ErrRecordStateChanged identifies a stale state update or invalid lifecycle transition order.
var ErrRecordStateChanged = errors.New("record status no longer allows this transition")

// ErrNoMedia means an ingest attempt stopped before producing media. Recording
// orchestration recognizes this outcome independently of the ingest adapter.
var ErrNoMedia = errors.New("record media was not produced")

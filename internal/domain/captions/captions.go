// Package captions defines a live-transcription contract independent of the media engine and
// provider SDK.
package captions

import (
	"context"
	"errors"
	"time"
)

var ErrUnavailable = errors.New("live_provider_unavailable")

// SessionConfig describes one connection's track without biometric identification. Audio is
// signed PCM16LE, mono, 16000 Hz; event times are relative to the first PCM sample.
type SessionConfig struct {
	SessionID       string `json:"sessionId"`
	TrackInstanceID string `json:"trackInstanceId"`
	Language        string `json:"language"`
	SampleRate      int    `json:"sampleRate"`
	Channels        int    `json:"channels"`
	Format          string `json:"format"`
}

// Event carries a provider utterance revision; a final result cannot subsequently be replaced by
// a partial result.
type Event struct {
	UtteranceID string `json:"utteranceId"`
	Sequence    int64  `json:"sequence"`
	Revision    int64  `json:"revision"`
	StartMS     int64  `json:"startMs"`
	EndMS       int64  `json:"endMs"`
	Text        string `json:"text"`
	Language    string `json:"language"`
	Final       bool   `json:"final"`
}

// LiveTranscriptionProvider opens a streaming session while its adapter hides provider SDK
// details.
type LiveTranscriptionProvider interface {
	// StartSession opens one bounded live-speech provider session for the authorized audio stream.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Operator-controlled configuration and processing limits.
	//
	// Returns:
	//   - result 1: The resulting session.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	StartSession(context.Context, SessionConfig) (Session, error)
}

// Session owns one external connection and a bounded result stream.
type Session interface {
	// WriteAudio forwards one bounded audio frame to the active speech session.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: The byte value required by the contract.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	WriteAudio(context.Context, []byte) error
	// Events returns the live event stream or serves its authorized transport adapter.
	//
	// Returns:
	//   - result 1: The bounded event or media channel owned by this operation.
	Events() <-chan Event
	// Close closes the component-owned resources and ends their lifecycle.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Close() error
}

// Caption binds text to the server-verified speaker identity and conference timeline.
type Caption struct {
	Cursor          int64  `json:"cursor"`
	Generation      int64  `json:"generation"`
	ID              string `json:"id"`
	ConferenceID    string `json:"conferenceId"`
	SessionID       string `json:"sessionId"`
	ParticipantID   string `json:"participantId"`
	Speaker         string `json:"speaker"`
	TrackInstanceID string `json:"trackInstanceId"`
	Event
	CreatedAt time.Time `json:"createdAt"`
}

// State tells admitted participants whether conference audio is being sent for transcription.
type State struct {
	ConferenceID         string    `json:"conferenceId"`
	SessionID            string    `json:"sessionId"`
	Enabled              bool      `json:"enabled"`
	Available            bool      `json:"available"`
	CanManage            bool      `json:"canManage"`
	Language             string    `json:"language"`
	Status               string    `json:"status"`
	Generation           int64     `json:"generation"`
	Origin               time.Time `json:"origin"`
	CanonicalRecordingID *string   `json:"canonicalRecordingId,omitempty"`
}

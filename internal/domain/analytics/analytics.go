// Package analytics defines technical conference aggregates without evaluating identity or
// productivity.
package analytics

import "time"

// Interval contains observed presence; overlapping connections belonging to one participant are
// merged.
type Interval struct {
	ParticipantID string
	Start, End    time.Time
}

// Point records the number of simultaneously present participants on the conference timeline.
type Point struct {
	AtMS  int64 `json:"atMs"`
	Count int   `json:"count"`
}

// Participant contains objective activity counters and approximate audio activity.
type Participant struct {
	ParticipantID   string `json:"participantId"`
	DisplayName     string `json:"displayName"`
	ParticipationMS int64  `json:"participationMs"`
	SpeakingMS      int64  `json:"speakingMs"`
	ObservedAudioMS int64  `json:"observedAudioMs"`
	ScreenMS        int64  `json:"screenMs"`
	MessageCount    int64  `json:"messageCount"`
}

// Conference contains aggregates that explicitly label speech activity as approximate.
type Conference struct {
	ConferenceID        string        `json:"conferenceId"`
	Enabled             bool          `json:"enabled"`
	DurationMS          int64         `json:"durationMs"`
	ParticipantCount    int           `json:"participantCount"`
	Timeline            []Point       `json:"timeline" gorm:"-"`
	Participants        []Participant `json:"participants" gorm:"-"`
	RecordingAvailable  bool          `json:"recordingAvailable"`
	TranscriptAvailable bool          `json:"transcriptAvailable"`
	ApproximateSpeaking bool          `json:"approximateSpeaking"`
	UpdatedAt           time.Time     `json:"updatedAt"`
}

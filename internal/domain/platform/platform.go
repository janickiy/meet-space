// Package platform defines safe product capabilities and read-only operational summaries.
package platform

import "time"

// Capabilities describes server features without provider addresses, tokens or secrets.
type Capabilities struct {
	LiveCaptions     bool     `json:"liveCaptions"`
	Transcription    bool     `json:"transcription"`
	AISummary        bool     `json:"aiSummary"`
	SemanticSearch   bool     `json:"semanticSearch"`
	MeetingAnalytics bool     `json:"meetingAnalytics"`
	RecordingModes   []string `json:"recordingModes"`
}

// Failure is a bounded operational event without user or conference identifiers or user content.
type Failure struct {
	Kind string    `json:"kind"`
	Code string    `json:"code"`
	At   time.Time `json:"at"`
}

// Summary contains an aggregate snapshot for a globally authorized administrator.
type Summary struct {
	AsOf                    time.Time       `json:"asOf"`
	ActiveConferences       int64           `json:"activeConferences"`
	JoinedParticipants      int64           `json:"joinedParticipants"`
	ActiveRecordings        int64           `json:"activeRecordings"`
	QueuedJobs              int64           `json:"queuedJobs"`
	FailedJobs24h           int64           `json:"failedJobs24h"`
	FailedRecordings24h     int64           `json:"failedRecordings24h"`
	FailedTranscriptions24h int64           `json:"failedTranscriptions24h"`
	APIReady                bool            `json:"apiReady"`
	MediaWorkerReady        bool            `json:"mediaWorkerReady"`
	Dependencies            map[string]bool `json:"dependencies"`
	RecentFailures          []Failure       `json:"recentFailures"`
}

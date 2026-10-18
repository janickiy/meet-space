package records

// Command holds the command data used by recording metadata.
type Command struct {
	Type               string `json:"type"`
	RecordID           string `json:"recordId"`
	Reason             string `json:"reason,omitempty"`
	SegmentDurationSec int    `json:"segmentDurationSec,omitempty"`
}

// WebRTCOfferRequest contains the client SDP offer used to establish recording ingestion.
type WebRTCOfferRequest struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// WebRTCAnswerResponse contains the recording receiver's negotiated SDP answer.
type WebRTCAnswerResponse struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// RecordDetails combines a recording with related files, segments and events for application
// reads.
type RecordDetails struct {
	Record   Record
	Files    []RecordFile
	Segments []RecordSegment
	Events   []RecordEvent
}

// RecordCard contains an authorized recording view with permitted links and related artifacts.
type RecordCard struct {
	Record
	Files    []RecordFileView    `json:"files"`
	Segments []RecordSegmentView `json:"segments"`
	Events   []RecordEvent       `json:"events,omitempty"`
}

// RecordFileView adds a temporary authorized read link to persisted file metadata.
type RecordFileView struct {
	RecordFile
	URL string `json:"url,omitempty"`
}

// RecordSegmentView adds the private bucket and temporary read link to segment metadata.
type RecordSegmentView struct {
	RecordSegment
	Bucket string `json:"bucket,omitempty"`
	URL    string `json:"url,omitempty"`
}

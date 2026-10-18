package media

// EgressRequest describes an internal recording-stream request protected by authorization and
// the media-room ownership lease.
type EgressRequest struct {
	Purpose            string `json:"purpose,omitempty"`
	RequestID          string `json:"requestId"`
	ConferenceID       string `json:"conferenceId"`
	RecordingID        string `json:"recordingId"`
	Route              Route  `json:"route"`
	SegmentDurationSec int    `json:"segmentDurationSec"`
}

// EgressTrack holds the egress track data used by authenticated media-room control.
type EgressTrack struct {
	Track
	MimeType    string `json:"mimeType"`
	ClockRate   uint32 `json:"clockRate"`
	Channels    uint16 `json:"channels"`
	PayloadType uint8  `json:"payloadType"`
	SSRC        uint32 `json:"ssrc"`
}

// EgressFrame carries an encoded RTP packet and its worker-clock capture timestamp.
type EgressFrame struct {
	Type       string       `json:"type"`
	Sequence   uint64       `json:"sequence"`
	CapturedAt int64        `json:"capturedAt"`
	Track      *EgressTrack `json:"track,omitempty"`
	TrackID    string       `json:"trackId,omitempty"`
	RTP        []byte       `json:"rtp,omitempty"`
	Code       string       `json:"code,omitempty"`
}

// EgressSubscription defines the replaceable dependency contract for authenticated media-room
// control.
type EgressSubscription interface {
	// Frames returns the subscription channel carrying encoded media frames.
	//
	// Returns:
	//   - result 1: The bounded event or media channel owned by this operation.
	Frames() <-chan EgressFrame
	// Done returns the channel closed when the owned process or subscription terminates.
	//
	// Returns:
	//   - result 1: The bounded event or media channel owned by this operation.
	Done() <-chan struct{}
	// Err returns the terminal subscription error, if one was recorded.
	//
	// Returns:
	//   - result 1: The owned process or subscription's terminal error, or nil after normal
	//     completion.
	Err() error
	// Close closes the component-owned resources and ends their lifecycle.
	Close()
	// Keyframes requests a video keyframe from the recording subscription.
	Keyframes()
	// Ping refreshes the liveness of the current recording subscription.
	Ping()
}

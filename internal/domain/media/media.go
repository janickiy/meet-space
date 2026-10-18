package media

import (
	"encoding/json"
	"time"

	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

// Media-signaling limits are independent of configurable realtime relay limits. Forty receive or
// publish slots can exceed 48 KiB in a valid Firefox offer.
const MaxSDPBytes = 128 << 10

// JSON escaping can expand SDP; internal commands and responses remain bounded.
const MaxCommandBytes = 1 << 20
const MaxICEBytes = 4096

// Binding links a media peer to a verified WebSocket session. Browser payloads cannot supply this identity.
type Binding struct {
	ConferenceID           string    `json:"conferenceId"`
	ParticipantID          string    `json:"participantId"`
	SessionID              string    `json:"sessionId"`
	ConnectionID           string    `json:"connectionId"`
	UserID                 string    `json:"userId"`
	AuthorizationExpiresAt time.Time `json:"authorizationExpiresAt"`
}

// Route contains the media-room owner's address and fencing version for authenticated internal
// calls.
type Route struct {
	WorkerID string `json:"workerId"`
	Endpoint string `json:"endpoint"`
	LeaseID  string `json:"leaseId"`
}

// Worker describes bounded media-worker presence in the distributed registry.
type Worker struct {
	Draining bool   `json:"draining,omitempty"`
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
}

// PeerView is the public representation of a physical media connection.
type PeerView struct {
	MediaPeerID   string `json:"mediaPeerId"`
	ConferenceID  string `json:"conferenceId"`
	ParticipantID string `json:"participantId"`
	SessionID     string `json:"sessionId"`
	ConnectionID  string `json:"connectionId"`
}

type Kind string
type Source string

const (
	KindAudio         Kind   = "audio"
	KindVideo         Kind   = "video"
	SourceMicrophone  Source = "microphone"
	SourceCamera      Source = "camera"
	SourceVideoScreen Source = "video/screen"
	SourceAudioScreen Source = "audio/screen"
)

// Track describes a published media track and its source.
type Track struct {
	ID            string `json:"id"`
	StreamID      string `json:"streamId"`
	MediaPeerID   string `json:"mediaPeerId"`
	ParticipantID string `json:"participantId"`
	Kind          Kind   `json:"kind"`
	Source        Source `json:"source"`
}

// Publication maps SDP MID to a source. TrackID identifies a browser capture generation, not a server track.
type Publication struct {
	MID     string `json:"mid"`
	Source  Source `json:"source"`
	TrackID string `json:"trackId,omitempty"`
}

// ParticipantPolicy contains persisted media-source permissions and a monotonic moderation
// version.
type ParticipantPolicy struct {
	Version           int64 `json:"version"`
	MicrophoneBlocked bool  `json:"microphoneBlocked"`
	CameraBlocked     bool  `json:"cameraBlocked"`
	ScreenBlocked     bool  `json:"screenBlocked"`
	Kicked            bool  `json:"kicked"`
}

// Allows reports whether the current policy permits the requested operation or source.
//
// Parameters:
//   - source: Media or content source belonging to the current operation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (p ParticipantPolicy) Allows(source Source) bool {
	if p.Kicked {
		return false
	}
	switch source {
	case SourceMicrophone:
		return !p.MicrophoneBlocked
	case SourceCamera:
		return !p.CameraBlocked
	case SourceVideoScreen:
		return !p.ScreenBlocked && !p.CameraBlocked
	case SourceAudioScreen:
		return !p.ScreenBlocked && !p.MicrophoneBlocked && !p.CameraBlocked
	default:
		return false
	}
}

// SourceKind normalizes a media source label to the supported microphone, camera or screen kind.
//
// Parameters:
//   - source: Media or content source belonging to the current operation.
//
// Returns:
//   - result 1: The resulting kind.
func SourceKind(source Source) Kind {
	switch source {
	case SourceMicrophone, SourceAudioScreen:
		return KindAudio
	case SourceCamera, SourceVideoScreen:
		return KindVideo
	default:
		return ""
	}
}

// Command holds the command data used by authenticated media-room control.
type Command struct {
	RequestID     string             `json:"requestId"`
	Binding       Binding            `json:"binding"`
	Route         Route              `json:"route"`
	MediaPeerID   string             `json:"mediaPeerId,omitempty"`
	NegotiationID string             `json:"negotiationId,omitempty"`
	SDP           string             `json:"sdp,omitempty"`
	Candidate     json.RawMessage    `json:"candidate,omitempty"`
	TrackID       string             `json:"trackId,omitempty"`
	Ticket        string             `json:"ticket,omitempty"`
	Publications  []Publication      `json:"publications"`
	Policy        *ParticipantPolicy `json:"policy,omitempty"`
	ConferenceID  string             `json:"conferenceId,omitempty"`
	ParticipantID string             `json:"participantId,omitempty"`
}

// VideoCaptureTarget contains the capture constraints for the selected video quality profile.
type VideoCaptureTarget struct {
	MaxWidth     int `json:"maxWidth"`
	MaxHeight    int `json:"maxHeight"`
	MaxFrameRate int `json:"maxFrameRate"`
}

// Result contains the media operation outcome and its bounded component metadata.
type Result struct {
	MediaPeerID        string               `json:"mediaPeerId"`
	WorkerID           string               `json:"workerId,omitempty"`
	MaxPeers           int                  `json:"maxPeers,omitempty"`
	ICEServers         []realtime.ICEServer `json:"iceServers,omitempty"`
	ICETransportPolicy string               `json:"iceTransportPolicy,omitempty"`
	ICEExpiresAt       int64                `json:"iceExpiresAt,omitempty"`
	Tracks             []Track              `json:"tracks,omitempty"`
	NegotiationID      string               `json:"negotiationId,omitempty"`
	SDP                string               `json:"sdp,omitempty"`
	VideoCapture       VideoCaptureTarget   `json:"videoCapture"`
	Policy             *ParticipantPolicy   `json:"policy,omitempty"`
}

// Signal contains a bounded signaling payload; the server assigns its trusted identity.
type Signal struct {
	MediaPeerID   string          `json:"mediaPeerId,omitempty"`
	NegotiationID string          `json:"negotiationId,omitempty"`
	SDP           string          `json:"sdp,omitempty"`
	Candidate     json.RawMessage `json:"candidate,omitempty"`
	TrackID       string          `json:"trackId,omitempty"`
	Publications  []Publication   `json:"publications,omitempty"`
}

// PolicyForParticipant projects authoritative membership into versioned media
// permissions. Conference availability is enforced separately by its owner.
//
// Parameters:
//   - p: The participant state processed by this operation.
//
// Returns:
//   - result 1: The resulting participant policy.
func PolicyForParticipant(p conferences.Participant) ParticipantPolicy {
	return ParticipantPolicy{Version: p.MediaPolicyVersion, MicrophoneBlocked: p.MicrophoneBlocked, CameraBlocked: p.CameraBlocked, ScreenBlocked: p.ScreenBlocked, Kicked: !p.CanParticipate()}
}

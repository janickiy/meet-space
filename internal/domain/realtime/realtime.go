package realtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/conferences"
)

// Session stores one physical WebSocket connection. Participant membership survives its closure.
type Session struct {
	ID             string     `json:"id" gorm:"type:uuid;primaryKey"`
	ConferenceID   string     `json:"conferenceId" gorm:"type:uuid"`
	ParticipantID  string     `json:"participantId" gorm:"type:uuid"`
	UserID         string     `json:"userId" gorm:"type:uuid"`
	ConnectionID   string     `json:"connectionId" gorm:"type:uuid"`
	ClientID       string     `json:"clientId,omitempty" gorm:"-"`
	Status         string     `json:"status"`
	ConnectedAt    time.Time  `json:"connectedAt"`
	DisconnectedAt *time.Time `json:"disconnectedAt,omitempty"`
	LastSeenAt     time.Time  `json:"lastSeenAt"`
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (Session) TableName() string { return "participant_sessions" }

// Identity carries verified user identity, token expiry and an optional durable auth session.
type Identity struct {
	UserID        string    `json:"userId"`
	ExpiresAt     time.Time `json:"expiresAt"`
	AuthSessionID string    `json:"authSessionId,omitempty"`
	ClientID      string    `json:"clientId,omitempty"`
}

// Envelope is the versioned WebSocket event. ReplyTo correlates a response with its request.
type Envelope struct {
	Version      int             `json:"version"`
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	ConferenceID string          `json:"conferenceId"`
	Timestamp    time.Time       `json:"timestamp"`
	Data         json.RawMessage `json:"data"`
	ReplyTo      string          `json:"replyTo,omitempty"`
}

// Event creates a server envelope with a fresh ID and UTC timestamp.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - data: Input bytes or structured event payload.
//
// Returns:
//   - result 1: The resulting envelope.
func Event(kind, conferenceID string, data any) Envelope {
	raw, _ := json.Marshal(data)
	return Envelope{Version: 1, ID: uuid.NewString(), Type: kind, ConferenceID: conferenceID, Timestamp: time.Now().UTC(), Data: raw}
}

// Presence contains aggregate participant liveness and the count of active physical connections.
type Presence struct {
	conferences.ParticipantView
	Online        bool     `json:"online"`
	Connections   int      `json:"connections"`
	ConnectionIDs []string `json:"connectionIds"`
}

// State contains the initial conference and visible-participant snapshot for a room.
type State struct {
	ConnectionID  string             `json:"connectionId"`
	ParticipantID string             `json:"participantId"`
	Status        conferences.Status `json:"status"`
	Participants  []Presence         `json:"participants"`
}

// Signal contains a bounded signaling payload whose trusted identity is assigned by the server.
type Signal struct {
	TargetConnectionID  string          `json:"targetConnectionId"`
	SDP                 string          `json:"sdp,omitempty"`
	Candidate           json.RawMessage `json:"candidate,omitempty"`
	SenderConnectionID  string          `json:"senderConnectionId,omitempty"`
	SenderParticipantID string          `json:"senderParticipantId,omitempty"`
}

// ICEConfig contains the ICE servers needed to establish a WebRTC connection.
type ICEConfig struct {
	ICEServers         []ICEServer `json:"iceServers"`
	ICETransportPolicy string      `json:"iceTransportPolicy,omitempty"`
	ExpiresAt          int64       `json:"expiresAt,omitempty"`
}

// ICEServer contains one ICE service's endpoints and optional credentials.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// Bus carries a trusted event and its recipient routing between API instances.
type Bus struct {
	Kind          string    `json:"kind"`
	Session       *Session  `json:"session,omitempty"`
	ConferenceID  string    `json:"conferenceId"`
	ParticipantID string    `json:"participantId,omitempty"`
	ConnectionID  string    `json:"connectionId,omitempty"`
	Event         *Envelope `json:"event,omitempty"`
}

// Subscription receives inter-instance events until cancellation or Close.
type Subscription interface {
	// Receive waits for the next event using the caller's context.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The resulting bus.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Receive(context.Context) (Bus, error)
	// Close releases the subscription and unblocks Receive.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Close() error
}

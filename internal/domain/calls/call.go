package calls

import "time"

const RingTimeout = 45 * time.Second
const LeaseTimeout = 45 * time.Second

type Peer struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// Call is durable signalling state. Client IDs identify the tab owning media;
// they do not replace authentication or either account's membership.
type Call struct {
	ID                 string     `json:"id"`
	ConversationID     string     `json:"conversationId"`
	ConferenceID       *string    `json:"conferenceId"`
	CallerUserID       string     `json:"callerUserId"`
	CalleeUserID       string     `json:"calleeUserId"`
	CallerClientID     string     `json:"callerClientId"`
	AcceptedByClientID *string    `json:"acceptedByClientId"`
	ClientRequestID    string     `json:"-"`
	Status             string     `json:"status"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	ExpiresAt          time.Time  `json:"expiresAt"`
	AcceptedAt         *time.Time `json:"acceptedAt"`
	EndedAt            *time.Time `json:"endedAt"`
	CallerLeaseUntil   *time.Time `json:"-"`
	CalleeLeaseUntil   *time.Time `json:"-"`
	Caller             Peer       `json:"caller" gorm:"-"`
	Callee             Peer       `json:"callee" gorm:"-"`
	DurationSec        int64      `json:"durationSec" gorm:"-"`
}

func (Call) TableName() string { return "direct_calls" }
func (c Call) Active() bool    { return c.Status == "ringing" || c.Status == "accepted" }
func (c Call) IsParticipant(user string) bool {
	return user == c.CallerUserID || user == c.CalleeUserID
}
func (c Call) OwnsMedia(user, client string) bool {
	return client != "" && ((user == c.CallerUserID && client == c.CallerClientID) ||
		(user == c.CalleeUserID && c.AcceptedByClientID != nil && client == *c.AcceptedByClientID))
}
func (c Call) Expired(now time.Time) bool {
	if c.Status == "ringing" {
		return !now.Before(c.ExpiresAt)
	}
	return c.Status == "accepted" && ((c.CallerLeaseUntil != nil && !now.Before(*c.CallerLeaseUntil)) || (c.CalleeLeaseUntil != nil && !now.Before(*c.CalleeLeaseUntil)))
}
func (c *Call) SetDuration(now time.Time) {
	if c.AcceptedAt == nil {
		return
	}
	until := now
	if c.EndedAt != nil {
		until = *c.EndedAt
	}
	c.DurationSec = max(0, int64(until.Sub(*c.AcceptedAt)/time.Second))
}

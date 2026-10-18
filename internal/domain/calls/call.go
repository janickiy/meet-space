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

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (Call) TableName() string { return "direct_calls" }

// Active reports whether the direct call is still in a nonterminal state.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (c Call) Active() bool { return c.Status == "ringing" || c.Status == "accepted" }

// IsParticipant checks whether the account is one of the two participants in the direct call.
//
// Parameters:
//   - user: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (c Call) IsParticipant(user string) bool {
	return user == c.CallerUserID || user == c.CalleeUserID
}

// OwnsMedia checks whether the account and physical client own the active direct-call media
// connection.
//
// Parameters:
//   - user: Acting account identifier used to scope access.
//   - client: Browser media-client identity or transport dependency, as defined by its type.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (c Call) OwnsMedia(user, client string) bool {
	return client != "" && ((user == c.CallerUserID && client == c.CallerClientID) ||
		(user == c.CalleeUserID && c.AcceptedByClientID != nil && client == *c.AcceptedByClientID))
}

// Expired checks whether the direct call has passed its state-dependent expiration boundary.
//
// Parameters:
//   - now: Reference time used for expiration or duration calculations.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (c Call) Expired(now time.Time) bool {
	if c.Status == "ringing" {
		return !now.Before(c.ExpiresAt)
	}
	return c.Status == "accepted" && ((c.CallerLeaseUntil != nil && !now.Before(*c.CallerLeaseUntil)) || (c.CalleeLeaseUntil != nil && !now.Before(*c.CalleeLeaseUntil)))
}

// SetDuration computes the nonnegative accepted-call duration through its saved end time or the
// supplied current time.
//
// Parameters:
//   - now: Reference time used for expiration or duration calculations.
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

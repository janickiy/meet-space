package integrations

import (
	"context"
	"time"
)

// Preferences selects personal notification categories and external channels; email and push are
// disabled by default.
type Preferences struct {
	UserID     string    `json:"-" gorm:"type:uuid;primaryKey"`
	Invitation bool      `json:"invitation"`
	Reminder   bool      `json:"reminder"`
	Recording  bool      `json:"recording"`
	Summary    bool      `json:"summary"`
	Email      bool      `json:"email"`
	Push       bool      `json:"push"`
	UpdatedAt  time.Time `json:"-"`
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (Preferences) TableName() string { return "notification_preferences" }

// DefaultPreferences returns the initial notification preferences for an account.
//
// Parameters:
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting preferences.
func DefaultPreferences(userID string) Preferences {
	return Preferences{UserID: userID, Invitation: true, Reminder: true, Recording: true, Summary: true}
}

// Allows reports whether the current policy permits the requested operation or source.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (p Preferences) Allows(kind string) bool {
	switch kind {
	case "conference.invited", "conference.rescheduled", "conference.cancelled":
		return p.Invitation
	case "conference.soon":
		return p.Reminder
	case "recording.ready":
		return p.Recording
	case "transcript.ready", "summary.ready":
		return p.Summary
	case "admission.decided", "processing.failed":
		return true
	}
	return false
}

// Device persists a device registration; its encrypted token and fingerprint are never included
// in API responses.
type Device struct {
	ID               string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID           string     `json:"-"`
	Platform         string     `json:"platform"`
	Provider         string     `json:"provider"`
	TokenCiphertext  string     `json:"-"`
	TokenFingerprint string     `json:"-"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	RevokedAt        *time.Time `json:"revokedAt,omitempty"`
	DisabledAt       *time.Time `json:"disabledAt,omitempty"`
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (Device) TableName() string { return "push_devices" }

// DeviceRequest accepts a token from a trusted device application, never arbitrary provider
// URLs.
type DeviceRequest struct {
	Platform string `json:"platform"`
	Token    string `json:"token"`
}

// CalendarConnection binds an external calendar to its owner; OAuth secrets remain server-only.
type CalendarConnection struct {
	ID                string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID            string     `json:"-"`
	Provider          string     `json:"provider"`
	CalendarID        string     `json:"calendarId"`
	Status            string     `json:"status"`
	AccessCiphertext  string     `json:"-"`
	RefreshCiphertext string     `json:"-"`
	Scopes            string     `json:"-"`
	ExpiresAt         *time.Time `json:"expiresAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (CalendarConnection) TableName() string { return "calendar_connections" }

// CalendarMapping stores one external event for each conference and connection pair.
type CalendarMapping struct {
	ID                 string     `json:"-" gorm:"type:uuid;primaryKey"`
	ConferenceID       string     `json:"conferenceId"`
	ConnectionID       string     `json:"-"`
	Provider           string     `json:"provider"`
	ExternalCalendarID string     `json:"externalCalendarId"`
	ExternalEventID    string     `json:"externalEventId"`
	SyncStatus         string     `json:"syncStatus"`
	SourceVersion      int64      `json:"-"`
	LastSyncedAt       *time.Time `json:"lastSyncedAt,omitempty"`
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (CalendarMapping) TableName() string { return "calendar_event_mappings" }

// OAuthState persists short-lived one-use state and an encrypted PKCE verifier.
type OAuthState struct {
	ID                 string
	UserID             string
	Provider           string
	StateHash          string
	VerifierCiphertext string
	ExpiresAt          time.Time
	UsedAt             *time.Time
}

// TableName returns the explicit database table name so GORM does not infer a different plural
// form.
//
// Returns:
//   - result 1: Exact database table name used by GORM.
func (OAuthState) TableName() string { return "calendar_oauth_states" }

// EmailMessage contains minimal delivery data and an identifier stable across retries.
type EmailMessage struct {
	IdempotencyKey string `json:"idempotencyKey"`
	To             string `json:"to"`
	Subject        string `json:"subject"`
	HTML           string `json:"html"`
	Text           string `json:"text"`
}

// PushMessage contains a frontend link and safe event text, never conference transcripts.
type PushMessage struct {
	IdempotencyKey string `json:"idempotencyKey"`
	Token          string `json:"token"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	URL            string `json:"url"`
}

// CalendarEvent represents an external event without importing a provider-specific SDK.
type CalendarEvent struct {
	IdempotencyKey string    `json:"idempotencyKey"`
	ID             string    `json:"id,omitempty"`
	CalendarID     string    `json:"calendarId"`
	Title          string    `json:"title"`
	JoinURL        string    `json:"joinUrl"`
	StartsAt       time.Time `json:"startsAt"`
	EndsAt         time.Time `json:"endsAt"`
	Attendees      []string  `json:"attendees,omitempty"`
	SourceVersion  int64     `json:"sourceVersion"`
}

// CalendarCredentials carries OAuth tokens only inside the server, never in public JSON or logs.
type CalendarCredentials struct {
	AccessToken  string     `json:"-"`
	RefreshToken string     `json:"-"`
	ExpiresAt    *time.Time `json:"-"`
	Scopes       string     `json:"-"`
}

// EmailProvider isolates delivery behind a stable message key. The HTTP gateway deduplicates it;
// SMTP preserves Message-ID but may redeliver after a lost acknowledgement.
type EmailProvider interface {
	// Send delivers the bounded email with a stable message identity; the configured transport
	// determines retry deduplication.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Persisted or outgoing chat message.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Send(context.Context, EmailMessage) error
}

// PushProvider isolates platform delivery from provider-specific SDKs.
type PushProvider interface {
	// Send delivers a minimal notification to the selected registered device without transcript text
	// or provider secrets.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Persisted or outgoing chat message.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Send(context.Context, PushMessage) error
}

// CalendarProvider defines event creation, update, cancellation and lookup; CreateEvent must
// honor its idempotency key.
type CalendarProvider interface {
	// CreateEvent creates an idempotent calendar event through the configured provider.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: The calendar credentials value required by the contract.
	//   - argument 3: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: The resulting calendar event.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CreateEvent(context.Context, CalendarCredentials, CalendarEvent) (CalendarEvent, error)
	// UpdateEvent updates the mapped external calendar event for the current conference schedule.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: The calendar credentials value required by the contract.
	//   - argument 3: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: The resulting calendar event.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	UpdateEvent(context.Context, CalendarCredentials, CalendarEvent) (CalendarEvent, error)
	// CancelEvent cancels the external calendar event associated with the saved mapping.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: The calendar credentials value required by the contract.
	//   - argument 3: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CancelEvent(context.Context, CalendarCredentials, CalendarEvent) error
	// GetEvent reads the mapped external calendar event for reconciliation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: The calendar credentials value required by the contract.
	//   - argument 3: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: The resulting calendar event.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetEvent(context.Context, CalendarCredentials, CalendarEvent) (CalendarEvent, error)
}

// OAuthProvider performs server-side authorization, refresh and revocation using bounded
// operator-configured scopes.
type OAuthProvider interface {
	// AuthorizeURL builds the provider authorization redirect with one-time state and S256 PKCE
	// challenge, without exposing a client secret.
	//
	// Parameters:
	//   - argument 1: Current state or validated state update.
	//   - argument 2: S256 PKCE challenge sent to the authorization provider.
	//
	// Returns:
	//   - result 1: Provider authorization URL without a client secret.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AuthorizeURL(string, string) (string, error)
	// Exchange exchanges the one-time authorization code and PKCE verifier for provider credentials
	// on the server.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Bounded public status or failure code.
	//   - argument 3: Token verifier or private PKCE verifier, as defined by its type.
	//
	// Returns:
	//   - result 1: The resulting calendar credentials.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Exchange(context.Context, string, string) (CalendarCredentials, error)
	// Refresh renews the short-lived access credential using the current provider or account refresh
	// session.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Untrusted serialized value to decode or validate.
	//
	// Returns:
	//   - result 1: The resulting calendar credentials.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Refresh(context.Context, string) (CalendarCredentials, error)
	// Revoke revokes the stored provider authorization without changing the account's local delivery
	// preferences.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Signed credential or operation-ownership token to verify.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Revoke(context.Context, string) error
}

// Capabilities identifies actually configured adapters; a no-op is not reported as successful
// external delivery.
type Capabilities struct {
	Email                   string `json:"email"`
	Push                    string `json:"push"`
	Calendar                string `json:"calendar"`
	CalendarOAuthConfigured bool   `json:"calendarOAuthConfigured"`
	MockConnectAllowed      bool   `json:"mockConnectAllowed"`
}

// Providers groups replaceable adapters and available connection methods.
type Providers struct {
	Email        EmailProvider
	Push         PushProvider
	Calendar     CalendarProvider
	OAuth        OAuthProvider
	Capabilities Capabilities
}

package integrations

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	d "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	n "github.com/janickiy/meet-space/internal/domain/notifications"
)

// Options contains the public frontend URL, shared provider timeout and finite UTC reminder
// offsets.
type Options struct {
	PublicURL          string
	ReminderOffsets    []time.Duration
	ProviderTimeout    time.Duration
	MockConnectAllowed bool
}

// ConferenceSnapshot contains current fields safe for external synchronization, excluding media
// paths.
type ConferenceSnapshot struct {
	ID, OwnerID, Title, InviteCode, Status string
	ScheduledAt                            *time.Time
	PlannedDurationMin                     *int
	IntegrationVersion                     int64
}

// Delivery contains the permitted notification fact and recipient address, never transcript
// content.
type Delivery struct {
	Notification n.Notification
	Email        string
	Preferences  d.Preferences
	Allowed      bool
}

// Repository defines durable integration operations; asynchronous writes must check the job
// lease in their transaction.
type Repository interface {
	// Preferences reads the acting account's saved delivery or conversation preferences.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: The resulting preferences.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Preferences(context.Context, string) (d.Preferences, error)
	// SavePreferences validates and saves the account's delivery preferences without triggering a
	// delivery.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: The preferences value required by the contract.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SavePreferences(context.Context, d.Preferences) error
	// Devices returns the account's registered delivery devices without exposing their private
	// credentials.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Whether to return only non-revoked delivery devices.
	//
	// Returns:
	//   - result 1: The collected device values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Devices(context.Context, string, bool) ([]d.Device, error)
	// SaveDevice stores an account-owned delivery device with its protected provider token.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Account-owned delivery device and its protected provider metadata.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveDevice(context.Context, d.Device) error
	// RevokeDevice revokes only the acting account's selected delivery device.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RevokeDevice(context.Context, string, string) error
	// Connections returns the acting account's provider connections with private credentials
	// excluded from the public view.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: The collected calendar connection values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Connections(context.Context, string) ([]d.CalendarConnection, error)
	// SaveConnection persists a validated account-owned provider connection and encrypted
	// credentials.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Gin request and response context, including the authenticated identity.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveConnection(context.Context, d.CalendarConnection) error
	// RefreshConnection updates the provider credentials only for the expected connection state.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Gin request and response context, including the authenticated identity.
	//   - argument 3: Previous credential or generation identity used to reject a stale update.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RefreshConnection(context.Context, d.CalendarConnection, string) error
	// RevokeConnection revokes the account-owned provider connection and invalidates its future
	// synchronization work.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	RevokeConnection(context.Context, string, string) error
	// SaveOAuthState stores a short-lived one-use OAuth state bound to the account and provider.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Current state or validated state update.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveOAuthState(context.Context, d.OAuthState) error
	// TakeOAuthState atomically consumes the matching one-use OAuth state to prevent callback
	// replay.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Configured external-provider adapter.
	//   - argument 4: Previously computed digest used for identity or integrity lookup.
	//
	// Returns:
	//   - result 1: The resulting o auth state.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	TakeOAuthState(context.Context, string, string, string) (d.OAuthState, error)
	// CalendarMappings returns saved conference-to-calendar mappings for the acting account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The collected calendar mapping values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CalendarMappings(context.Context, string, string) ([]d.CalendarMapping, error)
	// Conference loads the persisted conference data required for integration delivery or
	// synchronization.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting conference snapshot.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Conference(context.Context, string) (ConferenceSnapshot, error)
	// MappingsForSync returns calendar mappings eligible for the current schedule generation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The collected calendar mapping values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	MappingsForSync(context.Context, string) ([]d.CalendarMapping, error)
	// SaveMapping stores the provider event mapping only while the job lease and schedule version
	// remain current.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//   - argument 3: Validated backup manifest or media state, as defined by its type.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveMapping(context.Context, jobs.Job, d.CalendarMapping) error
	// FailCalendars records the bounded synchronization failure for affected calendar mappings.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//   - argument 3: Bounded public status or failure code.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	FailCalendars(context.Context, jobs.Job, string) error
	// AcquireCalendar claims the current calendar synchronization lease for one worker.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The callback used to complete or release the operation.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	AcquireCalendar(context.Context, string) (func(), error)
	// Fanout creates deduplicated delivery jobs for recipients allowed by the saved preferences.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//   - argument 3: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Fanout(context.Context, jobs.Job, string) error
	// ScheduleReminders creates reminder work for conferences entering the configured reminder
	// window.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Configured reminder offsets relative to the scheduled conference time.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ScheduleReminders(context.Context, []time.Duration) error
	// Delivery loads the recipient-specific notification and delivery metadata for a leased job.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//
	// Returns:
	//   - result 1: The resulting delivery.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Delivery(context.Context, jobs.Job) (Delivery, error)
	// CompleteDelivery records the delivery outcome only for the current job lease.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//   - argument 3: Lifecycle state or response status to record.
	//   - argument 4: Bounded public status or failure code.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	CompleteDelivery(context.Context, jobs.Job, string, string) error
	// ScheduleCalendarSync queues deduplicated calendar work for the current conference schedule
	// version.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ScheduleCalendarSync(context.Context, string) error
}

// TokenCipher encrypts a secret and binds it to a specific account and connection.
type TokenCipher interface {
	// Encrypt encrypts a private provider value with a random nonce and authenticated owner binding.
	//
	// Parameters:
	//   - argument 1: Value to validate, normalize or convert.
	//   - argument 2: Verified account, session and media-connection identity.
	//
	// Returns:
	//   - result 1: Authenticated encrypted provider value.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Encrypt(string, string) (string, error)
	// Decrypt authenticates the stored ciphertext and owner binding before returning the private
	// value.
	//
	// Parameters:
	//   - argument 1: Value to validate, normalize or convert.
	//   - argument 2: Verified account, session and media-connection identity.
	//
	// Returns:
	//   - result 1: Authenticated plaintext provider value for server-side use only.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Decrypt(string, string) (string, error)
}

// Service connects persisted events to external adapters without making a conference or
// recording wait for a provider.
type Service struct {
	repo     Repository
	adapters d.Providers
	cipher   TokenCipher
	options  Options
	html     *template.Template
	text     *texttemplate.Template
}

// NewService constructs the service with the dependencies and limits required for account
// integrations and delivery preferences.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - adapters: Operator-configured external-provider adapters.
//   - cipher: Authenticated encryption dependency for private provider credentials.
//   - options: Validated component construction or execution options.
//
// Returns:
//   - result 1: The constructed or loaded service.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewService(repo Repository, adapters d.Providers, cipher TokenCipher, options Options) (*Service, error) {
	allNoop := adapters.Capabilities.Email == "noop" && adapters.Capabilities.Push == "noop" && adapters.Capabilities.Calendar == "noop"
	u, err := url.Parse(options.PublicURL)
	if !(allNoop && options.PublicURL == "") && (err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http")) {
		return nil, errors.New("PUBLIC_FRONTEND_URL must be an absolute HTTP(S) URL")
	}
	if options.ProviderTimeout <= 0 {
		options.ProviderTimeout = 30 * time.Second
	}
	if options.ProviderTimeout > 5*time.Minute {
		return nil, errors.New("integration timeout exceeds five minutes")
	}
	if len(options.ReminderOffsets) == 0 {
		options.ReminderOffsets = []time.Duration{24 * time.Hour, 15 * time.Minute}
	}
	if len(options.ReminderOffsets) > 8 {
		return nil, errors.New("too many reminder offsets")
	}
	seen := map[time.Duration]bool{}
	for _, v := range options.ReminderOffsets {
		if v < time.Second || v%time.Second != 0 || v > 30*24*time.Hour || seen[v] {
			return nil, errors.New("invalid reminder offsets")
		}
		seen[v] = true
	}
	h := template.Must(template.New("notification-v1").Parse(`<h1>{{.Subject}}</h1><p>{{.Title}}</p><p>{{.Body}}</p><p><a href="{{.URL}}">Открыть встречу</a></p>`))
	t := texttemplate.Must(texttemplate.New("notification-v1").Parse("{{.Subject}}\n{{.Title}}\n{{.Body}}\n{{.URL}}\n"))
	return &Service{repo: repo, adapters: adapters, cipher: cipher, options: options, html: h, text: t}, nil
}

// Capabilities returns the product or integration capabilities actually enabled by the operator.
//
// Returns:
//   - result 1: The resulting capabilities.
func (s *Service) Capabilities() d.Capabilities { return s.adapters.Capabilities }

// Preferences reads the acting account's saved delivery or conversation preferences.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting preferences.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Preferences(ctx context.Context, userID string) (d.Preferences, error) {
	return s.repo.Preferences(ctx, userID)
}

// SavePreferences validates and saves the account's delivery preferences without triggering a
// delivery.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) SavePreferences(ctx context.Context, userID string, value d.Preferences) error {
	value.UserID = userID
	return s.repo.SavePreferences(ctx, value)
}

// Devices returns the account's registered delivery devices without exposing their private
// credentials.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The collected device values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Devices(ctx context.Context, userID string) ([]d.Device, error) {
	return s.repo.Devices(ctx, userID, false)
}

// RegisterDevice registers the acting account's validated delivery device and protects its
// private token.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting device.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) RegisterDevice(ctx context.Context, userID string, request d.DeviceRequest) (d.Device, error) {
	if s.adapters.Capabilities.Push == "noop" || s.cipher == nil {
		return d.Device{}, apperrors.ErrUnavailable
	}
	if (request.Platform != "web" && request.Platform != "ios" && request.Platform != "android") || len(request.Token) < 8 || len(request.Token) > 8192 {
		return d.Device{}, apperrors.ErrInvalidInput
	}
	sum := sha256.Sum256([]byte(request.Token))
	fp := hex.EncodeToString(sum[:])
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(userID+":"+s.adapters.Capabilities.Push+":"+fp)).String()
	binding := userID + ":device:" + id
	encrypted, err := s.cipher.Encrypt(request.Token, binding)
	if err != nil {
		return d.Device{}, apperrors.ErrUnavailable
	}
	device := d.Device{ID: id, UserID: userID, Platform: request.Platform, Provider: s.adapters.Capabilities.Push, TokenCiphertext: encrypted, TokenFingerprint: fp}
	err = s.repo.SaveDevice(ctx, device)
	return device, err
}

// RevokeDevice revokes only the acting account's selected delivery device.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) RevokeDevice(ctx context.Context, userID, id string) error {
	return s.repo.RevokeDevice(ctx, userID, id)
}

// Connections returns the acting account's provider connections with private credentials
// excluded from the public view.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The collected calendar connection values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Connections(ctx context.Context, userID string) ([]d.CalendarConnection, error) {
	return s.repo.Connections(ctx, userID)
}

// MockConnect creates a mock calendar connection only when the configured environment explicitly
// permits it.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting calendar connection.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) MockConnect(ctx context.Context, userID string) (d.CalendarConnection, error) {
	if !s.options.MockConnectAllowed || !s.adapters.Capabilities.MockConnectAllowed {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	c := d.CalendarConnection{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(userID+":calendar:mock")).String(), UserID: userID, Provider: "mock", CalendarID: "primary", Status: "connected"}
	if err := s.repo.SaveConnection(ctx, c); err != nil {
		return c, err
	}
	return c, s.repo.ScheduleCalendarSync(ctx, userID)
}

// OAuthStart creates one-use OAuth state and an account-bound PKCE verifier before redirecting
// to the provider.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - provider: Operator-selected provider identifier or fixed technical provider label.
//
// Returns:
//   - result 1: Provider authorization URL with one-time state and PKCE challenge.
//   - result 2: One-use state value bound to the account and provider.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) OAuthStart(ctx context.Context, userID, provider string) (string, string, error) {
	if provider != "generic" || s.adapters.OAuth == nil || s.cipher == nil {
		return "", "", apperrors.ErrUnavailable
	}
	state, err := randomToken()
	if err != nil {
		return "", "", err
	}
	verifier, err := randomToken()
	if err != nil {
		return "", "", err
	}
	id := uuid.NewString()
	encrypted, err := s.cipher.Encrypt(verifier, userID+":oauth:"+id)
	if err != nil {
		return "", "", err
	}
	record := d.OAuthState{ID: id, UserID: userID, Provider: provider, StateHash: digest(state), VerifierCiphertext: encrypted, ExpiresAt: time.Now().UTC().Add(10 * time.Minute)}
	if err = s.repo.SaveOAuthState(ctx, record); err != nil {
		return "", "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	authURL, err := s.adapters.OAuth.AuthorizeURL(state, base64.RawURLEncoding.EncodeToString(challenge[:]))
	return authURL, state, err
}

// OAuthCallback consumes the matching account and provider state once before exchanging the
// authorization code.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - provider: Operator-selected provider identifier or fixed technical provider label.
//   - code: Bounded public status or failure code.
//   - state: Current state or validated state update.
//
// Returns:
//   - result 1: The resulting calendar connection.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) OAuthCallback(ctx context.Context, userID, provider, code, state string) (d.CalendarConnection, error) {
	if provider != "generic" || s.adapters.OAuth == nil || s.cipher == nil {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	if len(code) < 1 || len(code) > 4096 || len(state) != 43 {
		return d.CalendarConnection{}, apperrors.ErrInvalidInput
	}
	record, err := s.repo.TakeOAuthState(ctx, userID, provider, digest(state))
	if err != nil {
		return d.CalendarConnection{}, err
	}
	verifier, err := s.cipher.Decrypt(record.VerifierCiphertext, userID+":oauth:"+record.ID)
	if err != nil {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
	defer cancel()
	credentials, err := s.adapters.OAuth.Exchange(op, code, verifier)
	if err != nil {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(userID+":calendar:generic")).String()
	connection := d.CalendarConnection{ID: id, UserID: userID, Provider: provider, CalendarID: "primary", Status: "connected"}
	if err = s.encryptCredentials(&connection, credentials); err != nil {
		return connection, err
	}
	if err = s.repo.SaveConnection(ctx, connection); err != nil {
		return connection, err
	}
	return connection, s.repo.ScheduleCalendarSync(ctx, userID)
}

// Disconnect revokes the acting account's integration connection and stops its future
// synchronization.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Disconnect(ctx context.Context, userID, id string) error {
	items, err := s.repo.Connections(ctx, userID)
	if err != nil {
		return err
	}
	var found *d.CalendarConnection
	for _, c := range items {
		if c.ID == id {
			v := c
			found = &v
			break
		}
	}
	if found == nil {
		return apperrors.ErrNotFound
	}
	token := ""
	if found.RefreshCiphertext != "" && s.cipher != nil {
		token, _ = s.cipher.Decrypt(found.RefreshCiphertext, userID+":calendar:"+id+":refresh")
	}
	if err = s.repo.RevokeConnection(ctx, userID, id); err != nil {
		return err
	}
	if s.adapters.OAuth != nil && token != "" {
		op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
		defer cancel()
		_ = s.adapters.OAuth.Revoke(op, token)
	}
	return nil
}

// CalendarMappings returns saved conference-to-calendar mappings for the acting account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: The collected calendar mapping values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) CalendarMappings(ctx context.Context, userID, conferenceID string) ([]d.CalendarMapping, error) {
	return s.repo.CalendarMappings(ctx, userID, conferenceID)
}

// Tick performs one bounded maintenance or delivery iteration after persisted state is
// available.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Tick(ctx context.Context) error {
	return s.repo.ScheduleReminders(ctx, s.options.ReminderOffsets)
}

// eventPayload contains only references needed by a background event.
type eventPayload struct {
	Event          string `json:"event"`
	NotificationID string `json:"notificationId"`
	Channel        string `json:"channel"`
}

// Handle dispatches the validated account integration operation to its application handler.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - job: Leased background job and its generation or ownership token.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Handle(ctx context.Context, job jobs.Job) error {
	var payload eventPayload
	if json.Unmarshal(job.Payload, &payload) != nil {
		return jobs.Error{Code: "integration_payload"}
	}
	switch job.Kind {
	case "integrations.invitation":
		return s.deliverInvitation(ctx, job)
	case "integrations.delivery":
		return s.deliver(ctx, job, payload)
	case "integrations.event":
		return s.repo.Fanout(ctx, job, payload.Event)
	case "integrations.conference":
		if err := s.repo.Fanout(ctx, job, payload.Event); err != nil {
			return err
		}
		return s.syncCalendar(ctx, job)
	case "integrations.calendar":
		return s.syncCalendar(ctx, job)
	}
	return jobs.Error{Code: "integration_kind"}
}

// FailJob records a bounded job failure without exposing provider responses or replacing a newer
// generation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - job: Leased background job and its generation or ownership token.
//   - code: Bounded public status or failure code.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) FailJob(ctx context.Context, job jobs.Job, code string) error {
	if job.Kind == "integrations.invitation" {
		repo, ok := s.repo.(invitationDeliveryRepository)
		if !ok {
			return jobs.Error{Code: "invitation_repository"}
		}
		return repo.CompleteInvitation(ctx, job, "failed", code)
	}
	if job.Kind == "integrations.delivery" {
		return s.repo.CompleteDelivery(ctx, job, "failed", code)
	}
	if job.Kind == "integrations.calendar" || job.Kind == "integrations.conference" {
		return s.repo.FailCalendars(ctx, job, code)
	}
	return nil
}

// templateData contains only a minimal safe event view.
type templateData struct{ Subject, Title, Body, URL string }

// RenderEmail builds escaped HTML and plain-text notification email from the versioned template
// and trusted UI link.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//   - to: Destination state or recipient of the operation.
//   - kind: Validated event, job, media or resource category.
//   - title: Validated conference or notification title.
//   - link: Trusted frontend link generated by the application.
//
// Returns:
//   - result 1: The resulting email message.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) RenderEmail(key, to, kind, title, link string) (d.EmailMessage, error) {
	body := eventText(kind)
	data := templateData{Subject: body, Title: title, Body: body, URL: link}
	var h, t bytes.Buffer
	if err := s.html.Execute(&h, data); err != nil {
		return d.EmailMessage{}, err
	}
	if err := s.text.Execute(&t, data); err != nil {
		return d.EmailMessage{}, err
	}
	return d.EmailMessage{IdempotencyKey: key, To: to, Subject: body, HTML: h.String(), Text: t.String()}, nil
}

// eventText builds the bounded calendar-event text from persisted conference data.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//
// Returns:
//   - result 1: Bounded external-calendar event text.
func eventText(kind string) string {
	switch kind {
	case "conference.invited":
		return "Приглашение на встречу"
	case "conference.rescheduled":
		return "Время встречи изменилось"
	case "conference.cancelled":
		return "Встреча отменена"
	case "conference.soon":
		return "Встреча скоро начнётся"
	case "recording.ready":
		return "Запись встречи готова"
	case "transcript.ready":
		return "Расшифровка встречи готова"
	case "summary.ready":
		return "Итоги встречи готовы"
	case "processing.failed":
		return "Обработка встречи не завершилась"
	default:
		return "Обновление встречи"
	}
}

// deliver checks the current recipient authorization immediately before the saved event or
// command is delivered.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - job: Leased background job and its generation or ownership token.
//   - payload: Structured event or command payload.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) deliver(ctx context.Context, job jobs.Job, payload eventPayload) error {
	delivery, err := s.repo.Delivery(ctx, job)
	if err != nil {
		return err
	}
	skip := !delivery.Allowed || !delivery.Preferences.Allows(delivery.Notification.Type)
	if payload.Channel == "email" {
		skip = skip || !delivery.Preferences.Email || s.adapters.Capabilities.Email == "noop"
	} else if payload.Channel == "push" {
		skip = skip || !delivery.Preferences.Push || s.adapters.Capabilities.Push == "noop"
	} else {
		return jobs.Error{Code: "delivery_channel"}
	}
	if skip {
		if err = s.repo.CompleteDelivery(ctx, job, "skipped", ""); err != nil {
			return err
		}
		return jobs.ErrSkip
	}
	conference, err := s.repo.Conference(ctx, job.ConferenceID)
	if err != nil {
		return err
	}
	link := strings.TrimRight(s.options.PublicURL, "/") + "/conferences/" + url.PathEscape(conference.ID)
	if delivery.Notification.Payload.RecordingID != "" && (delivery.Notification.Type == "transcript.ready" || delivery.Notification.Type == "summary.ready") {
		query := url.Values{"recording": {delivery.Notification.Payload.RecordingID}, "tab": {"transcript"}}
		if delivery.Notification.Type == "summary.ready" {
			query.Set("tab", "summary")
		}
		link += "?" + query.Encode()
	}
	op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
	defer cancel()
	if payload.Channel == "email" {
		message, err := s.RenderEmail(job.DedupKey, delivery.Email, delivery.Notification.Type, conference.Title, link)
		if err != nil {
			return err
		}
		if err = s.adapters.Email.Send(op, message); err != nil {
			return err
		}
	} else {
		devices, err := s.repo.Devices(ctx, delivery.Notification.UserID, true)
		if err != nil {
			return err
		}
		if len(devices) == 0 {
			if err = s.repo.CompleteDelivery(ctx, job, "skipped", ""); err != nil {
				return err
			}
			return jobs.ErrSkip
		}
		for _, device := range devices {
			if s.cipher == nil {
				return jobs.Error{Code: "provider_encryption_disabled"}
			}
			token, err := s.cipher.Decrypt(device.TokenCiphertext, device.UserID+":device:"+device.ID)
			if err != nil {
				return jobs.Error{Code: "device_token_invalid"}
			}
			if err = s.adapters.Push.Send(op, d.PushMessage{IdempotencyKey: job.DedupKey + ":" + device.ID, Token: token, Title: conference.Title, Body: eventText(delivery.Notification.Type), URL: link}); err != nil {
				return err
			}
		}
	}
	return s.repo.CompleteDelivery(ctx, job, "delivered", "")
}

// syncCalendar updates the external calendar mapping only while the job lease and conference
// schedule generation remain current.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - job: Leased background job and its generation or ownership token.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) syncCalendar(ctx context.Context, job jobs.Job) error {
	if s.adapters.Capabilities.Calendar == "noop" {
		return jobs.ErrSkip
	}
	release, err := s.repo.AcquireCalendar(ctx, job.ConferenceID)
	if err != nil {
		return err
	}
	defer release()
	conference, err := s.repo.Conference(ctx, job.ConferenceID)
	if err != nil {
		return err
	}
	if job.Version != conference.IntegrationVersion {
		return jobs.ErrSkip
	}
	connections, err := s.repo.Connections(ctx, conference.OwnerID)
	if err != nil {
		return err
	}
	mappings, err := s.repo.MappingsForSync(ctx, conference.ID)
	if err != nil {
		return err
	}
	for _, connection := range connections {
		if connection.Status != "connected" {
			continue
		}
		if (connection.Provider == "mock" && s.adapters.Capabilities.Calendar != "mock") || (connection.Provider != "mock" && s.adapters.Capabilities.Calendar != "http") {
			continue
		}
		mapping := d.CalendarMapping{ConferenceID: conference.ID, ConnectionID: connection.ID, Provider: connection.Provider, ExternalCalendarID: connection.CalendarID, SourceVersion: job.Version, SyncStatus: "pending"}
		for _, m := range mappings {
			if m.ConnectionID == connection.ID {
				mapping.ID = m.ID
				mapping.ExternalEventID = m.ExternalEventID
				break
			}
		}
		if conference.Status != "cancelled" && conference.ScheduledAt == nil {
			continue
		}
		credentials, err := s.connectionCredentials(ctx, &connection)
		if err != nil {
			return err
		}
		duration := 60
		if conference.PlannedDurationMin != nil {
			duration = *conference.PlannedDurationMin
		}
		event := d.CalendarEvent{IdempotencyKey: "calendar:" + conference.ID + ":" + connection.ID, ID: mapping.ExternalEventID, CalendarID: connection.CalendarID, Title: conference.Title, SourceVersion: job.Version, JoinURL: strings.TrimRight(s.options.PublicURL, "/") + "/i/" + url.PathEscape(conference.InviteCode)}
		if conference.ScheduledAt != nil {
			event.StartsAt = conference.ScheduledAt.UTC()
			event.EndsAt = event.StartsAt.Add(time.Duration(duration) * time.Minute)
		}
		op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
		if conference.Status == "cancelled" {
			if event.ID != "" {
				err = s.adapters.Calendar.CancelEvent(op, credentials, event)
			}
			mapping.SyncStatus = "cancelled"
		} else if event.ID == "" {
			var result d.CalendarEvent
			result, err = s.adapters.Calendar.CreateEvent(op, credentials, event)
			mapping.ExternalEventID = result.ID
			// After a lost response, idempotent creation may return an older event version. A newer schedule
			// must update the same identifier rather than marking obsolete data synchronized.
			if err == nil && (result.SourceVersion != job.Version || !result.StartsAt.Equal(event.StartsAt) || !result.EndsAt.Equal(event.EndsAt) || result.Title != event.Title || result.JoinURL != event.JoinURL) {
				event.ID = result.ID
				event.IdempotencyKey += ":" + strconv.FormatInt(job.Version, 10)
				_, err = s.adapters.Calendar.UpdateEvent(op, credentials, event)
			}
			mapping.SyncStatus = "synced"
		} else {
			event.IdempotencyKey += ":" + strconv.FormatInt(job.Version, 10)
			_, err = s.adapters.Calendar.UpdateEvent(op, credentials, event)
			mapping.SyncStatus = "synced"
		}
		cancel()
		if err != nil {
			mapping.SyncStatus = "failed"
			_ = s.repo.SaveMapping(ctx, job, mapping)
			return err
		}
		now := time.Now().UTC()
		mapping.LastSyncedAt = &now
		if err = s.repo.SaveMapping(ctx, job, mapping); err != nil {
			return err
		}
	}
	return nil
}

// encryptCredentials encrypts provider credentials before they are persisted.
//
// Parameters:
//   - connection: Current account integration or physical transport connection.
//   - credentials: Server-only provider credentials; never expose them in logs or public
//     responses.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) encryptCredentials(connection *d.CalendarConnection, credentials d.CalendarCredentials) error {
	if s.cipher == nil {
		return apperrors.ErrUnavailable
	}
	binding := connection.UserID + ":calendar:" + connection.ID
	access, err := s.cipher.Encrypt(credentials.AccessToken, binding+":access")
	if err != nil {
		return err
	}
	refresh := ""
	if credentials.RefreshToken != "" {
		refresh, err = s.cipher.Encrypt(credentials.RefreshToken, binding+":refresh")
		if err != nil {
			return err
		}
	}
	connection.AccessCiphertext = access
	connection.RefreshCiphertext = refresh
	connection.ExpiresAt = credentials.ExpiresAt
	connection.Scopes = credentials.Scopes
	return nil
}

// connectionCredentials loads and decrypts provider credentials, refreshing them when required.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - connection: Current account integration or physical transport connection.
//
// Returns:
//   - result 1: The resulting calendar credentials.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) connectionCredentials(ctx context.Context, connection *d.CalendarConnection) (d.CalendarCredentials, error) {
	if connection.Provider == "mock" {
		return d.CalendarCredentials{}, nil
	}
	if s.cipher == nil {
		return d.CalendarCredentials{}, jobs.Error{Code: "provider_encryption_disabled"}
	}
	binding := connection.UserID + ":calendar:" + connection.ID
	access, err := s.cipher.Decrypt(connection.AccessCiphertext, binding+":access")
	if err != nil {
		return d.CalendarCredentials{}, jobs.Error{Code: "calendar_token_invalid"}
	}
	refresh := ""
	if connection.RefreshCiphertext != "" {
		refresh, err = s.cipher.Decrypt(connection.RefreshCiphertext, binding+":refresh")
		if err != nil {
			return d.CalendarCredentials{}, jobs.Error{Code: "calendar_token_invalid"}
		}
	}
	credentials := d.CalendarCredentials{AccessToken: access, RefreshToken: refresh, ExpiresAt: connection.ExpiresAt, Scopes: connection.Scopes}
	if credentials.ExpiresAt != nil && credentials.ExpiresAt.Before(time.Now().Add(time.Minute)) {
		if s.adapters.OAuth == nil || refresh == "" {
			return credentials, jobs.Error{Code: "calendar_reconnect_required"}
		}
		credentials, err = s.adapters.OAuth.Refresh(ctx, refresh)
		if err != nil {
			return credentials, err
		}
		oldRefresh := connection.RefreshCiphertext
		if err = s.encryptCredentials(connection, credentials); err != nil {
			return credentials, err
		}
		if err = s.repo.RefreshConnection(ctx, *connection, oldRefresh); err != nil {
			return credentials, err
		}
	}
	return credentials, nil
}

// randomToken generates a cryptographically random one-time token.
//
// Returns:
//   - result 1: Cryptographically random URL-safe secret.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// digest hashes a one-time secret for comparison without storing the raw value.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: Hex-encoded digest of the supplied secret.
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

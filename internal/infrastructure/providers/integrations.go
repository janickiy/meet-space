package providers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	d "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
)

// AdapterConfig selects delivery mode and the trusted server-gateway endpoint; user-supplied
// URLs are rejected.
type AdapterConfig struct {
	Mode      string
	Endpoint  string
	Secret    string
	Timeout   time.Duration
	AllowHTTP bool
}

// IntegrationConfig groups independent delivery-channel and calendar OAuth settings.
type IntegrationConfig struct {
	Email, Push, Calendar AdapterConfig
	SMTP                  SMTPConfig
	OAuth                 OAuthConfig
	MockConnectAllowed    bool
}

// NewIntegrations constructs the integrations with the dependencies and limits required for
// bounded external integrations.
//
// Parameters:
//   - cfg: Operator-controlled configuration and processing limits.
//
// Returns:
//   - result 1: The resulting providers.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewIntegrations(cfg IntegrationConfig) (d.Providers, error) {
	var email d.EmailProvider
	emailMode := cfg.Email.Mode
	if emailMode == "smtp" {
		var err error
		email, err = NewSMTPEmail(cfg.SMTP)
		if err != nil {
			return d.Providers{}, err
		}
	} else {
		e, err := newGateway(cfg.Email)
		if err != nil {
			return d.Providers{}, err
		}
		email, emailMode = &EmailAdapter{e}, e.mode
	}
	p, err := newGateway(cfg.Push)
	if err != nil {
		return d.Providers{}, err
	}
	c, err := newGateway(cfg.Calendar)
	if err != nil {
		return d.Providers{}, err
	}
	result := d.Providers{Email: email, Push: &PushAdapter{p}, Calendar: &CalendarAdapter{gateway: c}, Capabilities: d.Capabilities{Email: emailMode, Push: p.mode, Calendar: c.mode, MockConnectAllowed: cfg.MockConnectAllowed && c.mode == "mock"}}
	if cfg.OAuth.ClientID != "" {
		o, err := NewOAuth(cfg.OAuth)
		if err != nil {
			return result, err
		}
		result.OAuth = o
		result.Capabilities.CalendarOAuthConfigured = true
	}
	return result, nil
}

// gateway performs bounded JSON HTTP calls or deterministic mock behavior without network
// access.
type gateway struct {
	mode     string
	endpoint string
	secret   string
	client   *http.Client
}

// newGateway creates a bounded HTTP gateway for an operator-configured integration endpoint.
//
// Parameters:
//   - cfg: Operator-controlled configuration and processing limits.
//
// Returns:
//   - result 1: The constructed or loaded gateway.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func newGateway(cfg AdapterConfig) (*gateway, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = "noop"
	}
	if mode != "noop" && mode != "mock" && mode != "http" {
		return nil, errors.New("invalid integration provider mode")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Timeout > 5*time.Minute {
		return nil, errors.New("integration provider timeout exceeds five minutes")
	}
	g := &gateway{mode: mode, secret: cfg.Secret, endpoint: strings.TrimRight(cfg.Endpoint, "/"), client: &http.Client{Timeout: cfg.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if mode == "http" {
		u, err := url.Parse(cfg.Endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || (u.Scheme != "https" && !(cfg.AllowHTTP && u.Scheme == "http")) {
			return nil, errors.New("integration endpoint must be a trusted HTTPS URL")
		}
		if cfg.Secret == "" {
			return nil, errors.New("HTTP integration provider secret is required")
		}
	}
	return g, nil
}

// call executes a bounded configured-provider request and returns only safe failure
// classifications.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - operation: Fixed allowlisted provider or protocol operation.
//   - key: Storage, cache or cryptographic key used by the operation.
//   - input: Input data required by this operation.
//   - output: Destination or generated result required by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (g *gateway) call(ctx context.Context, operation, key string, input, output any) error {
	if g.mode == "noop" {
		return jobs.ErrSkip
	}
	if g.mode == "mock" {
		return nil
	}
	body, err := json.Marshal(input)
	if err != nil {
		return jobs.Error{Code: "provider_input", Retryable: false}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint+"/"+operation, bytes.NewReader(body))
	if err != nil {
		return jobs.Error{Code: "provider_request"}
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	res, err := g.client.Do(req)
	if err != nil {
		return jobs.Error{Code: "provider_unavailable", Retryable: true}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
	if err != nil {
		return jobs.Error{Code: "provider_response", Retryable: true}
	}
	if len(raw) > 1<<20 {
		return jobs.Error{Code: "provider_response_too_large"}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		retry := res.StatusCode == 429 || res.StatusCode == 408 || res.StatusCode >= 500
		after := time.Duration(0)
		if seconds, err := time.ParseDuration(res.Header.Get("Retry-After") + "s"); err == nil && seconds > 0 && seconds <= time.Hour {
			after = seconds
		}
		code := "provider_http"
		if res.StatusCode == 429 {
			code = "provider_rate_limited"
		}
		return jobs.Error{Code: code, Retryable: retry, RetryAfter: after}
	}
	if output != nil && json.Unmarshal(raw, output) != nil {
		return jobs.Error{Code: "provider_invalid_response"}
	}
	return nil
}

// EmailAdapter implements EmailProvider through a server gateway or explicitly labeled mock and
// disabled modes.
type EmailAdapter struct{ gateway *gateway }

// Send delivers the bounded email with a stable message identity; the configured transport
// determines retry deduplication.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - message: Bounded email content, recipient and stable delivery identity.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (a *EmailAdapter) Send(ctx context.Context, message d.EmailMessage) error {
	return a.gateway.call(ctx, "email/send", message.IdempotencyKey, message, nil)
}

// PushAdapter implements safe push delivery through a replaceable gateway.
type PushAdapter struct{ gateway *gateway }

// Send delivers a minimal notification to the selected registered device without transcript text
// or provider secrets.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - message: Minimal push notification and its registered device recipient.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (a *PushAdapter) Send(ctx context.Context, message d.PushMessage) error {
	return a.gateway.call(ctx, "push/send", message.IdempotencyKey, message, nil)
}

// CalendarAdapter preserves a stable external event identifier and performs lifecycle operations
// through the gateway.
type CalendarAdapter struct{ gateway *gateway }

// calendarInput serializes credentials only into the protected trusted-gateway request, never
// public API responses or logs.
type calendarInput struct {
	Event       d.CalendarEvent `json:"event"`
	AccessToken string          `json:"accessToken"`
}

// invoke executes the configured calendar operation, using deterministic idempotency in
// explicitly permitted mock mode.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - operation: Fixed allowlisted provider or protocol operation.
//   - credentials: Server-only provider credentials; never expose them in logs or public
//     responses.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - result 1: The resulting calendar event.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (a *CalendarAdapter) invoke(ctx context.Context, operation string, credentials d.CalendarCredentials, event d.CalendarEvent) (d.CalendarEvent, error) {
	if a.gateway.mode == "mock" {
		if event.ID == "" {
			sum := sha256.Sum256([]byte(event.IdempotencyKey))
			event.ID = "mock-" + hex.EncodeToString(sum[:12])
		}
		return event, nil
	}
	var out d.CalendarEvent
	err := a.gateway.call(ctx, "calendar/"+operation, event.IdempotencyKey+":"+operation, calendarInput{event, credentials.AccessToken}, &out)
	if err != nil {
		return out, err
	}
	if out.ID == "" || len(out.ID) > 512 || len(out.CalendarID) > 512 {
		return out, jobs.Error{Code: "calendar_missing_event_id"}
	}
	return out, nil
}

// CreateEvent creates an idempotent calendar event through the configured provider.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - c: The calendar credentials value processed by this operation.
//   - e: The calendar event state processed by this operation.
//
// Returns:
//   - result 1: The resulting calendar event.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (a *CalendarAdapter) CreateEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) (d.CalendarEvent, error) {
	return a.invoke(ctx, "create", c, e)
}

// UpdateEvent updates the mapped external calendar event for the current conference schedule.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - c: The calendar credentials value processed by this operation.
//   - e: The calendar event state processed by this operation.
//
// Returns:
//   - result 1: The resulting calendar event.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (a *CalendarAdapter) UpdateEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) (d.CalendarEvent, error) {
	return a.invoke(ctx, "update", c, e)
}

// CancelEvent cancels the external calendar event associated with the saved mapping.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - c: The calendar credentials value processed by this operation.
//   - e: The calendar event state processed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (a *CalendarAdapter) CancelEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) error {
	if a.gateway.mode == "mock" {
		return nil
	}
	return a.gateway.call(ctx, "calendar/cancel", e.IdempotencyKey+":cancel", calendarInput{e, c.AccessToken}, nil)
}

// GetEvent reads the mapped external calendar event for reconciliation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - c: The calendar credentials value processed by this operation.
//   - e: The calendar event state processed by this operation.
//
// Returns:
//   - result 1: The resulting calendar event.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (a *CalendarAdapter) GetEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) (d.CalendarEvent, error) {
	return a.invoke(ctx, "get", c, e)
}

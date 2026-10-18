package app

import (
	"context"
	"encoding/json"
	"time"

	content "github.com/janickiy/meet-space/internal/domain/content"
	integrations "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/operations"
)

// measuredEmail counts delivery attempts around a replaceable adapter without copying email
// content.
type measuredEmail struct{ integrations.EmailProvider }

// Send delivers the bounded email with a stable message identity; the configured transport
// determines retry deduplication.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - message: Bounded email content, recipient and stable delivery identity.
//
// Returns:
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func (p measuredEmail) Send(ctx context.Context, message integrations.EmailMessage) (err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("email", time.Since(started), err) }()
	return p.EmailProvider.Send(ctx, message)
}

// measuredPush measures platform delivery calls without placing device tokens in metrics.
type measuredPush struct{ integrations.PushProvider }

// Send delivers a minimal notification to the selected registered device without transcript text
// or provider secrets.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - message: Minimal push notification and its registered device recipient.
//
// Returns:
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func (p measuredPush) Send(ctx context.Context, message integrations.PushMessage) (err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("push", time.Since(started), err) }()
	return p.PushProvider.Send(ctx, message)
}

// measuredCalendar preserves the provider-independent contract and a fixed set of technical
// metric labels.
type measuredCalendar struct{ integrations.CalendarProvider }

// CreateEvent creates an idempotent calendar event through the configured provider.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - credentials: Server-only provider credentials; never expose them in logs or public
//     responses.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - out: The resulting calendar event.
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func (p measuredCalendar) CreateEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (out integrations.CalendarEvent, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.CreateEvent(ctx, credentials, event)
}

// UpdateEvent updates the mapped external calendar event for the current conference schedule.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - credentials: Server-only provider credentials; never expose them in logs or public
//     responses.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - out: The resulting calendar event.
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func (p measuredCalendar) UpdateEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (out integrations.CalendarEvent, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.UpdateEvent(ctx, credentials, event)
}

// CancelEvent cancels the external calendar event associated with the saved mapping.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - credentials: Server-only provider credentials; never expose them in logs or public
//     responses.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func (p measuredCalendar) CancelEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.CancelEvent(ctx, credentials, event)
}

// GetEvent reads the mapped external calendar event for reconciliation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - credentials: Server-only provider credentials; never expose them in logs or public
//     responses.
//   - event: Trusted event to publish or process.
//
// Returns:
//   - out: The resulting calendar event.
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func (p measuredCalendar) GetEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (out integrations.CalendarEvent, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.GetEvent(ctx, credentials, event)
}

// measuredSTT measures the audio request while preserving streaming and the adapter's provider
// name.
type measuredSTT struct{ content.TranscriptionProvider }

// Transcribe submits bounded recording audio to the configured transcription provider.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - out: The resulting transcription result.
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func (p measuredSTT) Transcribe(ctx context.Context, request content.TranscriptionRequest) (out content.TranscriptionResult, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("stt", time.Since(started), err) }()
	return p.TranscriptionProvider.Transcribe(ctx, request)
}

// measuredAI measures each chunk or merge without changing model instructions or schema
// validation.
type measuredAI struct{ content.AIProvider }

// Summarize generates a schema-constrained summary from the supplied transcript input.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - out: The resulting raw message.
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func (p measuredAI) Summarize(ctx context.Context, request content.AIRequest) (out json.RawMessage, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("ai", time.Since(started), err) }()
	return p.AIProvider.Summarize(ctx, request)
}

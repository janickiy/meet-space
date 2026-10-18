// Package captions coordinates isolated live speech recognition and recovery of saved final
// utterances.
package captions

import (
	"context"
	domain "github.com/janickiy/meet-space/internal/domain/captions"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"io"
	"time"
)

// Lease binds auxiliary processing to the current conference-settings version.
type Lease struct {
	domain.State
	Token     string
	EnabledAt time.Time
	Attempts  int
}

// Repository keeps SQL authorization and ownership checks separate from audio processing.
type Repository interface {
	// Read reads the live-caption state state or stream required by the caller.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The resulting state.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Read(context.Context, string, string) (domain.State, error)
	// Set updates the persisted live-caption state settings after validating the request.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Whether the requested feature or preference should be enabled.
	//   - argument 5: Requested speech language within the supported provider contract.
	//
	// Returns:
	//   - result 1: The resulting state.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Set(context.Context, string, string, bool, string) (domain.State, error)
	// Finals returns a cursor-bounded page of saved final captions so clients can recover missed
	// realtime events.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Boundary after which results or state are selected.
	//   - argument 5: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The collected caption values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Finals(context.Context, string, string, int64, int) ([]domain.Caption, error)
	// Claim atomically acquires the current live-caption state lease or ownership record.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Whether the requested processing lease includes technical activity
	//     aggregation.
	//   - argument 3: Maximum permitted size or number of characters.
	//   - argument 4: Maximum permitted job attempts or the current retry count.
	//
	// Returns:
	//   - result 1: The resulting lease.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Claim(context.Context, bool, int, int) (Lease, error)
	// Renew extends the live-caption state lease only while its owner and fencing identity still
	// match.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Current lease identity used to fence stale workers.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Renew(context.Context, Lease) (bool, error)
	// Finish records completion of the leased live-caption state operation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Current lease identity used to fence stale workers.
	//   - argument 3: Lifecycle state or response status to record.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Finish(context.Context, Lease, string) error
	// Status records the bounded processing status associated with the current caption lease.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Current lease identity used to fence stale workers.
	//   - argument 3: Lifecycle state or response status to record.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Status(context.Context, Lease, string) error
	// SaveFinal persists a final live-caption event only for the current lease and revision.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Current lease identity used to fence stale workers.
	//   - argument 3: Validated caption event to persist.
	//   - argument 4: Maximum permitted size or number of characters.
	//
	// Returns:
	//   - result 1: The resulting caption.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SaveFinal(context.Context, Lease, domain.Caption, int) (domain.Caption, error)
	// Speaker resolves the public speaker label for the caption's authorized participant.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Participant identifier whose speaker information is requested.
	//
	// Returns:
	//   - result 1: Verified public speaker label.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Speaker(context.Context, string, string) (string, error)
	// Observe persists bounded activity observations for the currently leased conference-caption
	// run.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Current lease identity used to fence stale workers.
	//   - argument 3: Participant identifier whose speaker information is requested.
	//   - argument 4: Observed speech duration in milliseconds.
	//   - argument 5: Total observed audio duration in milliseconds.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Observe(context.Context, Lease, string, int64, int64) error
}

// AudioTap opens a protected SFU stream without using RabbitMQ for individual frames.
type AudioTap interface {
	// Open opens the scoped media, recording or session resource required by this component.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting read closer.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Open(context.Context, string, string) (io.ReadCloser, error)
}

// Decoder converts Opus into bounded PCM chunks outside the SFU.
type Decoder interface {
	// Decode decodes the bounded encoded audio stream into PCM frames for the caption pipeline.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Media-track metadata or negotiated track instance.
	//   - argument 3: Encoded media-frame input channel.
	//   - argument 4: Callback invoked while the operation's resource ownership and authorization
	//     remain valid.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Decode(context.Context, media.EgressTrack, <-chan media.EgressFrame, func([]byte) error) error
}

// Events publishes bounded caption results through the existing realtime transport.
type Events interface {
	// Publish publishes the saved live-caption state change through its configured event or command
	// transport.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Scoped publication and subscription transport.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Publish(context.Context, realtime.Bus) error
}

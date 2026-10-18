package conferences

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

// ControlRepository defines the replaceable dependency contract for conference lifecycle and
// participant membership.
type ControlRepository interface {
	// Moderate applies a participant moderation action after checking the actor's role and the
	// target restrictions.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Conference participant identifier.
	//   - argument 5: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting participant.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Moderate(context.Context, string, string, string, domain.ModerationRequest) (domain.Participant, error)
	// UpdateMediaState saves the participant's declared media-source state.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Current state or validated state update.
	//
	// Returns:
	//   - result 1: The resulting participant.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	UpdateMediaState(context.Context, string, string, domain.MediaState) (domain.Participant, error)
	// ReconcileParticipants returns participants whose persisted media restrictions need
	// reconciliation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Boundary after which results or state are selected.
	//   - argument 3: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The collected participant values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ReconcileParticipants(context.Context, string, int) ([]domain.Participant, error)
}

// PolicyController defines the replaceable dependency contract for conference lifecycle and
// participant membership.
type PolicyController interface {
	// SetParticipantPolicy updates participant policy for conference lifecycle and participant
	// membership.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Conference participant identifier.
	//   - argument 4: Effective server-side media policy.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SetParticipantPolicy(context.Context, string, string, media.ParticipantPolicy) error
}

// ControlEvents defines the replaceable dependency contract for conference lifecycle and
// participant membership.
type ControlEvents interface {
	// Broadcast publishes a trusted event only to the currently authorized conference or
	// conversation recipients.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Trusted event to publish or process.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Broadcast(context.Context, realtime.Envelope) error
	// ConferenceChanged notifies connected sessions or observers of the conference's committed state
	// change.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	ConferenceChanged(context.Context, string)
}

// ControlService coordinates persisted moderation with enforcement on live media.
type ControlService struct {
	repo   ControlRepository
	media  PolicyController
	events ControlEvents
}

// NewControlService constructs the control service with the dependencies and limits required for
// conference lifecycle and participant membership.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - controller: Media policy and command controller.
//   - events: Destination for committed lifecycle or chat events.
//
// Returns:
//   - result 1: The constructed or loaded control service.
func NewControlService(repo ControlRepository, controller PolicyController, events ControlEvents) *ControlService {
	return &ControlService{repo: repo, media: controller, events: events}
}

// Moderate applies a participant moderation action after checking the actor's role and the
// target restrictions.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - participantID: Conference participant identifier.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting participant view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ControlService) Moderate(ctx context.Context, userID, conferenceID, participantID string, request domain.ModerationRequest) (domain.ParticipantView, error) {
	p, err := s.repo.Moderate(ctx, conferenceID, userID, participantID, request)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	if s.events != nil {
		kind := "participant.media.updated"
		if request.Action == "role" {
			kind = "participant.role.updated"
		}
		if request.Action == "kick" {
			kind = "participant.kicked"
		}
		_ = s.events.Broadcast(ctx, realtime.Event(kind, conferenceID, map[string]any{"participant": p.View()}))
		s.events.ConferenceChanged(ctx, conferenceID)
	}
	slog.Info("conference moderation", "conference_id", conferenceID, "actor_user_id", userID, "participant_id", participantID, "action", request.Action, "policy_version", p.MediaPolicyVersion)
	if s.media != nil {
		if err = s.media.SetParticipantPolicy(ctx, conferenceID, participantID, media.PolicyForParticipant(p)); err != nil {
			return p.View(), apperrors.New(apperrors.ErrUnavailable, "moderation saved; media enforcement is retrying")
		}
	}
	return p.View(), nil
}

// UpdateMediaState saves the participant's declared media-source state.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - state: Current state or validated state update.
//
// Returns:
//   - result 1: The resulting participant view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ControlService) UpdateMediaState(ctx context.Context, userID, conferenceID string, state domain.MediaState) (domain.ParticipantView, error) {
	p, err := s.repo.UpdateMediaState(ctx, conferenceID, userID, state)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	if s.events != nil {
		_ = s.events.Broadcast(ctx, realtime.Event("participant.media.updated", conferenceID, map[string]any{"participant": p.View()}))
	}
	return p.View(), nil
}

// Disconnected handles closure of a physical connection and releases only the resources owned by
// that connection.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - session: Persistent or physical session state, as defined by its type.
func (s *ControlService) Disconnected(ctx context.Context, session realtime.Session) {
	repo, ok := s.repo.(interface {
		// ClearDisconnectedMedia clears declared media state when the participant no longer has an
		// active authorized session.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Conference identifier limiting the operation scope.
		//   - argument 3: Conference participant identifier.
		//
		// Returns:
		//   - result 1: The resulting participant.
		//   - result 2: Whether the operation's documented condition holds or its conditional state
		//     change succeeded.
		//   - result 3: nil on success; otherwise the validation, access, transport or persistence
		//     failure.
		ClearDisconnectedMedia(context.Context, string, string) (domain.Participant, bool, error)
	})
	if !ok {
		return
	}
	p, changed, err := repo.ClearDisconnectedMedia(ctx, session.ConferenceID, session.ParticipantID)
	if err == nil && changed && s.events != nil {
		_ = s.events.Broadcast(ctx, realtime.Event("participant.media.updated", session.ConferenceID, map[string]any{"participant": p.View()}))
	}
}

// Run runs conference lifecycle and participant membership until the context is canceled or
// processing fails.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
func (s *ControlService) Run(ctx context.Context) {
	if s.media == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	after := uuid.Nil.String()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		query, cancel := context.WithTimeout(ctx, 3*time.Second)
		rows, err := s.repo.ReconcileParticipants(query, after, 64)
		cancel()
		if err != nil {
			continue
		}
		if len(rows) < 64 {
			after = uuid.Nil.String()
		} else {
			after = rows[len(rows)-1].ID
		}
		var workers sync.WaitGroup
		jobs := make(chan domain.Participant)
		for range min(8, len(rows)) {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for p := range jobs {
					op, done := context.WithTimeout(ctx, 2*time.Second)
					_ = s.media.SetParticipantPolicy(op, p.ConferenceID, p.ID, media.PolicyForParticipant(p))
					done()
				}
			}()
		}
		for _, p := range rows {
			select {
			case jobs <- p:
			case <-ctx.Done():
				close(jobs)
				workers.Wait()
				return
			}
		}
		close(jobs)
		workers.Wait()
	}
}

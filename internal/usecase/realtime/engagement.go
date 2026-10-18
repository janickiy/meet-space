package realtime

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
)

// Engagement checks participant admission before publishing temporary emoji reactions.
type Engagement struct {
	repo Repository
	hub  *Hub
}

// NewEngagement constructs the engagement with the dependencies and limits required for physical
// sessions and realtime event delivery.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - hub: Realtime hub coordinating authorized socket delivery.
//
// Returns:
//   - result 1: The constructed or loaded engagement.
func NewEngagement(repo Repository, hub *Hub) *Engagement {
	return &Engagement{repo: repo, hub: hub}
}

// Authorize checks the current identity and saved access state before physical sessions and
// realtime event delivery.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Engagement) Authorize(ctx context.Context, conferenceID, userID string) error {
	_, err := s.authorized(ctx, conferenceID, userID)
	return err
}

// authorized rechecks saved membership against the active conference so revoked access cannot be
// reused.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: The resulting participant.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Engagement) authorized(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	actor, err := s.repo.Authorize(ctx, conferenceID, userID)
	if err != nil {
		return actor, err
	}
	status, roster, err := s.repo.Roster(ctx, conferenceID)
	if err != nil {
		return actor, err
	}
	if status != conferences.Active {
		return actor, apperrors.New(apperrors.ErrConflict, "conference is not active")
	}
	for _, p := range roster {
		if p.ID == actor.ID && p.CanParticipate() {
			return p, nil
		}
	}
	return actor, apperrors.ErrForbidden
}

// Reaction publishes a supported temporary reaction on behalf of an admitted conference
// participant.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - userID: Acting account identifier used to scope access.
//   - emoji: One of the protocol-supported temporary reactions.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Engagement) Reaction(ctx context.Context, conferenceID, userID, emoji string) error {
	if !domain.AllowedReaction(emoji) {
		return apperrors.New(apperrors.ErrInvalidInput, "unsupported reaction")
	}
	actor, err := s.authorized(ctx, conferenceID, userID)
	if err != nil {
		return err
	}
	return s.hub.Broadcast(ctx, domain.Event("reaction.created", conferenceID, map[string]string{"participantId": actor.ID, "emoji": emoji}))
}

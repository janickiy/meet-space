package conferences

import (
	"context"
	"errors"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
)

// productRepository defines the replaceable dependency contract for conference lifecycle and
// participant membership.
type productRepository interface {
	// DecideAdmission records an authorized waiting-room acceptance or rejection.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Acting account identifier used to scope access.
	//   - argument 4: Conference participant identifier.
	//   - argument 5: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting participant.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	DecideAdmission(context.Context, string, string, string, domain.AdmissionRequest) (domain.Participant, error)
	// UpdateSchedule saves the authorized conference schedule and its updated synchronization
	// version.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Acting account identifier used to scope access.
	//   - argument 4: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting conference.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	UpdateSchedule(context.Context, string, string, domain.ScheduleRequest) (domain.Conference, error)
	// Timeline returns the account's paginated upcoming, active or past conference timeline.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Search text or validated query filters.
	//
	// Returns:
	//   - result 1: The collected conference values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Timeline(context.Context, string, domain.TimelineQuery) ([]domain.Conference, error)
	// History builds the authorized completed-conference view with participant history and recording
	// summaries.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting history view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	History(context.Context, string, string) (domain.HistoryView, error)
}

// Self returns the acting account's membership, including waiting-room and admission state.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting participant view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Self(ctx context.Context, userID, id string) (domain.ParticipantView, error) {
	p, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	return p.View(), err
}

// Admission accepts or rejects waiting-room admission after checking the acting member's
// authority.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//   - participantID: Conference participant identifier.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting participant view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Admission(ctx context.Context, userID, id, participantID string, request domain.AdmissionRequest) (domain.ParticipantView, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.ParticipantView{}, apperrors.ErrUnavailable
	}
	p, err := repo.DecideAdmission(ctx, id, userID, participantID, request)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	kind := "participant.admitted"
	if request.Decision == "reject" {
		kind = "participant.rejected"
	}
	s.event(ctx, kind, id, p)
	return p.View(), nil
}

// Schedule updates a planned conference schedule after validating ownership and time
// constraints.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Schedule(ctx context.Context, userID, id string, request domain.ScheduleRequest) (domain.View, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.View{}, apperrors.ErrUnavailable
	}
	c, err := repo.UpdateSchedule(ctx, id, userID, request)
	if err != nil {
		return domain.View{}, err
	}
	s.changed(ctx, id)
	return c.View(), nil
}

// Timeline returns the account's paginated upcoming, active or past conference timeline.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - query: Search text or validated query filters.
//
// Returns:
//   - result 1: The resulting timeline page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Timeline(ctx context.Context, userID string, query domain.TimelineQuery) (domain.TimelinePage, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.TimelinePage{}, apperrors.ErrUnavailable
	}
	rows, err := repo.Timeline(ctx, userID, query)
	if err != nil {
		return domain.TimelinePage{}, err
	}
	page := domain.TimelinePage{Items: []domain.View{}}
	if len(rows) > query.Limit {
		rows = rows[:query.Limit]
		cursor := domain.EncodeTimelineCursor(rows[len(rows)-1])
		page.NextCursor = &cursor
	}
	for _, c := range rows {
		// Listing does not require invitation data. Preserve waiting-membership privacy without querying
		// membership separately for every conference.
		view := c.View()
		view.InviteCode, view.InviteURL = "", ""
		page.Items = append(page.Items, view)
	}
	return page, nil
}

// History builds the authorized completed-conference view with participant history and recording
// summaries.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting history view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) History(ctx context.Context, userID, id string) (domain.HistoryView, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.HistoryView{}, apperrors.ErrUnavailable
	}
	return repo.History(ctx, id, userID)
}

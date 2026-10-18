package conferences

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// repository defines the replaceable dependency contract for conference lifecycle and
// participant membership.
type repository interface {
	// Create creates and persists a new conference for the acting account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//   - argument 3: Account, participant or worker that owns the resource.
	//
	// Returns:
	//   - result 1: The resulting conference.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Create(context.Context, domain.Conference, domain.Participant) (domain.Conference, error)
	// Get loads the requested conference within the caller's authorized scope.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting conference.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Get(context.Context, string) (domain.Conference, error)
	// GetByInvite loads the conference associated with a valid invitation code.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Bounded public status or failure code.
	//
	// Returns:
	//   - result 1: The resulting conference.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetByInvite(context.Context, string) (domain.Conference, error)
	// ListForUser returns only conferences visible through the specified account's membership.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Maximum number of results or operations in this batch.
	//   - argument 4: Number of results skipped before the requested page.
	//
	// Returns:
	//   - result 1: The collected conference values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ListForUser(context.Context, string, int, int) ([]domain.Conference, error)
	// Membership loads the account's saved membership in the requested conference.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Acting account identifier used to scope access.
	//
	// Returns:
	//   - result 1: The resulting participant.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Membership(context.Context, string, string) (domain.Participant, error)
	// Participants returns the conference participants visible to the acting account.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Maximum number of results or operations in this batch.
	//   - argument 4: Number of results skipped before the requested page.
	//
	// Returns:
	//   - result 1: The collected participant values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Participants(context.Context, string, int, int) ([]domain.Participant, error)
	// Transition applies an allowed conference lifecycle transition without overwriting a newer
	// state.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//   - argument 4: Target state, account or decoded value, as defined by its type.
	//
	// Returns:
	//   - result 1: The resulting conference.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Transition(context.Context, string, string, domain.Status) (domain.Conference, error)
	// Join creates or resumes the authorized membership or media-room connection, preserving its
	// current identity.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//   - argument 3: Acting account identifier used to scope access.
	//   - argument 4: Validated conference invitation code.
	//
	// Returns:
	//   - result 1: The resulting participant.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Join(context.Context, string, users.User, string) (domain.Participant, error)
	// Leave removes the active membership or media connection while preserving the saved
	// conversation or conference history.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting participant.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Leave(context.Context, string, string) (domain.Participant, error)
}

// userRepository defines the replaceable dependency contract for conference lifecycle and
// participant membership.
type userRepository interface {
	// GetByID loads the conference by ID.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the resource addressed by this operation.
	//
	// Returns:
	//   - result 1: The resulting user.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetByID(context.Context, string) (users.User, error)
}

// Service coordinates conference lifecycle and participant membership through its configured
// persistence and transport dependencies.
type Service struct {
	repository repository
	users      userRepository
	invite     func() (string, error)
	observer   interface {
		// ConferenceChanged notifies connected sessions or observers of the conference's committed state
		// change.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Conference identifier limiting the operation scope.
		ConferenceChanged(context.Context, string)
	}
}

// NewService constructs the service with the dependencies and limits required for conference
// lifecycle and participant membership.
//
// Parameters:
//   - repository: Persistence dependency used by the application operation.
//   - users: Account identifiers or account repository, as defined by its type.
//   - invite: Function generating a cryptographically random invitation code.
//
// Returns:
//   - result 1: The constructed or loaded service.
func NewService(repository repository, users userRepository, invite func() (string, error)) *Service {
	return &Service{repository: repository, users: users, invite: invite}
}

// SetObserver updates observer for conference lifecycle and participant membership.
//
// Parameters:
//   - observer: Observer notified after a persisted lifecycle change.
func (s *Service) SetObserver(observer interface {
	// ConferenceChanged notifies connected sessions or observers of the conference's committed state
	// change.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	ConferenceChanged(context.Context, string)
}) {
	s.observer = observer
}

// changed notifies the component observer after local or persisted state changes.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
func (s *Service) changed(ctx context.Context, id string) {
	if s.observer != nil {
		s.observer.ConferenceChanged(ctx, id)
	}
}

// Create creates and persists a new conference for the acting account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Create(ctx context.Context, userID string, request domain.CreateRequest) (domain.View, error) {
	if err := domain.ValidateSchedule(request.ScheduledAt, request.PlannedDurationMin, time.Now()); err != nil {
		return domain.View{}, err
	}
	title, err := domain.NormalizeTitle(request.Title)
	if err != nil {
		return domain.View{}, err
	}
	user, err := s.currentUser(ctx, userID)
	if err != nil {
		return domain.View{}, err
	}
	for range 5 {
		code, err := s.invite()
		if err != nil {
			return domain.View{}, err
		}
		if !validInvite(code) {
			return domain.View{}, fmt.Errorf("invalid generated invite code")
		}
		conference := domain.Conference{ID: uuid.NewString(), OwnerID: user.ID, Title: title, InviteCode: code, Status: domain.Created,
			WaitingRoomEnabled: request.WaitingRoomEnabled, ScheduledAt: request.ScheduledAt, PlannedDurationMin: request.PlannedDurationMin}
		if conference.ScheduledAt != nil {
			at := conference.ScheduledAt.UTC()
			conference.ScheduledAt = &at
			conference.Status = domain.Scheduled
		}
		ownerID := user.ID
		owner := domain.Participant{ID: uuid.NewString(), ConferenceID: conference.ID, UserID: &ownerID,
			DisplayName: user.ParticipantName(), Role: domain.Owner, Status: domain.Left, AdmissionState: domain.AdmissionAdmitted}
		created, err := s.repository.Create(ctx, conference, owner)
		if errors.Is(err, domain.ErrInviteCollision) {
			continue
		}
		if err != nil {
			return domain.View{}, err
		}
		return created.View(), nil
	}
	return domain.View{}, fmt.Errorf("unable to generate a unique invite code")
}

// List returns a bounded page of conference records using the supplied pagination and filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - limit: Maximum number of results or operations in this batch.
//   - offset: Number of results skipped before the requested page.
//
// Returns:
//   - result 1: The collected view values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]domain.View, error) {
	items, err := s.repository.ListForUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	views := make([]domain.View, 0, len(items))
	for _, item := range items {
		view := item.View()
		view.InviteCode, view.InviteURL = "", ""
		views = append(views, view)
	}
	return views, nil
}

// Read reads the conference state or stream required by the caller.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Read(ctx context.Context, userID, id string) (domain.View, error) {
	conference, err := s.repository.Get(ctx, id)
	if err != nil {
		return domain.View{}, err
	}
	p, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	if err != nil {
		return domain.View{}, err
	}
	view := conference.View()
	if !p.CanReadHistory() {
		view.InviteCode, view.InviteURL = "", ""
	}
	return view, nil
}

// Transition applies an allowed conference lifecycle transition without overwriting a newer
// state.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//   - target: Target state, account or decoded value, as defined by its type.
//
// Returns:
//   - result 1: The resulting view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Transition(ctx context.Context, userID, id string, target domain.Status) (domain.View, error) {
	conference, err := s.repository.Transition(ctx, id, userID, target)
	if err != nil {
		return domain.View{}, err
	}
	s.changed(ctx, id)
	return conference.View(), nil
}

// Participants returns the conference participants visible to the acting account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//   - limit: Maximum number of results or operations in this batch.
//   - offset: Number of results skipped before the requested page.
//
// Returns:
//   - result 1: The collected participant view values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Participants(ctx context.Context, userID, id string, limit, offset int) ([]domain.ParticipantView, error) {
	if _, err := s.repository.Get(ctx, id); err != nil {
		return nil, err
	}
	actor, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	if !actor.CanReadHistory() {
		return nil, apperrors.ErrForbidden
	}
	var items []domain.Participant
	if repo, ok := s.repository.(interface {
		// ParticipantsVisible returns only the participant information visible to the acting account.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Identifier of the resource addressed by this operation.
		//   - argument 3: Acting account identifier used to scope access.
		//   - argument 4: Maximum number of results or operations in this batch.
		//   - argument 5: Number of results skipped before the requested page.
		//
		// Returns:
		//   - result 1: The collected participant values.
		//   - result 2: nil on success; otherwise the validation, access, transport or persistence
		//     failure.
		ParticipantsVisible(context.Context, string, string, int, int) ([]domain.Participant, error)
	}); ok {
		items, err = repo.ParticipantsVisible(ctx, id, userID, limit, offset)
	} else {
		items, err = s.repository.Participants(ctx, id, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	views := make([]domain.ParticipantView, 0, len(items))
	for _, item := range items {
		if !item.IsAdmitted() && actor.Role != domain.Owner && actor.Role != domain.CoHost {
			continue
		}
		views = append(views, item.View())
	}
	return views, nil
}

// Join creates or resumes the authorized membership or media-room connection, preserving its
// current identity.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting participant view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Join(ctx context.Context, userID, id string, request domain.JoinRequest) (domain.ParticipantView, error) {
	if request.InviteCode != "" && !validInvite(request.InviteCode) {
		return domain.ParticipantView{}, apperrors.New(apperrors.ErrInvalidInput, "invalid inviteCode")
	}
	user, err := s.currentUser(ctx, userID)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	participant, err := s.repository.Join(ctx, id, user, request.InviteCode)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	if participant.Status == domain.Waiting {
		s.event(ctx, "participant.waiting", id, participant)
	}
	return participant.View(), nil
}

// Leave removes the active membership or media connection while preserving the saved
// conversation or conference history.
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
func (s *Service) Leave(ctx context.Context, userID, id string) (domain.ParticipantView, error) {
	participant, err := s.repository.Leave(ctx, id, userID)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	return participant.View(), nil
}

// LookupInvite returns the limited conference information associated with a valid invitation
// code.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - code: Bounded public status or failure code.
//
// Returns:
//   - result 1: The resulting invite view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) LookupInvite(ctx context.Context, code string) (domain.InviteView, error) {
	if !validInvite(code) {
		return domain.InviteView{}, apperrors.ErrNotFound
	}
	conference, err := s.repository.GetByInvite(ctx, code)
	if err != nil {
		return domain.InviteView{}, err
	}
	return domain.InviteView{ID: conference.ID, Title: conference.Title, Status: conference.Status, WaitingRoomEnabled: conference.WaitingRoomEnabled, ScheduledAt: conference.ScheduledAt}, nil
}

// JoinInvite joins an authenticated account through a valid invitation while preserving existing
// membership.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - code: Bounded public status or failure code.
//
// Returns:
//   - result 1: The resulting participant view.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) JoinInvite(ctx context.Context, userID, code string) (domain.ParticipantView, error) {
	invite, err := s.LookupInvite(ctx, code)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	return s.Join(ctx, userID, invite.ID, domain.JoinRequest{InviteCode: code})
}

// currentUser loads the acting account and prepares its public participant information.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting user.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) currentUser(ctx context.Context, id string) (users.User, error) {
	user, err := s.users.GetByID(ctx, id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.User{}, apperrors.ErrUnauthorized
	}
	return user, err
}

// event builds the trusted event representing a committed conference membership change.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - kind: Validated event, job, media or resource category.
//   - id: Identifier of the resource addressed by this operation.
//   - participant: Participant membership or public participant state.
func (s *Service) event(ctx context.Context, kind, id string, participant domain.Participant) {
	if events, ok := s.observer.(interface {
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
	}); ok {
		_ = events.Broadcast(ctx, realtime.Event(kind, id, map[string]any{"participant": participant.View()}))
	}
}

// validInvite validates the invitation-code shape before accessing persistence.
//
// Parameters:
//   - code: Bounded public status or failure code.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func validInvite(code string) bool {
	if len(code) != 32 {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(code)
	return err == nil && len(decoded) == 24
}

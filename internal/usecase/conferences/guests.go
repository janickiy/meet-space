package conferences

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type GuestRepository interface {
	// JoinGuest creates a conference-scoped guest membership through a validated invitation.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Bounded public status or failure code.
	//   - argument 3: Name of the setting, cookie, resource or provider being addressed.
	//   - argument 4: Existing guest account identifier eligible to resume this invitation.
	//
	// Returns:
	//   - result 1: The resulting user.
	//   - result 2: The resulting participant.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	JoinGuest(context.Context, string, string, string) (users.User, domain.Participant, error)
}

type GuestTokens interface {
	// IssueGuest issues a short-lived guest JWT restricted to one conference.
	//
	// Parameters:
	//   - argument 1: Acting account identifier used to scope access.
	//   - argument 2: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: Signed guest JWT restricted to the invited conference.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	IssueGuest(string, string) (string, error)
}

type GuestSession struct {
	users.LoginResponse
	Item domain.ParticipantView `json:"item"`
}

type GuestService struct {
	Repository GuestRepository
	Tokens     GuestTokens
	Observer   interface {
		// ConferenceChanged notifies connected sessions or observers of the conference's committed state
		// change.
		//
		// Parameters:
		//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
		//   - argument 2: Conference identifier limiting the operation scope.
		ConferenceChanged(context.Context, string)
	}
}

// Join creates or resumes a guest only after an explicit invitation join.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - code: Bounded public status or failure code.
//   - name: Name of the setting, cookie, resource or provider being addressed.
//   - resumeUserID: Existing guest account identifier eligible to resume this invitation.
//
// Returns:
//   - result 1: The resulting guest session.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *GuestService) Join(ctx context.Context, code, name, resumeUserID string) (GuestSession, error) {
	if !validInvite(code) {
		return GuestSession{}, apperrors.ErrNotFound
	}
	name, err := users.NormalizeDisplayName(name)
	if err != nil {
		return GuestSession{}, err
	}
	user, participant, err := s.Repository.JoinGuest(ctx, code, name, resumeUserID)
	if err != nil {
		return GuestSession{}, err
	}
	token, err := s.Tokens.IssueGuest(user.ID, participant.ConferenceID)
	if err != nil {
		return GuestSession{}, err
	}
	if s.Observer != nil {
		s.Observer.ConferenceChanged(ctx, participant.ConferenceID)
	}
	return GuestSession{LoginResponse: users.LoginResponse{Status: "success", AccessToken: token, TokenType: "Bearer", ExpiresIn: 3600, User: user.View()}, Item: participant.View()}, nil
}

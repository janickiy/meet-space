package conferences

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type InvitationRepository interface {
	// SearchInvitationUsers returns a bounded list of registered invitation candidates visible to
	// the acting organizer.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Search text or validated query filters.
	//
	// Returns:
	//   - result 1: The collected invitation user values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SearchInvitationUsers(context.Context, string, string, string) ([]domain.InvitationUser, error)
	// Invite creates or updates conference invitations for the validated recipients.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Identifier of the account performing the operation.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The collected invitation result values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Invite(context.Context, string, string, domain.InvitationRequest) ([]domain.InvitationResult, error)
}

type InvitationService struct{ Repository InvitationRepository }

// Search searches conference lifecycle and participant membership using bounded input and the
// caller's visibility filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - actorID: Identifier of the account performing the operation.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - query: Search text or validated query filters.
//
// Returns:
//   - result 1: The collected invitation user values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *InvitationService) Search(ctx context.Context, actorID, conferenceID, query string) ([]domain.InvitationUser, error) {
	query = strings.TrimSpace(query)
	queryLength := utf8.RuneCountInString(query)
	if !utf8.ValidString(query) || queryLength < 2 || queryLength > 100 {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "query must contain 2 to 100 characters")
	}
	return s.Repository.SearchInvitationUsers(ctx, actorID, conferenceID, query)
}

// Invite creates or updates conference invitations for the validated recipients.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - actorID: Identifier of the account performing the operation.
//   - conferenceID: Conference identifier limiting the operation scope.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The collected invitation result values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *InvitationService) Invite(ctx context.Context, actorID, conferenceID string, request domain.InvitationRequest) ([]domain.InvitationResult, error) {
	// The limit applies to submitted recipients, before deduplication.
	recipientCount := len(request.Emails) + len(request.UserIDs)
	if recipientCount == 0 || recipientCount > 20 {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "select 1 to 20 recipients")
	}
	normalized := domain.InvitationRequest{}
	seenEmails, seenUserIDs := map[string]bool{}, map[string]bool{}
	for _, email := range request.Emails {
		email = users.NormalizeEmail(email)
		if err := domain.ValidateInvitationEmail(email); err != nil {
			return nil, err
		}
		if !seenEmails[email] {
			normalized.Emails = append(normalized.Emails, email)
			seenEmails[email] = true
		}
	}
	for _, userID := range request.UserIDs {
		parsed, err := uuid.Parse(userID)
		if err != nil || parsed == uuid.Nil {
			return nil, apperrors.ErrInvalidInput
		}
		userID = parsed.String()
		if !seenUserIDs[userID] {
			normalized.UserIDs = append(normalized.UserIDs, userID)
			seenUserIDs[userID] = true
		}
	}
	return s.Repository.Invite(ctx, actorID, conferenceID, normalized)
}

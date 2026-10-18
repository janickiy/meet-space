package chat

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/chat"
)

type SearchRepository interface {
	// SearchMessagePage returns a bounded message-history search page within the authorized chat
	// scope.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference or conversation identifier selected by the repository mode.
	//   - argument 4: Search text or validated query filters.
	//   - argument 5: Opaque continuation boundary from the previous page.
	//   - argument 6: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The resulting page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SearchMessagePage(context.Context, string, string, string, string, int) (domain.Page, error)
	// ChatMessageContext returns the visible history around a requested message within its
	// authorized scope.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference or conversation identifier selected by the repository mode.
	//   - argument 4: Message identifier within the authorized conversation.
	//
	// Returns:
	//   - result 1: The resulting page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ChatMessageContext(context.Context, string, string, string) (domain.Page, error)
}

// SearchService shares the ordinary chat visibility policy without marking results read.
type SearchService struct{ Repo SearchRepository }

// Search searches persistent chat messages and private attachments using bounded input and the
// caller's visibility filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conversation: Conversation identifier limiting the operation scope.
//   - query: Search text or validated query filters.
//   - cursor: Opaque continuation boundary from the previous page.
//   - limit: Maximum number of results or operations in this batch.
//
// Returns:
//   - result 1: The resulting page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *SearchService) Search(ctx context.Context, user, conversation, query, cursor string, limit int) (domain.Page, error) {
	query, err := domain.NormalizeSearchQuery(query)
	if err != nil {
		return domain.Page{}, err
	}
	if limit < 1 || limit > 50 {
		return domain.Page{}, apperrors.ErrInvalidInput
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.Repo.SearchMessagePage(bounded, user, conversation, query, cursor, limit)
}

// Context returns the visible messages surrounding a selected message so clients can navigate to
// a search result.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conversation: Conversation identifier limiting the operation scope.
//   - messageID: Message identifier within the authorized conversation.
//
// Returns:
//   - result 1: The resulting page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *SearchService) Context(ctx context.Context, user, conversation, messageID string) (domain.Page, error) {
	id, err := domain.UUID(messageID)
	if err != nil {
		return domain.Page{}, err
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.Repo.ChatMessageContext(bounded, user, conversation, id)
}

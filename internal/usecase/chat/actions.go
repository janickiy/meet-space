package chat

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	chatdomain "github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

type ActionRepository interface {
	// ChatInfo returns the authenticated member's conference-chat information.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The resulting info.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ChatInfo(context.Context, string, string) (chatdomain.Info, error)
	// ChatMembers returns a cursor-bounded page of members visible in the authorized conference
	// chat.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Opaque continuation boundary from the previous page.
	//   - argument 5: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The collected member view values.
	//   - result 2: Continuation cursor for the next authorized member page.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ChatMembers(context.Context, string, string, string, int) ([]chatdomain.MemberView, string, error)
	// UpdateChatInfo updates authorized conference-chat metadata and returns its member-specific
	// view.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting info.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	UpdateChatInfo(context.Context, string, string, chatdomain.UpdateInfoRequest) (chatdomain.Info, error)
	// ChatPreferences reads the acting member's saved conference-chat notification preferences.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The resulting preferences.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ChatPreferences(context.Context, string, string) (chatdomain.Preferences, error)
	// SetChatPreferences saves only the acting member's conference-chat notification preferences.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Whether the requested feature or preference should be enabled.
	//
	// Returns:
	//   - result 1: The resulting preferences.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SetChatPreferences(context.Context, string, string, bool) (chatdomain.Preferences, error)
	// LeaveChat records the account's departure from the conference chat according to membership
	// rules.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	LeaveChat(context.Context, string, string) error
	// SearchMessages returns a bounded page of visible messages matching the validated search query.
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
	//   - result 1: The resulting message page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SearchMessages(context.Context, string, string, string, string, int) (chatdomain.MessagePage, error)
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
	ChatMessageContext(context.Context, string, string, string) (chatdomain.Page, error)
	// ChatMaterials returns a bounded page of visible message attachments and links.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Validated event, job, media or resource category.
	//   - argument 5: Opaque continuation boundary from the previous page.
	//   - argument 6: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The resulting material page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ChatMaterials(context.Context, string, string, string, string, int) (chatdomain.MaterialPage, error)
	// ChatPins returns the visible pinned messages of the conference chat.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Opaque continuation boundary from the previous page.
	//   - argument 5: Maximum number of results or operations in this batch.
	//
	// Returns:
	//   - result 1: The resulting message page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	ChatPins(context.Context, string, string, string, int) (chatdomain.MessagePage, error)
	// SetChatPin changes the pin state of a visible conference-chat message.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Message identifier within the authorized conversation.
	//   - argument 5: Whether the message should be pinned or marked important.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	SetChatPin(context.Context, string, string, string, bool) error
}

type actionObserver interface {
	// ConferenceChanged notifies connected sessions or observers of the conference's committed state
	// change.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	ConferenceChanged(context.Context, string)
}

type AccountPresence interface {
	// Online returns the currently live presence state for the requested account identifiers.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Account identifiers or account repository, as defined by its type.
	//
	// Returns:
	//   - result 1: The resulting map[string]bool.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Online(context.Context, []string) (map[string]bool, error)
}

type ConferencePresence interface {
	// GetActiveSessions returns the current physical sessions used to authorize media commands.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The collected session values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetActiveSessions(context.Context, string) ([]realtime.Session, error)
}

type ActionService struct {
	repo     ActionRepository
	observer actionObserver
	presence AccountPresence
	guests   ConferencePresence
}

// NewActionService constructs the action service with the dependencies and limits required for
// persistent chat messages and private attachments.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - observer: Observer notified after a persisted lifecycle change.
//
// Returns:
//   - result 1: The constructed or loaded action service.
func NewActionService(repo ActionRepository, observer actionObserver) *ActionService {
	return &ActionService{repo: repo, observer: observer}
}

// WithPresence attaches presence to the component before its operations are served.
//
// Parameters:
//   - accounts: Account presence or account-validation dependency.
//   - guests: Conference-session presence dependency used for guest accounts.
//
// Returns:
//   - result 1: The constructed or loaded action service.
func (s *ActionService) WithPresence(accounts AccountPresence, guests ConferencePresence) *ActionService {
	s.presence, s.guests = accounts, guests
	return s
}

// Info returns the member-specific chat information visible to the current account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: The resulting info.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) Info(ctx context.Context, user, conference string) (chatdomain.Info, error) {
	return s.repo.ChatInfo(ctx, user, conference)
}

// Members returns the current member identifiers or member views for the authorized
// conversation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - cursor: Opaque continuation boundary from the previous page.
//   - limit: Maximum number of results or operations in this batch.
//
// Returns:
//   - result 1: The collected member view values.
//   - result 2: Continuation cursor for the next authorized member page.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) Members(ctx context.Context, user, conference, cursor string, limit int) ([]chatdomain.MemberView, string, error) {
	if limit < 1 || limit > 100 {
		return nil, "", apperrors.ErrInvalidInput
	}
	// Authorization and page bounds precede Redis access: this is not a user directory.
	items, next, err := s.repo.ChatMembers(ctx, user, conference, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if len(items) == 0 {
		return items, next, nil
	}
	ids, hasGuests := []string{}, false
	for _, item := range items {
		if item.UserID == nil {
			continue
		}
		if item.IsGuest {
			hasGuests = true
		} else {
			ids = append(ids, *item.UserID)
		}
	}
	lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	statuses := map[string]bool{}
	accountAvailable := s.presence != nil
	if len(ids) > 0 && accountAvailable {
		statuses, err = s.presence.Online(lookup, ids)
		accountAvailable = err == nil
	}
	guestStatuses := map[string]bool{}
	guestAvailable := s.guests != nil
	if hasGuests && guestAvailable {
		sessions, err := s.guests.GetActiveSessions(lookup, conference)
		guestAvailable = err == nil
		if guestAvailable {
			for _, session := range sessions {
				if session.ConferenceID == conference {
					guestStatuses[session.UserID] = true
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	for i := range items {
		// A presence outage must not turn a valid PG member into a false offline status.
		items[i].Online = nil
		if items[i].UserID == nil {
			continue
		}
		if items[i].IsGuest {
			if guestAvailable {
				online := guestStatuses[*items[i].UserID]
				items[i].Online = &online
			}
		} else if accountAvailable {
			online := statuses[*items[i].UserID]
			items[i].Online = &online
		}
	}
	return items, next, nil
}

// UpdateInfo updates the chat's validated title or description after the management permission
// check.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The resulting info.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) UpdateInfo(ctx context.Context, user, conference string, request chatdomain.UpdateInfoRequest) (chatdomain.Info, error) {
	if request.Title == nil && request.Description == nil {
		return chatdomain.Info{}, apperrors.ErrInvalidInput
	}
	if request.Title != nil {
		title, err := conferences.NormalizeTitle(*request.Title)
		if err != nil {
			return chatdomain.Info{}, err
		}
		request.Title = &title
	}
	if request.Description != nil {
		value := strings.TrimSpace(*request.Description)
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 1000 {
			return chatdomain.Info{}, apperrors.New(apperrors.ErrInvalidInput, "description must be at most 1000 characters")
		}
		request.Description = &value
	}
	info, err := s.repo.UpdateChatInfo(ctx, user, conference, request)
	if err == nil && s.observer != nil {
		s.observer.ConferenceChanged(ctx, conference)
	}
	return info, err
}

// Preferences reads the acting account's saved delivery or conversation preferences.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: The resulting preferences.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) Preferences(ctx context.Context, user, conference string) (chatdomain.Preferences, error) {
	return s.repo.ChatPreferences(ctx, user, conference)
}

// SetPreferences saves the acting account's validated conversation preferences.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - enabled: Whether the requested feature or preference should be enabled.
//
// Returns:
//   - result 1: The resulting preferences.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) SetPreferences(ctx context.Context, user, conference string, enabled bool) (chatdomain.Preferences, error) {
	return s.repo.SetChatPreferences(ctx, user, conference, enabled)
}

// Leave removes the active membership or media connection while preserving the saved
// conversation or conference history.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) Leave(ctx context.Context, user, conference string) error {
	err := s.repo.LeaveChat(ctx, user, conference)
	if err == nil && s.observer != nil {
		s.observer.ConferenceChanged(ctx, conference)
	}
	return err
}

// Search searches persistent chat messages and private attachments using bounded input and the
// caller's visibility filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - query: Search text or validated query filters.
//   - cursor: Opaque continuation boundary from the previous page.
//   - limit: Maximum number of results or operations in this batch.
//
// Returns:
//   - result 1: The resulting message page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) Search(ctx context.Context, user, conference, query, cursor string, limit int) (chatdomain.MessagePage, error) {
	query = strings.TrimSpace(query)
	queryLength := utf8.RuneCountInString(query)
	if !utf8.ValidString(query) || queryLength < 2 || queryLength > 200 {
		return chatdomain.MessagePage{}, apperrors.New(apperrors.ErrInvalidInput, "query must contain 2 to 200 characters")
	}
	if limit < 1 || limit > 50 {
		return chatdomain.MessagePage{}, apperrors.ErrInvalidInput
	}
	return s.repo.SearchMessages(ctx, user, conference, query, cursor, limit)
}

// Context returns the visible messages surrounding a selected message so clients can navigate to
// a search result.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - messageID: Message identifier within the authorized conversation.
//
// Returns:
//   - result 1: The resulting page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) Context(ctx context.Context, user, conference, messageID string) (chatdomain.Page, error) {
	id, err := chatdomain.UUID(messageID)
	if err != nil {
		return chatdomain.Page{}, err
	}
	return s.repo.ChatMessageContext(ctx, user, conference, id)
}

// Materials returns a bounded page of visible conversation attachments and shared links.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - kind: Validated event, job, media or resource category.
//   - cursor: Opaque continuation boundary from the previous page.
//   - limit: Maximum number of results or operations in this batch.
//
// Returns:
//   - result 1: The resulting material page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) Materials(ctx context.Context, user, conference, kind, cursor string, limit int) (chatdomain.MaterialPage, error) {
	if kind != "image" && kind != "file" && kind != "link" {
		return chatdomain.MaterialPage{}, apperrors.New(apperrors.ErrInvalidInput, "kind must be image, file or link")
	}
	if limit < 1 || limit > 50 {
		return chatdomain.MaterialPage{}, apperrors.ErrInvalidInput
	}
	return s.repo.ChatMaterials(ctx, user, conference, kind, cursor, limit)
}

// Pins returns a bounded page of the conversation's pinned messages.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - cursor: Opaque continuation boundary from the previous page.
//   - limit: Maximum number of results or operations in this batch.
//
// Returns:
//   - result 1: The resulting message page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) Pins(ctx context.Context, user, conference, cursor string, limit int) (chatdomain.MessagePage, error) {
	if limit < 1 || limit > 50 {
		return chatdomain.MessagePage{}, apperrors.ErrInvalidInput
	}
	return s.repo.ChatPins(ctx, user, conference, cursor, limit)
}

// SetPin updates pin for persistent chat messages and private attachments.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - conference: Conference identifier limiting the operation scope.
//   - messageID: Message identifier within the authorized conversation.
//   - important: Whether the message should be pinned or marked important.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *ActionService) SetPin(ctx context.Context, user, conference, messageID string, important bool) error {
	id, err := chatdomain.UUID(messageID)
	if err != nil {
		return err
	}
	return s.repo.SetChatPin(ctx, user, conference, id, important)
}

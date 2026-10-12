package chat

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/chat"
)

type SearchRepository interface {
	SearchMessagePage(context.Context, string, string, string, string, int) (domain.Page, error)
	ChatMessageContext(context.Context, string, string, string) (domain.Page, error)
}

// SearchService shares the ordinary chat visibility policy without marking results read.
type SearchService struct{ Repo SearchRepository }

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

func (s *SearchService) Context(ctx context.Context, user, conversation, messageID string) (domain.Page, error) {
	id, err := domain.UUID(messageID)
	if err != nil {
		return domain.Page{}, err
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.Repo.ChatMessageContext(bounded, user, conversation, id)
}

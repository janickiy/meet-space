package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/chat"
)

type searchProbe struct {
	called   bool
	query    string
	cursor   string
	limit    int
	deadline time.Time
}

func (r *searchProbe) SearchMessagePage(ctx context.Context, _, _, query, cursor string, limit int) (domain.Page, error) {
	r.called, r.query, r.cursor, r.limit = true, query, cursor, limit
	r.deadline, _ = ctx.Deadline()
	return domain.Page{Items: []domain.Message{}, UnreadCount: 7}, nil
}
func (r *searchProbe) ChatMessageContext(ctx context.Context, _, _, _ string) (domain.Page, error) {
	r.called = true
	r.deadline, _ = ctx.Deadline()
	return domain.Page{Items: []domain.Message{}}, nil
}

func TestMessageSearchValidationBeforeRepository(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		limit       int
	}{
		{"empty", " \n ", 20}, {"single rune", "я", 20}, {"single emoji", "😀", 20},
		{"too long unicode", strings.Repeat("я", 201), 20}, {"invalid UTF8", string([]byte{0xff, 0xfe}), 20},
		{"nul", "ab\x00cd", 20}, {"zero limit", "text", 0}, {"large limit", "text", 51},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &searchProbe{}
			_, err := (&SearchService{Repo: repo}).Search(context.Background(), "user", "chat", tc.query, "", tc.limit)
			if !errors.Is(err, apperrors.ErrInvalidInput) || repo.called {
				t.Fatalf("invalid request reached repository or wrong error: %v", err)
			}
		})
	}
}

func TestMessageSearchPreservesLiteralUnicodeAndDeadline(t *testing.T) {
	for _, query := range []string{"😀😀", strings.Repeat("я", 200), "100%_!\\", "ПрИвЕт"} {
		repo := &searchProbe{}
		page, err := (&SearchService{Repo: repo}).Search(context.Background(), "user", "chat", " \n"+query+"\t ", "cursor", 20)
		if err != nil || repo.query != query || repo.cursor != "cursor" || repo.limit != 20 || page.UnreadCount != 7 {
			t.Fatalf("literal query changed or page lost: %v", err)
		}
		if remaining := time.Until(repo.deadline); remaining <= 0 || remaining > 5*time.Second {
			t.Fatalf("unbounded database work: %v", remaining)
		}
	}
}

func TestMessageContextValidatesTargetAndPreservesShorterDeadline(t *testing.T) {
	repo := &searchProbe{}
	service := &SearchService{Repo: repo}
	if _, err := service.Context(context.Background(), "user", "chat", "not-a-uuid"); !errors.Is(err, apperrors.ErrInvalidInput) || repo.called {
		t.Fatal("invalid target reached repository")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	expected, _ := ctx.Deadline()
	if _, err := service.Context(ctx, "user", "chat", uuid.NewString()); err != nil || !repo.called || !repo.deadline.Equal(expected) {
		t.Fatalf("parent deadline was not preserved: %v", err)
	}
}

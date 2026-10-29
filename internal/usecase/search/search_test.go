package search

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/search"
)

type searchRepo struct {
	calls int
	user  string
	query domain.Query
}

func (r *searchRepo) Search(_ context.Context, user string, q domain.Query) (domain.Page, error) {
	r.calls++
	r.user = user
	r.query = q
	return domain.Page{Items: []domain.Result{}, Limit: q.Limit, Offset: q.Offset}, nil
}

func TestKeywordSearchNormalizesAndPreservesFilters(t *testing.T) {
	repo := &searchRepo{}
	user, conference, participant := uuid.NewString(), uuid.NewString(), uuid.NewString()
	from := time.Now().UTC()
	to := from.Add(time.Hour)
	query := domain.Query{Query: "  План встречи  ", ConferenceID: conference, ParticipantID: participant, Membership: "owned", From: &from, To: &to, Limit: 20, Offset: 5}
	page, err := (&Service{Repo: repo}).Search(context.Background(), user, query)
	if err != nil || repo.calls != 1 || repo.user != user || repo.query.Query != "План встречи" || repo.query.Mode != "keyword" || repo.query.Source != "all" {
		t.Fatalf("normalized query=%+v, calls=%d, error=%v", repo.query, repo.calls, err)
	}
	if repo.query.ConferenceID != conference || repo.query.ParticipantID != participant || repo.query.Membership != "owned" || repo.query.From != &from || repo.query.To != &to || page.EffectiveMode != "keyword" || page.Limit != 20 || page.Offset != 5 {
		t.Fatalf("lost filters or pagination: query=%+v page=%+v", repo.query, page)
	}
}

func TestSearchRejectsRemovedFeaturesAndInvalidFilters(t *testing.T) {
	cases := map[string]func(*domain.Query){
		"semantic":        func(q *domain.Query) { q.Mode = "semantic" },
		"hybrid":          func(q *domain.Query) { q.Mode = "hybrid" },
		"transcript":      func(q *domain.Query) { q.Source = "transcript" },
		"summary":         func(q *domain.Query) { q.Source = "summary" },
		"empty":           func(q *domain.Query) { q.Query = "  " },
		"long":            func(q *domain.Query) { q.Query = strings.Repeat("я", 501) },
		"nul":             func(q *domain.Query) { q.Query = "a\x00b" },
		"invalid utf8":    func(q *domain.Query) { q.Query = string([]byte{0xff}) },
		"limit zero":      func(q *domain.Query) { q.Limit = 0 },
		"limit large":     func(q *domain.Query) { q.Limit = 101 },
		"offset negative": func(q *domain.Query) { q.Offset = -1 },
		"offset large":    func(q *domain.Query) { q.Offset = 10001 },
		"membership":      func(q *domain.Query) { q.Membership = "other" },
		"participant":     func(q *domain.Query) { q.ParticipantID = "invalid" },
		"conference":      func(q *domain.Query) { q.ConferenceID = uuid.Nil.String() },
		"date order":      func(q *domain.Query) { a := time.Now(); b := a.Add(-time.Hour); q.From = &a; q.To = &b },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &searchRepo{}
			q := domain.Query{Query: "meeting", Limit: 20}
			mutate(&q)
			_, err := (&Service{Repo: repo}).Search(context.Background(), uuid.NewString(), q)
			if !errors.Is(err, apperrors.ErrInvalidInput) || repo.calls != 0 {
				t.Fatalf("error=%v calls=%d", err, repo.calls)
			}
		})
	}
}

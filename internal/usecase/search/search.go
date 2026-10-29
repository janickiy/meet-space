// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Package search validates conference-title queries with repository-level authorization.
package search

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	apperrors "github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/search"
)

type Repository interface {
	Search(context.Context, string, domain.Query) (domain.Page, error)
}

type Service struct{ Repo Repository }

// Search validates bounded keyword filters before querying authorized meetings.
func (s *Service) Search(ctx context.Context, userID string, query domain.Query) (domain.Page, error) {
	if query.Mode == "" {
		query.Mode = "keyword"
	}
	if query.Source == "" {
		query.Source = "all"
	}
	if query.Mode != "keyword" || (query.Source != "all" && query.Source != "conference") {
		return domain.Page{}, apperrors.ErrInvalidInput
	}
	if query.Membership != "" && query.Membership != "all" && query.Membership != "owned" && query.Membership != "participating" {
		return domain.Page{}, apperrors.ErrInvalidInput
	}
	for _, field := range []*string{&query.ConferenceID, &query.ParticipantID} {
		if *field == "" {
			continue
		}
		id, err := uuid.Parse(*field)
		if err != nil || id == uuid.Nil {
			return domain.Page{}, apperrors.ErrInvalidInput
		}
		*field = id.String()
	}
	query.Query = strings.TrimSpace(query.Query)
	if query.Query == "" || !utf8.ValidString(query.Query) || utf8.RuneCountInString(query.Query) > 500 || len(query.Query) > 2000 ||
		strings.ContainsRune(query.Query, 0) ||
		query.Limit < 1 || query.Limit > 100 || query.Offset < 0 || query.Offset > 10000 {
		return domain.Page{}, apperrors.ErrInvalidInput
	}
	if (query.From != nil && query.From.IsZero()) || (query.To != nil && query.To.IsZero()) || (query.From != nil && query.To != nil && query.From.After(*query.To)) {
		return domain.Page{}, apperrors.ErrInvalidInput
	}
	page, err := s.Repo.Search(ctx, userID, query)
	page.EffectiveMode = "keyword"
	return page, err
}

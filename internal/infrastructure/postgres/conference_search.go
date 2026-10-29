// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"context"
	domain "github.com/janickiy/meet-space/internal/domain/search"
	"gorm.io/gorm"
)

type ConferenceSearchRepository struct{ db *gorm.DB }

func NewConferenceSearchRepository(db *gorm.DB) *ConferenceSearchRepository {
	return &ConferenceSearchRepository{db: db}
}

// Membership authorization is applied before ranking and pagination.
const conferenceSearchSQL = `WITH searchquery AS (SELECT websearch_to_tsquery('simple'::regconfig,?) AS query),
 permitted AS (SELECT c.* FROM conferences c WHERE c.kind='meeting' AND
 EXISTS(SELECT 1 FROM conference_participants p WHERE p.conference_id=c.id AND p.user_id=? AND p.admission_state='admitted' AND p.status IN ('joined','left'))
 AND (?='' OR c.id::text=?)
 AND (? IN ('','all') OR (?='owned' AND c.owner_id::text=?) OR (?='participating' AND c.owner_id::text<>?))
 AND (?='' OR EXISTS(SELECT 1 FROM conference_participants fp WHERE fp.conference_id=c.id AND fp.id::text=?))
 AND (?::timestamptz IS NULL OR COALESCE(c.finished_at,c.scheduled_at,c.created_at)>=?::timestamptz)
 AND (?::timestamptz IS NULL OR COALESCE(c.finished_at,c.scheduled_at,c.created_at)<=?::timestamptz)),
 matches AS (
 SELECT 'conference'::text AS type,c.id AS conference_id,c.title AS conference_title,
 left(c.title,240) AS snippet,ts_rank(c.search_vector,q.query) AS rank
 FROM permitted c CROSS JOIN searchquery q WHERE c.search_vector@@q.query) `

func (r *ConferenceSearchRepository) Search(ctx context.Context, userID string, query domain.Query) (domain.Page, error) {
	page := domain.Page{Items: []domain.Result{}, Limit: query.Limit, Offset: query.Offset}
	args := []any{query.Query, userID, query.ConferenceID, query.ConferenceID, query.Membership, query.Membership, userID, query.Membership, userID, query.ParticipantID, query.ParticipantID, query.From, query.From, query.To, query.To}
	db := r.db.WithContext(ctx)
	if err := db.Raw(conferenceSearchSQL+"SELECT count(*) FROM matches", args...).Scan(&page.Total).Error; err != nil {
		return page, err
	}
	args = append(args, query.Limit, query.Offset)
	err := db.Raw(conferenceSearchSQL+"SELECT * FROM matches ORDER BY rank DESC,conference_id LIMIT ? OFFSET ?", args...).Scan(&page.Items).Error
	return page, err
}

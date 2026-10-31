package recordings

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
)

type libraryRepository interface {
	Library(context.Context, string, records.LibraryQuery) ([]records.LibraryRecord, error)
}

// Library reads one authorized page directly from recordings, including records
// whose meetings are still active or absent from the meeting/chat timeline.
func (s *Service) Library(ctx context.Context, userID string, query records.LibraryQuery) (records.LibraryPage, error) {
	page := records.LibraryPage{Items: []records.LibraryItem{}}
	if err := query.Validate(); err != nil {
		return page, err
	}
	repository, ok := s.repo.(libraryRepository)
	if !ok {
		return page, apperrors.ErrUnavailable
	}
	rows, err := repository.Library(ctx, userID, query)
	if err != nil {
		return page, err
	}
	if len(rows) > query.Limit {
		rows = rows[:query.Limit]
		cursor := records.EncodeLibraryCursor(rows[len(rows)-1].Recording)
		page.NextCursor = &cursor
	}
	// The existing reader signs the same file artifacts returned by Read/List.
	// A bounded batch avoids a detail request per row when supported by the reader.
	cards := make(map[string]records.RecordCard, len(rows))
	if reader, ok := s.reader.(interface {
		ReadComposites(context.Context, []string) ([]records.RecordCard, error)
	}); ok && len(rows) > 0 {
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.Recording.UUID)
		}
		items, err := reader.ReadComposites(ctx, ids)
		if err != nil {
			return page, err
		}
		for _, item := range items {
			cards[item.UUID] = item
		}
	} else {
		for _, row := range rows {
			item, err := s.card(ctx, row.Recording.UUID)
			if err != nil {
				return page, err
			}
			cards[item.UUID] = item
		}
	}
	for _, row := range rows {
		card, ok := cards[row.Recording.UUID]
		if !ok || card.ConferenceID != row.Conference.ID {
			return page, apperrors.ErrNotFound
		}
		card.Status = records.PublicStatus(card.Status)
		conference := row.Conference.View()
		conference.InviteCode, conference.InviteURL = "", ""
		page.Items = append(page.Items, records.LibraryItem{Conference: conference, Recording: card})
	}
	return page, nil
}

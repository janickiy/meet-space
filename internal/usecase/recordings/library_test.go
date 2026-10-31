package recordings

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/records"
)

type libraryTestRepository struct {
	Repository
	rows  []records.LibraryRecord
	actor string
	query records.LibraryQuery
	calls int
	err   error
}

func (r *libraryTestRepository) Library(_ context.Context, actor string, query records.LibraryQuery) ([]records.LibraryRecord, error) {
	r.actor, r.query = actor, query
	r.calls++
	return r.rows, r.err
}

type libraryTestReader struct {
	cards []records.RecordCard
	ids   []string
}

func (r *libraryTestReader) ReadComposite(context.Context, string) (records.RecordCard, error) {
	panic("batch expected")
}
func (r *libraryTestReader) ReadComposites(_ context.Context, ids []string) ([]records.RecordCard, error) {
	r.ids = ids
	return r.cards, nil
}

func TestLibraryUsesAuthorizedBatchAndKeepsRecordOrder(t *testing.T) {
	at := time.Now().UTC()
	r := &libraryTestRepository{}
	reader := &libraryTestReader{}
	for i, status := range []conferences.Status{conferences.Active, conferences.Finished, conferences.Cancelled} {
		id := []string{"new", "old", "extra"}[i]
		record := records.Record{ID: int64(3 - i), UUID: id, ConferenceID: id + "-meeting", CreatedAt: at}
		r.rows = append(r.rows, records.LibraryRecord{Recording: record, Conference: conferences.Conference{ID: record.ConferenceID, Title: id, Status: status, InviteCode: "private-invitation"}})
		reader.cards = append([]records.RecordCard{{Record: record, Files: []records.RecordFileView{{URL: "https://storage.invalid/signed-preview"}}}}, reader.cards...)
	}
	// A compliant batch reader returns only requested records, but its result order is not relied on.
	reader.cards = reader.cards[1:]
	page, err := NewConferenceService(r, reader, nil, nil).Library(context.Background(), "account", records.LibraryQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if r.actor != "account" || r.query.Limit != 2 || len(reader.ids) != 2 || reader.ids[0] != "new" || reader.ids[1] != "old" {
		t.Fatalf("scope/batch: %+v %v", r, reader.ids)
	}
	if len(page.Items) != 2 || page.Items[0].Recording.UUID != "new" || page.Items[1].Recording.UUID != "old" {
		t.Fatalf("order: %+v", page)
	}
	for _, item := range page.Items {
		if item.Conference.InviteCode != "" || item.Conference.InviteURL != "" || len(item.Recording.Files) != 1 {
			t.Fatalf("safe DTO/artifacts: %+v", item)
		}
	}
	if page.NextCursor == nil {
		t.Fatal("missing continuation")
	}
	cursor, err := records.DecodeLibraryCursor(*page.NextCursor)
	if err != nil || cursor.ID != 2 || !cursor.At.Equal(at) {
		t.Fatalf("wrong cursor: %+v %v", cursor, err)
	}
	r.rows = r.rows[:2]
	last, err := NewConferenceService(r, reader, nil, nil).Library(context.Background(), "account", records.LibraryQuery{Limit: 2})
	if err != nil || last.NextCursor != nil || len(last.Items) != 2 {
		t.Fatalf("exact final page must not claim another page: %+v %v", last, err)
	}
}

func TestLibraryEmptyAndRejectedRequests(t *testing.T) {
	r := &libraryTestRepository{}
	s := NewConferenceService(r, &libraryTestReader{}, nil, nil)
	page, err := s.Library(context.Background(), "account", records.LibraryQuery{Limit: 30})
	if err != nil || page.Items == nil || len(page.Items) != 0 || page.NextCursor != nil {
		t.Fatalf("empty page: %+v %v", page, err)
	}
	if _, err := s.Library(context.Background(), "account", records.LibraryQuery{Limit: 30, Cursor: "invalid!"}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("cursor error: %v", err)
	}
	if r.calls != 1 {
		t.Fatalf("invalid cursor reached repository: %d", r.calls)
	}
	r.err = apperrors.ErrForbidden
	if _, err := s.Library(context.Background(), "account", records.LibraryQuery{Limit: 30}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal(err)
	}
}

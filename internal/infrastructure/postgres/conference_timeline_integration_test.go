package postgres

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// TestAuditTimelineSorting checks real PostgreSQL pagination, ties, date filters and access.
func TestAuditTimelineSorting(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	actor, outsider := uuid.NewString(), uuid.NewString()
	for _, id := range []string{actor, outsider} {
		if err := db.Create(&users.User{ID: id, Email: id + "@example.invalid", PasswordHash: "synthetic"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := NewConferenceRepository(db)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for viewIndex, view := range []string{"upcoming", "active", "past"} {
		meetings := make([]conferences.Conference, 9)
		for i := range meetings {
			at := base.Add(time.Duration(i/2) * time.Hour)
			meeting := conferences.Conference{ID: fmt.Sprintf("00000000-0000-4000-8000-%012d", viewIndex*100+i+1), OwnerID: actor, Kind: "meeting", Title: view, InviteCode: strings.ReplaceAll(uuid.NewString(), "-", ""), Status: conferences.Created, CreatedAt: at, UpdatedAt: at}
			switch view {
			case "upcoming":
				// Alternate undated and scheduled rows while keeping a shared effective date.
				if i%2 == 0 {
					meeting.Status, meeting.ScheduledAt = conferences.Scheduled, &at
					meeting.CreatedAt = base.Add(-24 * time.Hour)
				}
			case "active":
				started := at.Add(24 * time.Hour)
				meeting.Status, meeting.StartedAt = conferences.Active, &started
			case "past":
				started := at.Add(-time.Hour)
				meeting.Status, meeting.StartedAt, meeting.FinishedAt = conferences.Finished, &started, &at
				meeting.CreatedAt = base.Add(-48 * time.Hour)
			}
			if err := db.Create(&meeting).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,admission_state,joined_at) VALUES(?,?,?,'Synthetic user','owner','joined','admitted',?)", uuid.NewString(), meeting.ID, actor, at).Error; err != nil {
				t.Fatal(err)
			}
			meetings[i] = meeting
		}
		for _, order := range []string{"", "newest", "oldest"} {
			t.Run(view+"/"+order, func(t *testing.T) {
				expected := slices.Clone(meetings)
				sort.Slice(expected, func(i, j int) bool {
					a, _ := conferences.DecodeTimelineCursor(conferences.EncodeTimelineCursor(expected[i], order))
					b, _ := conferences.DecodeTimelineCursor(conferences.EncodeTimelineCursor(expected[j], order))
					less := a.At.Before(b.At) || (a.At.Equal(b.At) && a.ID < b.ID)
					if order == "oldest" {
						return less
					}
					return a.At.After(b.At) || (a.At.Equal(b.At) && a.ID > b.ID)
				})
				query := conferences.TimelineQuery{View: view, Scope: "all", Sort: order, Limit: 3}
				var got []string
				for page := 0; page < 5; page++ {
					rows, err := repo.Timeline(ctx, actor, query)
					if err != nil {
						t.Fatal(err)
					}
					hasMore := len(rows) > query.Limit
					if hasMore {
						rows = rows[:query.Limit]
					}
					for _, row := range rows {
						got = append(got, row.ID)
					}
					if !hasMore {
						break
					}
					query.Cursor = conferences.EncodeTimelineCursor(rows[len(rows)-1], order)
				}
				want := make([]string, len(expected))
				for i, meeting := range expected {
					want[i] = meeting.ID
				}
				if !slices.Equal(got, want) {
					t.Fatalf("paginated IDs=%v want=%v", got, want)
				}
				from, to := base.Add(time.Hour), base.Add(2*time.Hour)
				query.Cursor, query.From, query.To, query.Limit = "", &from, &to, 100
				rows, err := repo.Timeline(ctx, actor, query)
				if err != nil || len(rows) != 4 {
					t.Fatalf("bounded date results=%d error=%v", len(rows), err)
				}
				for _, row := range rows {
					cursor, _ := conferences.DecodeTimelineCursor(conferences.EncodeTimelineCursor(row, order))
					if cursor.At.Before(from) || cursor.At.After(to) {
						t.Fatal("date filter did not apply before pagination")
					}
				}
				rows, err = repo.Timeline(ctx, outsider, query)
				if err != nil || len(rows) != 0 {
					t.Fatalf("outsider results=%d error=%v", len(rows), err)
				}
			})
		}
	}
}

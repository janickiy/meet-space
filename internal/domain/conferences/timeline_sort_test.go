package conferences

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

// TestTimelineSortValidation preserves the legacy order and rejects unsafe sorts.
func TestTimelineSortValidation(t *testing.T) {
	for _, sort := range []string{"", "newest", "oldest", "NEWEST", "ascending", "oldest;DROP TABLE conferences"} {
		query := TimelineQuery{View: "past", Scope: "all", Limit: 20, Sort: sort}
		valid := sort == "" || sort == "newest" || sort == "oldest"
		if err := query.Validate(); (err == nil) != valid {
			t.Errorf("sort=%q error=%v valid=%v", sort, err, valid)
		}
	}
}

// TestTimelineCursorDirection binds pagination to its order without rejecting old clients.
func TestTimelineCursorDirection(t *testing.T) {
	meeting := Conference{ID: "00000000-0000-4000-8000-000000000001", CreatedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	for _, cursorSort := range []string{"", "newest", "oldest"} {
		cursor := EncodeTimelineCursor(meeting, cursorSort)
		for _, querySort := range []string{"", "newest", "oldest"} {
			query := TimelineQuery{View: "past", Scope: "all", Limit: 20, Sort: querySort, Cursor: cursor}
			valid := (cursorSort == "oldest") == (querySort == "oldest")
			if err := query.Validate(); (err == nil) != valid {
				t.Errorf("cursor sort=%q query sort=%q error=%v valid=%v", cursorSort, querySort, err, valid)
			}
		}
	}
	data, err := json.Marshal(TimelineCursor{At: meeting.CreatedAt, ID: meeting.ID, Sort: "arbitrary"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeTimelineCursor(base64.RawURLEncoding.EncodeToString(data)); err == nil {
		t.Fatal("accepted a cursor with an unknown order")
	}
}

// TestTimelineCursorDate verifies the exact timestamp priority used by SQL ordering.
func TestTimelineCursorDate(t *testing.T) {
	created := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	scheduled, finished := created.Add(time.Hour), created.Add(2*time.Hour)
	meeting := Conference{ID: "00000000-0000-4000-8000-000000000001", CreatedAt: created}
	for _, expected := range []time.Time{created, scheduled, finished} {
		if expected.Equal(scheduled) {
			meeting.ScheduledAt = &scheduled
		}
		if expected.Equal(finished) {
			meeting.FinishedAt = &finished
		}
		cursor, err := DecodeTimelineCursor(EncodeTimelineCursor(meeting, "oldest"))
		if err != nil || cursor == nil || !cursor.At.Equal(expected) || cursor.ID != meeting.ID || cursor.Sort != "oldest" {
			t.Fatalf("expected=%v cursor=%+v error=%v", expected, cursor, err)
		}
	}
}

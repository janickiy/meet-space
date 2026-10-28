package conferencesapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/conferences"
)

type timelineCapture struct {
	Service
	called bool
	query  conferences.TimelineQuery
}

// Timeline records the validated filters without invoking other service methods.
func (s *timelineCapture) Timeline(_ context.Context, _ string, query conferences.TimelineQuery) (conferences.TimelinePage, error) {
	s.called, s.query = true, query
	return conferences.TimelinePage{Items: []conferences.View{}}, nil
}

// TestTimelineSortQuery verifies HTTP defaults, explicit orders and cursor validation.
func TestTimelineSortQuery(t *testing.T) {
	meeting := conferences.Conference{ID: "00000000-0000-4000-8000-000000000001", CreatedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	oldestCursor := conferences.EncodeTimelineCursor(meeting, "oldest")
	for _, tc := range []struct {
		name  string
		query string
		want  string
		code  int
	}{
		{name: "default", want: "newest", code: http.StatusOK},
		{name: "newest", query: "sort=newest", want: "newest", code: http.StatusOK},
		{name: "oldest", query: "sort=oldest", want: "oldest", code: http.StatusOK},
		{name: "oldest_next_page", query: "sort=oldest&cursor=" + url.QueryEscape(oldestCursor), want: "oldest", code: http.StatusOK},
		{name: "unsupported", query: "sort=descending", code: http.StatusUnprocessableEntity},
		{name: "wrong_cursor_order", query: "sort=newest&cursor=" + url.QueryEscape(oldestCursor), code: http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/me/conferences?"+tc.query, nil)
			service := &timelineCapture{}
			NewHandler(service).Timeline(c)
			if response.Code != tc.code {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.code, response.Body.String())
			}
			if tc.code == http.StatusOK {
				if !service.called || service.query.Sort != tc.want {
					t.Fatalf("service called=%v sort=%q want=%q", service.called, service.query.Sort, tc.want)
				}
			} else if service.called {
				t.Fatal("invalid query reached the service")
			}
		})
	}
}

package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	recordingsapp "github.com/janickiy/meet-space/internal/app/recordings"
	"github.com/janickiy/meet-space/internal/domain/records"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type recordingLibraryService struct {
	recordingsapp.Service
	actor string
	query records.LibraryQuery
	calls int
}

func (s *recordingLibraryService) Library(_ context.Context, actor string, q records.LibraryQuery) (records.LibraryPage, error) {
	s.actor, s.query = actor, q
	s.calls++
	return records.LibraryPage{Items: []records.LibraryItem{}}, nil
}

type recordingLibraryTokens struct{}

func (recordingLibraryTokens) Verify(token string) (string, error) {
	if token == "synthetic-valid" {
		return "account", nil
	}
	return "", errors.New("invalid")
}

func (recordingLibraryTokens) VerifySession(token string) (string, string, time.Time, error) {
	if token == "synthetic-valid" {
		return "account", "", time.Now().Add(time.Hour), nil
	}
	if token == "synthetic-guest" {
		return "guest", "guest-meeting", time.Now().Add(time.Hour), nil
	}
	return "", "", time.Time{}, errors.New("invalid")
}

func TestRecordingLibraryRouteIsAuthenticatedAndHasExplicitEndCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &recordingLibraryService{}
	r := gin.New()
	RegisterConferenceRecordingRoutes(r, recordingsapp.NewHandler(s), httpmiddleware.Authenticate(recordingLibraryTokens{}))
	request := func(path string, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if authorized {
			req.Header.Set("Authorization", "Bearer synthetic-valid")
		}
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		return res
	}
	if got := request("/api/v1/recordings", false); got.Code != http.StatusUnauthorized || s.calls != 0 {
		t.Fatalf("unauthorized: %d %s", got.Code, got.Body.String())
	}
	got := request("/api/v1/recordings", true)
	if got.Code != http.StatusOK || s.actor != "account" || s.query.Limit != 30 {
		t.Fatalf("default page: %d %s %+v", got.Code, got.Body.String(), s)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if string(body["nextCursor"]) != "null" || string(body["items"]) != "[]" {
		t.Fatalf("empty/end contract: %s", got.Body.String())
	}
	for _, suffix := range []string{"?cursor=bad!", "?limit=0", "?limit=101", "?limit=", "?limit=x", "?limit=30&limit=20", "?cursor=&cursor=other"} {
		if got := request("/api/v1/recordings"+suffix, true); got.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid query %q returned %d %s", suffix, got.Code, got.Body.String())
		}
	}
	if s.calls != 1 {
		t.Fatalf("invalid requests reached service: %d", s.calls)
	}
	guest := httptest.NewRequest(http.MethodGet, "/api/v1/recordings", nil)
	guest.Header.Set("Authorization", "Bearer synthetic-guest")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, guest)
	if response.Code != http.StatusForbidden || s.calls != 1 {
		t.Fatalf("guest escaped meeting scope: %d %s", response.Code, response.Body.String())
	}
}

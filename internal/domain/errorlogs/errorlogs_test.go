package errorlogs

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSanitizeDropsRawDiagnosticsAndBoundsFrames(t *testing.T) {
	secret := "private_password_and_SQL_payload"
	e := Sanitize(Event{Service: "api", Instance: "instance", Level: "ERROR", Code: "unrecognized." + secret, Message: secret, Route: "/api/v1/reset?token=" + secret, RequestID: secret, Release: "release-1", Stack: []Frame{{File: "https://private.invalid/" + secret, Line: 1, Column: 1}, {File: "index-abcdefgh123.js", Line: 3, Column: 9}}})
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), secret) || e.Message != "Ошибка компонента" || e.Code != "component.error" || e.Route != "unknown" || e.RequestID != "" || len(e.Stack) != 1 {
		t.Fatalf("unsafe or incorrect projection: %s", raw)
	}
	if e.Count != 1 || len(e.Fingerprint) != 64 {
		t.Fatal("invalid bounded aggregate")
	}
	same := Sanitize(Event{Service: "api", Instance: "another", Code: "component.error", Release: "release-1", Route: "unknown"})
	if same.Fingerprint != e.Fingerprint {
		t.Fatal("instance should not fragment fingerprints")
	}
	distinct := Sanitize(Event{Service: "api", Code: "go.error.0123456789ab", Release: "release-1", Route: "unknown"})
	if distinct.Code != "go.error.0123456789ab" || distinct.Fingerprint == e.Fingerprint {
		t.Fatal("safe caller digest lost")
	}
}
func TestErrorQueryAndCursorValidation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	e := Event{ID: uuid.NewString(), LastSeen: now}
	cursor := EncodeCursor(e)
	decoded, err := DecodeCursor(cursor)
	if err != nil || decoded.ID != e.ID || !decoded.LastSeen.Equal(now) {
		t.Fatal("cursor round trip", err)
	}
	valid := Query{Service: "admin-frontend", Level: "WARN", Before: cursor, From: now.Add(-time.Hour).Format(time.RFC3339Nano), To: now.Format(time.RFC3339Nano)}
	q, err := ValidateQuery(valid)
	if err != nil || q.PageSize != 25 {
		t.Fatal("valid query", err)
	}
	for _, bad := range []Query{{PageSize: 101}, {PageSize: -1}, {Service: "unexpected"}, {Level: "INFO"}, {RequestID: "private_token"}, {Code: "sql select *"}, {Before: "not_cursor"}, {From: "no_date"}, {From: now.Format(time.RFC3339), To: now.Add(-time.Hour).Format(time.RFC3339)}} {
		if _, err := ValidateQuery(bad); err == nil {
			t.Fatalf("invalid query accepted: %+v", bad)
		}
	}
}

func TestUUIDsAreCanonicalForBoundedStorage(t *testing.T) {
	id := uuid.New()
	urn := id.URN()
	e := Sanitize(Event{ID: urn, RequestID: urn, Code: "http.5xx"})
	if e.ID != id.String() || e.RequestID != id.String() {
		t.Fatal("UUID not canonicalized")
	}
	q, err := ValidateQuery(Query{RequestID: urn})
	if err != nil || q.RequestID != id.String() {
		t.Fatal("query UUID not canonicalized")
	}
	decoded, err := DecodeCursor(EncodeCursor(Event{ID: urn, LastSeen: time.Now()}))
	if err != nil || decoded.ID != id.String() {
		t.Fatal("cursor UUID not canonicalized")
	}
}

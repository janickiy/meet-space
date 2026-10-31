package records

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLibraryCursorRoundTripAndValidation(t *testing.T) {
	at := time.Date(2026, 10, 27, 0, 27, 1, 123456000, time.UTC)
	encoded := EncodeLibraryCursor(Record{ID: 42, CreatedAt: at})
	cursor, err := DecodeLibraryCursor(encoded)
	if err != nil || cursor.ID != 42 || !cursor.At.Equal(at) {
		t.Fatalf("cursor roundtrip: %+v %v", cursor, err)
	}
	if cursor, err := DecodeLibraryCursor(""); cursor != nil || err != nil {
		t.Fatalf("empty cursor: %+v %v", cursor, err)
	}
	encode := func(raw string) string { return base64.RawURLEncoding.EncodeToString([]byte(raw)) }
	for name, raw := range map[string]string{
		"malformed": "bad!", "oversized": strings.Repeat("a", 257), "padding": encoded + "=",
		"line break": encoded + "\n", "missing id": encode(`{"at":"2026-10-27T00:27:00Z"}`),
		"zero id":       encode(`{"at":"2026-10-27T00:27:00Z","id":0}`),
		"negative id":   encode(`{"at":"2026-10-27T00:27:00Z","id":-1}`),
		"overflow id":   encode(`{"at":"2026-10-27T00:27:00Z","id":9223372036854775808}`),
		"zero time":     encode(`{"at":"0001-01-01T00:00:00Z","id":1}`),
		"unknown field": encode(`{"at":"2026-10-27T00:27:00Z","id":1,"offset":12}`),
		"trailing json": encode(`{"at":"2026-10-27T00:27:00Z","id":1} {}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeLibraryCursor(raw); err == nil {
				t.Fatal("accepted invalid cursor")
			}
		})
	}
	for _, limit := range []int{0, -1, 101} {
		if err := (LibraryQuery{Limit: limit}).Validate(); err == nil {
			t.Fatal(fmt.Sprintf("accepted limit %d", limit))
		}
	}
}

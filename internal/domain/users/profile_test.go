package users

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfilePatchOmissionClearAndValidation(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		name, bio bool
		valid     bool
	}{
		{`{"displayName":"Имя"}`, true, false, true},
		{`{"bio":"О себе"}`, false, true, true},
		{`{"bio":""}`, false, true, true},
		{`{"displayName":"Имя","bio":"О себе"}`, true, true, true},
		{`{}`, false, false, false},
		{`{"displayName":""}`, true, false, false},
	} {
		var patch UpdateProfileRequest
		if err := json.Unmarshal([]byte(tc.raw), &patch); err != nil {
			t.Fatal(err)
		}
		if (patch.DisplayName != nil) != tc.name || (patch.Bio != nil) != tc.bio {
			t.Fatal("field presence changed")
		}
		_, err := patch.Normalized()
		if (err == nil) != tc.valid {
			t.Fatalf("validation mismatch for %s: %v", tc.raw, err)
		}
	}
	for _, raw := range []string{`null`, `[]`, `{"bio":null}`, `{"displayName":null}`, `{"bio":123}`, `{"bio":true}`, `{"userId":"other","bio":"text"}`, `{"avatarVersion":"other"}`} {
		var patch UpdateProfileRequest
		if err := json.Unmarshal([]byte(raw), &patch); err == nil {
			t.Fatal("invalid profile JSON accepted")
		}
	}
}
func TestBioUnicodeLengthAndPlainMultilineText(t *testing.T) {
	for _, value := range []string{"", strings.Repeat("я", 1000), strings.Repeat("🙂", 1000), "  Не обрезать пробелы  ", "Первая\n\tВторая", "<b>plain text</b>"} {
		got, err := NormalizeBio(value)
		if err != nil || got != value {
			t.Fatal("valid bio changed or rejected", err)
		}
	}
	if got, err := NormalizeBio("Первая\r\nВторая"); err != nil || got != "Первая\nВторая" {
		t.Fatal("CRLF not normalized", err)
	}
	for _, value := range []string{strings.Repeat("🙂", 1001), "nul\x00", "bare\rreturn", "control\x1b", "invalid\xff"} {
		if _, err := NormalizeBio(value); err == nil {
			t.Fatal("invalid biography accepted")
		}
	}
	encoded, err := json.Marshal((User{Bio: "Текст"}).View())
	if err != nil || !strings.Contains(string(encoded), `"bio":"Текст"`) {
		t.Fatal("own account view omitted bio", err)
	}
}

package admin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAdministratorValidation(t *testing.T) {
	for input, want := range map[string]string{" Admin ": "admin", "Elena.OPS": "elena.ops", "user-123": "user-123", " User@Example.com ": "user@example.com", "operator+local@example.com": "operator+local@example.com"} {
		got, err := NormalizeUsername(input)
		if err != nil || got != want {
			t.Fatalf("normalize %q: %q %v", input, got, err)
		}
	}
	for _, v := range []string{"a", "ab", "user name@example.com", "user@example.com\nother", "админ", strings.Repeat("a", 41), "_admin", "@admin"} {
		if _, err := NormalizeUsername(v); err == nil {
			t.Errorf("accepted invalid username %q", v)
		}
	}
	for _, v := range []string{"", "2468246", strings.Repeat("😀", 7), strings.Repeat("a", 129), strings.Repeat("😀", 129), "invalid\xffpassword"} {
		if ValidatePassword(v) == nil {
			t.Errorf("accepted invalid password")
		}
	}
	for _, v := range []string{"24682468", "onlyletterspassword", "correct-password123", "длинныйпароль123", strings.Repeat("😀", 8), strings.Repeat("a", 128), strings.Repeat("😀", 128)} {
		if err := ValidatePassword(v); err != nil {
			t.Errorf("valid password rejected: %v", err)
		}
	}
	a := Administrator{Username: "admin", PasswordHash: "secret-not-for-json"}
	b, err := json.Marshal(a)
	if err != nil || strings.Contains(string(b), "secret") {
		t.Fatalf("password leaked: %s %v", b, err)
	}
}
func TestAdminQueryBounds(t *testing.T) {
	q := (Query{Page: -2, PageSize: 1000, Order: "asc; DROP TABLE users"}).Normalized()
	if q.Page != 1 || q.PageSize != 100 || q.Order != "desc" {
		t.Fatal(q)
	}
}

func TestUpdateEmailNullAndUnknownField(t *testing.T) {
	var request UpdateRequest
	if err := json.Unmarshal([]byte(`{"email":null}`), &request); err != nil || request.Email == nil || *request.Email != "" {
		t.Fatalf("explicit null does not clear email: %+v %v", request, err)
	}
	if err := json.Unmarshal([]byte(`{"password_hash":"bad"}`), &request); err == nil {
		t.Fatal("unknown sensitive field accepted")
	}
	if q := (Query{Q: strings.Repeat("я", 201)}).Normalized(); len([]rune(q.Q)) != 200 {
		t.Fatal("unicode query truncation failed")
	}
}

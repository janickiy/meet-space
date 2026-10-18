package folders

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const MaxFolders = 100

type Folder struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Position          int       `json:"position"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	ItemCount         int64     `json:"itemCount"`
	ConversationCount int64     `json:"conversationCount"`
	ConferenceCount   int64     `json:"conferenceCount"`
	Contains          *bool     `json:"contains,omitempty" gorm:"-"`
}

// Invite fields are always empty here; a folder never grants invitation access.
type ConferenceView struct {
	conferences.View
	ParticipantCount *int64 `json:"participantCount,omitempty"`
}
type Item struct {
	Type     string `json:"type"`
	Item     any    `json:"item"`
	InFolder *bool  `json:"inFolder,omitempty"`
}
type Page struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}
type Filter struct {
	Type   string
	Search string
}

// NormalizeName normalizes a folder name and its comparison key to enforce naming constraints.
//
// Parameters:
//   - raw: Untrusted serialized value to decode or validate.
//
// Returns:
//   - name: Validated normalized display name of the folder.
//   - key: Canonical comparison key used to enforce name uniqueness.
//   - err: nil on success; otherwise the validation, access, transport or persistence failure.
func NormalizeName(raw string) (name, key string, err error) {
	name, err = normalizeText(raw, 50, true)
	if err != nil {
		return "", "", err
	}
	return name, norm.NFC.String(cases.Fold().String(name)), nil
}

// NormalizeFilter normalizes and validates the filters used to list folder items.
//
// Parameters:
//   - filter: Validated filters restricting the result set.
//
// Returns:
//   - result 1: The resulting filter.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NormalizeFilter(filter Filter) (Filter, error) {
	if filter.Type == "" {
		filter.Type = "all"
	}
	if filter.Type != "all" && !ValidKind(filter.Type) {
		return filter, apperrors.ErrInvalidInput
	}
	var err error
	filter.Search, err = normalizeText(filter.Search, 100, false)
	return filter, err
}

// ValidKind reports whether an item kind is supported by folders.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func ValidKind(kind string) bool { return kind == "conversation" || kind == "conference" }

// normalizeText normalizes UTF-8 text and enforces the requested length and presence
// constraints.
//
// Parameters:
//   - raw: Untrusted serialized value to decode or validate.
//   - maximum: Maximum permitted size or number of characters.
//   - required: Whether an empty value must be rejected.
//
// Returns:
//   - result 1: Validated normalized UTF-8 text.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func normalizeText(raw string, maximum int, required bool) (string, error) {
	if !utf8.ValidString(raw) {
		return "", apperrors.ErrInvalidInput
	}
	text := norm.NFC.String(strings.TrimSpace(raw))
	if utf8.RuneCountInString(text) > maximum || (required && text == "") {
		return "", apperrors.New(apperrors.ErrInvalidInput, "invalid folder name or search length")
	}
	for _, r := range text {
		joiner := r == '\u200c' || r == '\u200d'
		if unicode.IsControl(r) || (unicode.Is(unicode.Cf, r) && !joiner) {
			return "", apperrors.New(apperrors.ErrInvalidInput, "unsupported control characters")
		}
	}
	return text, nil
}

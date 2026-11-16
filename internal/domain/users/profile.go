package users

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

const MaxBioCharacters = 1000

// UpdateProfileRequest preserves omission independently from an explicit empty
// value. An empty bio clears it; an empty name remains invalid.
type UpdateProfileRequest struct {
	DisplayName *string `json:"displayName,omitempty"`
	Bio         *string `json:"bio,omitempty"`
}

func (r *UpdateProfileRequest) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return apperrors.ErrInvalidInput
	}
	var result UpdateProfileRequest
	for key, raw := range fields {
		var value string
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &value) != nil {
			return apperrors.ErrInvalidInput
		}
		switch key {
		case "displayName":
			result.DisplayName = &value
		case "bio":
			result.Bio = &value
		default:
			return apperrors.ErrInvalidInput
		}
	}
	*r = result
	return nil
}

// NormalizeBio keeps multiline plain text, counting Unicode code points rather
// than UTF-8 bytes. CRLF input is normalized to a single line feed.
func NormalizeBio(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", apperrors.New(apperrors.ErrInvalidInput, "bio must contain valid Unicode text")
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if utf8.RuneCountInString(value) > MaxBioCharacters {
		return "", apperrors.New(apperrors.ErrInvalidInput, "bio must contain at most 1000 characters")
	}
	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return "", apperrors.New(apperrors.ErrInvalidInput, "bio must not contain control characters other than line feeds or tabs")
		}
	}
	return value, nil
}

func (r UpdateProfileRequest) Normalized() (UpdateProfileRequest, error) {
	var normalized UpdateProfileRequest
	if r.DisplayName == nil && r.Bio == nil {
		return normalized, apperrors.New(apperrors.ErrInvalidInput, "provide displayName or bio")
	}
	if r.DisplayName != nil {
		name, err := NormalizeDisplayName(*r.DisplayName)
		if err != nil {
			return normalized, err
		}
		normalized.DisplayName = &name
	}
	if r.Bio != nil {
		bio, err := NormalizeBio(*r.Bio)
		if err != nil {
			return normalized, err
		}
		normalized.Bio = &bio
	}
	return normalized, nil
}

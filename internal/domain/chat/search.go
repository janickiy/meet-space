package chat

import (
	"strings"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// NormalizeSearchQuery bounds literal message searches before any database work.
func NormalizeSearchQuery(query string) (string, error) {
	query = strings.TrimSpace(query)
	length := utf8.RuneCountInString(query)
	if !utf8.ValidString(query) || strings.ContainsRune(query, 0) || length < 2 || length > 200 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "query must contain 2 to 200 characters")
	}
	return query, nil
}

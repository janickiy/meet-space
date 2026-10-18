package chat

import (
	"strings"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// NormalizeSearchQuery bounds literal message searches before any database work.
//
// Parameters:
//   - query: Search text or validated query filters.
//
// Returns:
//   - result 1: Bounded normalized search text.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NormalizeSearchQuery(query string) (string, error) {
	query = strings.TrimSpace(query)
	length := utf8.RuneCountInString(query)
	if !utf8.ValidString(query) || strings.ContainsRune(query, 0) || length < 2 || length > 200 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "query must contain 2 to 200 characters")
	}
	return query, nil
}

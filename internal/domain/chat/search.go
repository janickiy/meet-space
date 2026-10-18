package chat

import (
	"strings"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// NormalizeSearchQuery ограничивает поиск по буквам в сообщении перед любыми операциями с базой
// данных.
//
// @Parameters:
//   - query: Текст для поиска или валидированные фильтры запросов.
//
// @returns:
//   - result1: Ограниченный нормализованный поисковый запрос.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NormalizeSearchQuery(query string) (string, error) {
	query = strings.TrimSpace(query)
	length := utf8.RuneCountInString(query)
	if !utf8.ValidString(query) || strings.ContainsRune(query, 0) || length < 2 || length > 200 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "query must contain 2 to 200 characters")
	}
	return query, nil
}

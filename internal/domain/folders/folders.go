// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

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

// Поля приглашения всегда пусты, так как папка никогда не предоставляет доступ по приглашению.
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

// NormalizeName нормализует имя папки и ключ сравнения для обеспечения соблюдения ограничений
// на использование имен.
//
// @Parameters:
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//
// @returns:
//   - name: Проверенное нормализованное отображаемое имя папки.
//   - key: Канонический ключ сравнения, используемый для обеспечения уникальности имени.
//   - err: nil — в случае успеха; в противном случае — ошибка валидации, доступа, передачи
//     данных или сохранения.
func NormalizeName(raw string) (name, key string, err error) {
	name, err = normalizeText(raw, 50, true)
	if err != nil {
		return "", "", err
	}
	return name, norm.NFC.String(cases.Fold().String(name)), nil
}

// NormalizeFilter нормализует и проверяет фильтры, используемые для вывода списка элементов
// папки.
//
// @Parameters:
//   - filter: Проверенные фильтры, ограничивающие набор результатов.
//
// @returns:
//   - result1: Результирующий фильтр.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// ValidKind сообщает, поддерживаются ли папки для того или иного типа элементов.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func ValidKind(kind string) bool { return kind == "conversation" || kind == "conference" }

// normalizeText нормализует текст UTF-8 и обеспечивает соблюдение заданных ограничений по длине
// и наличию.
//
// @Parameters:
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//   - maximum: Максимально допустимый размер или количество символов.
//   - required: Нужно ли отклонять пустое значение.
//
// @returns:
//   - result1: Валидированный нормализованный текст UTF-8.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

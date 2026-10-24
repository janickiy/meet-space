// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

const (
	MaxTextRunes             = 4000
	MaxAttachments           = 5
	MaxAttachmentBytes int64 = 10 << 20
	AttachmentTTL            = 30 * time.Minute
	UploadLease              = 2 * time.Minute
	DownloadTTL              = 5 * time.Minute
)

// Message сохраняет порядок сообщений в чате, их ревизию, автора и информацию о повторных
// попытках отправки.  Его внутренний отпечаток запроса не передаётся в JSON.
type Message struct {
	ID                 string        `json:"id" gorm:"type:uuid;primaryKey"`
	Sequence           int64         `json:"sequence,string" gorm:"autoIncrement"`
	ConferenceID       string        `json:"conferenceId,omitempty"`
	ConversationID     string        `json:"conversationId,omitempty"`
	SenderID           string        `json:"senderId" gorm:"column:sender_user_id"`
	SenderName         string        `json:"senderName" gorm:"->;-:migration"`
	ClientRequestID    string        `json:"clientRequestId"`
	RequestFingerprint string        `json:"-"`
	Text               string        `json:"text"`
	ReplyTo            *string       `json:"replyTo" gorm:"column:reply_to_id"`
	ReplyPreview       *ReplyPreview `json:"replyPreview,omitempty" gorm:"-"`
	Version            int64         `json:"version"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
	DeletedAt          *time.Time    `json:"deletedAt"`
	Attachments        []Attachment  `json:"attachments" gorm:"-"`
	Important          bool          `json:"important,omitempty" gorm:"-"`
	Pinned             bool          `json:"pinned,omitempty" gorm:"-"`
	CallID             *string       `json:"-" gorm:"->;-:migration"`
	Call               *CallHistory  `json:"call,omitempty" gorm:"-"`
}

// CallHistory отображает достоверное состояние вызова, не раскрывая данные о комнате или
// устройстве.
type CallHistory struct {
	ID           string     `json:"id"`
	CallerUserID string     `json:"callerUserId"`
	CalleeUserID string     `json:"calleeUserId"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	AcceptedAt   *time.Time `json:"acceptedAt"`
	EndedAt      *time.Time `json:"endedAt"`
	DurationSec  int64      `json:"durationSec"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Message) TableName() string { return "chat_messages" }

// ReplyPreview содержит ограниченное представление сообщения, на которое был отправлен ответ, и
// поддерживает обратимое удаление.
type ReplyPreview struct {
	Sequence   int64  `json:"sequence,string,omitempty"`
	ID         string `json:"id"`
	Text       string `json:"text"`
	SenderName string `json:"senderName"`
	Deleted    bool   `json:"deleted"`
}

// Attachment сохраняет информацию о владельце, конференции, метаданные объекта и состояние
// загрузки. Внутренние пути к объектам и токены аренды исключены из JSON.
type Attachment struct {
	ID               string     `json:"id" gorm:"type:uuid;primaryKey"`
	ConferenceID     string     `json:"conferenceId,omitempty"`
	ConversationID   string     `json:"conversationId,omitempty"`
	OwnerID          string     `json:"ownerId" gorm:"column:owner_user_id"`
	ClientRequestID  string     `json:"clientRequestId"`
	Filename         string     `json:"filename"`
	MimeType         string     `json:"mimeType"`
	Size             int64      `json:"size"`
	Checksum         string     `json:"checksum,omitempty"`
	Status           string     `json:"status"`
	MessageID        *string    `json:"messageId,omitempty"`
	ObjectKey        string     `json:"-"`
	UploadToken      *string    `json:"-"`
	UploadLeaseUntil *time.Time `json:"-"`
	UploadedAt       *time.Time `json:"-"`
	CleanedAt        *time.Time `json:"-"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	ExpiresAt        time.Time  `json:"expiresAt"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Attachment) TableName() string { return "chat_attachments" }

// Prefix возвращает префикс объекта, управляемый сервером, для вложения.
//
// @returns:
//   - result1: Префикс приватного вложения, управляемый сервером.
func (a Attachment) Prefix() string {
	if a.ConversationID != "" {
		return "attachments/direct/" + a.ConversationID + "/" + a.ID + "/"
	}
	return "attachments/" + a.ConferenceID + "/" + a.ID + "/"
}

// SendRequest содержит текст сообщения, ссылку на ответ, идентификаторы вложений и ключ
// идемпотентности повторных попыток.
type SendRequest struct {
	ClientRequestID string   `json:"clientRequestId"`
	Text            string   `json:"text"`
	ReplyTo         string   `json:"replyTo,omitempty"`
	AttachmentIDs   []string `json:"attachmentIds,omitempty"`
}

// EditRequest содержит текст для замены существующего сообщения.
type EditRequest struct {
	Text string `json:"text"`
}

// InitRequest описывает будущее вложение до выделения ключа для загрузки, принадлежащего
// серверу.
type InitRequest struct {
	ClientRequestID string `json:"clientRequestId"`
	Filename        string `json:"filename"`
	MimeType        string `json:"mimeType"`
	Size            int64  `json:"size"`
}

// ReadRequest идентифицирует сообщение, через которое учётная запись ознакомилась с перепиской.
type ReadRequest struct {
	MessageID string `json:"messageId"`
}

// ReadState содержит монотонную границу чтения и видимое количество непрочитанных сообщений.
type ReadState struct {
	LastReadMessageID *string `json:"lastReadMessageId"`
	LastReadSequence  int64   `json:"-"`
	UnreadCount       int64   `json:"unreadCount"`
	ParticipantID     string  `json:"-"`
}

// Page содержит одну страницу с ограниченным объёмом данных и метаданные о продолжении.
type Page struct {
	Items             []Message `json:"items"`
	NextCursor        string    `json:"nextCursor,omitempty"`
	UnreadCount       int64     `json:"unreadCount"`
	LastReadMessageID *string   `json:"lastReadMessageId"`
}

// UUID проверяет и нормализует UUID перед использованием в качестве идентификатора ресурса.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Канонический ненулевой UUID.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func UUID(value string) (string, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return "", apperrors.New(apperrors.ErrInvalidInput, "a non-zero UUID is required")
	}
	return id.String(), nil
}

// NormalizeText нормализует и ограничивает текст сообщения перед сохранением.
//
// @Parameters:
//   - text: Текст UTF-8 для проверки, нормализации или обработки.
//   - allowEmpty: Указывает, разрешён ли пустой нормализованный текст.
//
// @returns:
//   - result1: Проверенный нормализованный текст сообщения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NormalizeText(text string, allowEmpty bool) (string, error) {
	text = strings.TrimSpace(text)
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxTextRunes || (!allowEmpty && text == "") || strings.ContainsRune(text, 0) {
		return "", apperrors.New(apperrors.ErrInvalidInput, "message must contain 1 to 4000 characters or an attachment")
	}
	return text, nil
}

// NormalizeSend проверяет текст сообщения, ответ и ссылки на вложения перед отправкой
// сообщения.
//
// @Parameters:
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Результирующий запрос на отправку.
//   - result2: Стабильный отпечаток SHA-256 нормализованного запроса на отправку.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NormalizeSend(request SendRequest) (SendRequest, string, error) {
	var err error
	request.ClientRequestID, err = UUID(request.ClientRequestID)
	if err != nil {
		return request, "", err
	}
	if len(request.AttachmentIDs) > MaxAttachments {
		return request, "", apperrors.New(apperrors.ErrInvalidInput, "at most 5 attachments are allowed")
	}
	request.AttachmentIDs = append([]string(nil), request.AttachmentIDs...)
	seen := map[string]bool{}
	for i, id := range request.AttachmentIDs {
		id, err = UUID(id)
		if err != nil {
			return request, "", err
		}
		if seen[id] {
			return request, "", apperrors.New(apperrors.ErrInvalidInput, "duplicate attachment")
		}
		seen[id] = true
		request.AttachmentIDs[i] = id
	}
	sort.Strings(request.AttachmentIDs)
	if request.ReplyTo != "" {
		request.ReplyTo, err = UUID(request.ReplyTo)
		if err != nil {
			return request, "", err
		}
	}
	request.Text, err = NormalizeText(request.Text, len(request.AttachmentIDs) > 0)
	if err != nil {
		return request, "", err
	}
	raw, _ := json.Marshal(request)
	sum := sha256.Sum256(raw)
	return request, hex.EncodeToString(sum[:]), nil
}

// NormalizeInit проверяет метаданные вложений перед попыткой их загрузки.
//
// @Parameters:
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Результирующий запрос на инициализацию.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NormalizeInit(request InitRequest) (InitRequest, error) {
	var err error
	request.ClientRequestID, err = UUID(request.ClientRequestID)
	if err != nil {
		return request, err
	}
	request.Filename = strings.TrimSpace(request.Filename)
	if !utf8.ValidString(request.Filename) || request.Filename == "" || utf8.RuneCountInString(request.Filename) > 180 || filepath.Base(request.Filename) != request.Filename || strings.ContainsAny(request.Filename, "/\\") || strings.Contains(request.Filename, "..") {
		return request, apperrors.New(apperrors.ErrInvalidInput, "invalid filename")
	}
	for _, r := range request.Filename {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return request, apperrors.New(apperrors.ErrInvalidInput, "invalid filename")
		}
	}
	request.MimeType, err = normalizeAttachmentMIME(request.Filename, request.MimeType)
	if err != nil {
		return request, err
	}
	if request.Size < 1 || request.Size > MaxAttachmentBytes {
		return request, apperrors.New(apperrors.ErrInvalidInput, fmt.Sprintf("attachment size must be between 1 and %d bytes", MaxAttachmentBytes))
	}
	return request, nil
}

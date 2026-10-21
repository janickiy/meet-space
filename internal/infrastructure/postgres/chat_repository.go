// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type ChatRepository struct {
	db     *gorm.DB
	direct bool
}

// NewChatRepository создаёт репозиторий чатов с зависимостями и ограничениями, необходимыми для
// хранения сообщений в чатах и приватных вложений.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий чатов.
func NewChatRepository(db *gorm.DB) *ChatRepository {
	return &ChatRepository{db: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})}
}

// authorizeChat проверяет сохранённые данные о членстве и состоянии конференции перед доступом
// к ресурсам чата конференции.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - locked: Указывает, должен ли запрос блокировать выбранную строку перед изменением.
//   - write: Выполняет ли этот доступ мутацию и требует ли авторизации на запись.
//
// @returns:
//   - result1: Полученный участник.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func authorizeChat(tx *gorm.DB, userID, conferenceID string, locked, write bool) (conferences.Participant, error) {
	c, err := findConference(tx, conferenceID, locked)
	if err != nil {
		return conferences.Participant{}, err
	}
	p, err := findMembership(tx, conferenceID, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return p, apperrors.ErrForbidden
	}
	if err != nil {
		return p, err
	}
	if !p.CanReadHistory() {
		return p, apperrors.ErrForbidden
	}
	var left int64
	if err := tx.Table("conference_chat_preferences").Where("conference_id=? AND user_id=? AND left_at IS NOT NULL", conferenceID, userID).Count(&left).Error; err != nil {
		return p, err
	}
	if left > 0 {
		return p, apperrors.ErrForbidden
	}
	if write && (c.Status != conferences.Active || !p.CanParticipate()) {
		return p, apperrors.New(apperrors.ErrConflict, "chat is read-only unless the conference is active and you have joined")
	}
	return p, nil
}

// chatCursor кодирует стабильную границу разбивки истории сообщений на страницы.
//
// @Parameters:
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - sequence: Стабильная последовательность упорядочивания сообщений или событий.
//
// @returns:
//   - result1: Непрозрачный курсор продолжения истории сообщений.
func chatCursor(conferenceID string, sequence int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(conferenceID + ":" + strconv.FormatInt(sequence, 10)))
}

// parseChatCursor декодирует и проверяет границу разбивки истории сообщений на страницы.
//
// @Parameters:
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func parseChatCursor(conferenceID, cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	if len(cursor) > 128 {
		return 0, apperrors.ErrInvalidInput
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, apperrors.ErrInvalidInput
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 2 || parts[0] != conferenceID {
		return 0, apperrors.ErrInvalidInput
	}
	n, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || n < 1 {
		return 0, apperrors.ErrInvalidInput
	}
	return n, nil
}

// messageQuery формирует запрос к сообщениям с привязкой к авторизованному диалогу.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//
// @returns:
//   - result1: Созданная или загруженная база данных.
func (r *ChatRepository) messageQuery(tx *gorm.DB) *gorm.DB {
	if r.direct {
		return r.messages(tx).Select("conversation_messages.*, COALESCE(NULLIF(u.display_name,''),'Пользователь') AS sender_name").Joins("JOIN users u ON u.id=conversation_messages.sender_user_id")
	}
	return r.messages(tx).Select("chat_messages.*, p.display_name AS sender_name").Joins(r.sql("JOIN conference_participants p ON p.conference_id=chat_messages.conference_id AND p.user_id=chat_messages.sender_user_id"))
}

// loadMessage загружает сообщение вместе с сопутствующими метаданными, необходимыми вызывающей
// стороне.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Полученное сообщение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) loadMessage(tx *gorm.DB, conferenceID, id, userID string) (chat.Message, error) {
	var message chat.Message
	cutoff, err := r.historyCutoff(tx, userID, conferenceID)
	if err != nil {
		return message, err
	}
	err = r.messageQuery(tx).Where(r.sql("chat_messages.id=? AND chat_messages.conference_id=? AND sequence>?"), id, conferenceID, cutoff).Take(&message).Error
	if err != nil {
		return message, mapNotFound(err)
	}
	rows := []chat.Message{message}
	err = r.decorateMessages(tx, rows, cutoff)
	return rows[0], err
}

// decorateMessages загружает сопутствующие метаданные о вложениях и вызовах для страницы
// сообщений.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - items: Ограниченная коллекция записей для обработки.
//   - cutoffs: Необязательные границы истории для конкретного участника.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) decorateMessages(tx *gorm.DB, items []chat.Message, cutoffs ...int64) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	var cutoff int64
	if len(cutoffs) > 0 {
		cutoff = cutoffs[0]
	}
	replyIDs := []string{}
	for i := range items {
		items[i].Attachments = []chat.Attachment{}
		ids = append(ids, items[i].ID)
		if items[i].ReplyTo != nil {
			replyIDs = append(replyIDs, *items[i].ReplyTo)
		}
	}
	if r.direct {
		if err := decorateCallHistory(tx, items, ids); err != nil {
			return err
		}
		if err := decorateConversationPins(tx, items, ids); err != nil {
			return err
		}
	}
	var attachments []chat.Attachment
	if err := r.attachments(tx).Where("message_id IN ? AND status='attached'", ids).Order("created_at,id").Find(&attachments).Error; err != nil {
		return err
	}
	byMessage := map[string][]chat.Attachment{}
	for _, a := range attachments {
		byMessage[*a.MessageID] = append(byMessage[*a.MessageID], a)
	}
	replies := map[string]chat.ReplyPreview{}
	if len(replyIDs) > 0 {
		var rows []chat.Message
		if err := r.messageQuery(tx).Where(r.sql("chat_messages.conference_id=? AND chat_messages.id IN ? AND sequence>?"), r.messageScope(items[0]), replyIDs, cutoff).Find(&rows).Error; err != nil {
			return err
		}
		for _, m := range rows {
			replies[m.ID] = chat.ReplyPreview{ID: m.ID, Sequence: m.Sequence, Text: m.Text, SenderName: m.SenderName, Deleted: m.DeletedAt != nil}
		}
	}
	for i := range items {
		if items[i].DeletedAt == nil {
			if files := byMessage[items[i].ID]; files != nil {
				items[i].Attachments = files
			}
		} else {
			items[i].Text = ""
		}
		if items[i].ReplyTo != nil {
			if reply, ok := replies[*items[i].ReplyTo]; ok {
				items[i].ReplyPreview = &reply
			} else if cutoff > 0 {
				items[i].ReplyTo = nil
				items[i].ReplyPreview = nil
			}
		}
	}
	return nil
}

// readState загружает сохранённую границу прочитанных сообщений и количество непрочитанных
// сообщений для авторизованной области чата.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Полученное состояние чтения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) readState(tx *gorm.DB, userID, conferenceID string) (chat.ReadState, error) {
	var state chat.ReadState
	if err := tx.Table(r.readTable()).Select("last_read_message_id,last_read_sequence").Where(r.sql("conference_id=? AND user_id=?"), conferenceID, userID).Scan(&state).Error; err != nil {
		return state, err
	}
	cutoff, err := r.historyCutoff(tx, userID, conferenceID)
	if err != nil {
		return state, err
	}
	if cutoff > state.LastReadSequence {
		state.LastReadSequence = cutoff
	}
	err = r.messages(tx).Where(r.sql("conference_id=? AND sequence>? AND sender_user_id<>? AND deleted_at IS NULL"), conferenceID, state.LastReadSequence, userID).Count(&state.UnreadCount).Error
	if cutoff > 0 && state.LastReadSequence <= cutoff {
		state.LastReadMessageID = nil
	}
	return state, err
}

// List возвращает страницу с ограниченным количеством записей чата с использованием заданных
// параметров разбивки на страницы и фильтров.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) List(ctx context.Context, userID, conferenceID, cursor string, limit int) (chat.Page, error) {
	page := chat.Page{Items: []chat.Message{}}
	before, err := parseChatCursor(conferenceID, cursor)
	if err != nil {
		return page, err
	}
	if limit < 1 || limit > 100 {
		return page, apperrors.New(apperrors.ErrInvalidInput, "limit must be between 1 and 100")
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.authorize(tx, userID, conferenceID, false, false); err != nil {
			return err
		}
		cutoff, err := r.historyCutoff(tx, userID, conferenceID)
		if err != nil {
			return err
		}
		query := r.messageQuery(tx).Where(r.sql("chat_messages.conference_id=? AND sequence>?"), conferenceID, cutoff)
		if before > 0 {
			query = query.Where("sequence<?", before)
		}
		if err := query.Order("sequence DESC").Limit(limit + 1).Find(&page.Items).Error; err != nil {
			return err
		}
		if len(page.Items) > limit {
			page.Items = page.Items[:limit]
			page.NextCursor = chatCursor(conferenceID, page.Items[len(page.Items)-1].Sequence)
		}
		for i, j := 0, len(page.Items)-1; i < j; i, j = i+1, j-1 {
			page.Items[i], page.Items[j] = page.Items[j], page.Items[i]
		}
		if err := r.decorateMessages(tx, page.Items, cutoff); err != nil {
			return err
		}
		if err := r.markImportant(tx, userID, conferenceID, page.Items); err != nil {
			return err
		}
		state, err := r.readState(tx, userID, conferenceID)
		page.UnreadCount = state.UnreadCount
		page.LastReadMessageID = state.LastReadMessageID
		return err
	})
	return page, err
}

// markImportant проецирует персональные закладки только в аутентифицированные чтения REST.  В
// рассылках конференций и сохранённых сообщениях никогда не используется маркер другого
// участника.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - items: Ограниченная коллекция записей для обработки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) markImportant(tx *gorm.DB, userID, conferenceID string, items []chat.Message) error {
	if r.direct || len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	var pinned []string
	if err := tx.Table("conference_chat_pins").Where("conference_id=? AND user_id=? AND message_id IN ?", conferenceID, userID, ids).Pluck("message_id", &pinned).Error; err != nil {
		return err
	}
	set := make(map[string]bool, len(pinned))
	for _, id := range pinned {
		set[id] = true
	}
	for i := range items {
		items[i].Important = set[items[i].ID] && items[i].DeletedAt == nil
	}
	return nil
}

// Send проверяет ссылки на доступ, ответы и вложения и сохраняет сообщение чата с защитой от
// повторяющихся идентичных запросов.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//   - fingerprint: Стабильный отпечаток запроса, используемый для обнаружения повторяющихся
//     идентичных запросов.
//
// @returns:
//   - result1: Полученное сообщение.
//   - result2: Указывает, создал ли этот запрос новое состояние или повторно использовал
//     идемпотентный результат.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) Send(ctx context.Context, userID, conferenceID string, request chat.SendRequest, fingerprint string) (chat.Message, bool, error) {
	var message chat.Message
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.authorize(tx, userID, conferenceID, true, false); err != nil {
			return err
		}
		err := r.messages(tx).Where(r.sql("conference_id=? AND sender_user_id=? AND client_request_id=?"), conferenceID, userID, request.ClientRequestID).Take(&message).Error
		if err == nil {
			if message.RequestFingerprint != fingerprint {
				return apperrors.New(apperrors.ErrConflict, "clientRequestId was already used for a different message")
			}
			message, err = r.loadMessage(tx, conferenceID, message.ID, userID)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if _, err := r.authorize(tx, userID, conferenceID, false, true); err != nil {
			return err
		}
		cutoff, err := r.historyCutoff(tx, userID, conferenceID)
		if err != nil {
			return err
		}
		var replyTo *string
		if request.ReplyTo != "" {
			var reply chat.Message
			if err := r.messages(tx).Where(r.sql("id=? AND conference_id=? AND deleted_at IS NULL AND sequence>?"), request.ReplyTo, conferenceID, cutoff).Take(&reply).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apperrors.New(apperrors.ErrInvalidInput, "reply target is unavailable in this conference")
				}
				return err
			}
			replyTo = &request.ReplyTo
		}
		var attachments []chat.Attachment
		if len(request.AttachmentIDs) > 0 {
			if err := r.attachments(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", request.AttachmentIDs).Find(&attachments).Error; err != nil {
				return err
			}
			if len(attachments) != len(request.AttachmentIDs) {
				return apperrors.New(apperrors.ErrInvalidInput, "attachment is unavailable")
			}
			for _, a := range attachments {
				if r.attachmentScope(a) != conferenceID || a.OwnerID != userID || a.Status != "ready" || !a.ExpiresAt.After(time.Now()) {
					return apperrors.New(apperrors.ErrConflict, "attachment is unavailable or expired")
				}
			}
		}
		message = chat.Message{ID: uuid.NewString(), ConferenceID: conferenceID, SenderID: userID, ClientRequestID: request.ClientRequestID, RequestFingerprint: fingerprint, Text: request.Text, ReplyTo: replyTo, Version: 1}
		r.setMessageScope(&message, conferenceID)
		if err := r.messages(tx).Create(&message).Error; err != nil {
			return err
		}
		if !r.direct {
			if err := tx.Exec(`INSERT INTO chat_notification_jobs(message_id,conference_id)
				VALUES(?::uuid,?::uuid) ON CONFLICT DO NOTHING`, message.ID, conferenceID).Error; err != nil {
				return err
			}
		}
		if len(attachments) > 0 {
			if err := r.attachments(tx).Where("id IN ?", request.AttachmentIDs).Updates(map[string]any{"status": "attached", "message_id": message.ID, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
				return err
			}
		}
		if r.direct {
			if err := tx.Table("conversations").Where("id=?", conferenceID).Updates(map[string]any{"last_message_at": message.CreatedAt, "last_message_id": message.ID, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
				return err
			}
			if err := tx.Table("conversation_members").Where("conversation_id=? AND hidden_at IS NOT NULL", conferenceID).
				Updates(map[string]any{"hidden_at": nil, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
				return err
			}
			if err := createConversationNotifications(tx, message.ID); err != nil {
				return err
			}
		}
		created = true
		message, err = r.loadMessage(tx, conferenceID, message.ID, userID)
		return err
	})
	if err != nil {
		return chat.Message{}, false, err
	}
	return message, created, nil
}

// Edit — при редактировании существующий текст сообщения автора изменяется после проверки видимости и
// возможности редактирования.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - text: Текст UTF-8 для проверки, нормализации или обработки.
//
// @returns:
//   - result1: Полученное сообщение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) Edit(ctx context.Context, userID, conferenceID, id, text string) (chat.Message, error) {
	var item chat.Message
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.authorize(tx, userID, conferenceID, true, true); err != nil {
			return err
		}
		var message chat.Message
		cutoff, err := r.historyCutoff(tx, userID, conferenceID)
		if err != nil {
			return err
		}
		if err := r.messages(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(r.sql("id=? AND conference_id=? AND sequence>?"), id, conferenceID, cutoff).Take(&message).Error; err != nil {
			return mapNotFound(err)
		}
		if message.SenderID != userID {
			return apperrors.ErrForbidden
		}
		if message.CallID != nil {
			return apperrors.New(apperrors.ErrForbidden, "call history cannot be edited")
		}
		if message.DeletedAt != nil {
			return apperrors.New(apperrors.ErrConflict, "message was deleted")
		}
		if text == "" {
			var count int64
			if err := r.attachments(tx).Where("message_id=? AND status='attached'", id).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return apperrors.New(apperrors.ErrInvalidInput, "message cannot be empty")
			}
		}
		if err := r.messages(tx).Where("id=?", id).Updates(map[string]any{"text": text, "version": gorm.Expr("version+1"), "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		item, err = r.loadMessage(tx, conferenceID, id, userID)
		return err
	})
	if err != nil {
		return chat.Message{}, err
	}
	return item, nil
}

// Delete удаляет запрошенный чат, используя область видимости вызывающей стороны и правила
// жизненного цикла ресурса.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученное сообщение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) Delete(ctx context.Context, userID, conferenceID, id string) (chat.Message, error) {
	var item chat.Message
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := r.authorize(tx, userID, conferenceID, true, true)
		if err != nil {
			return err
		}
		var message chat.Message
		cutoff, err := r.historyCutoff(tx, userID, conferenceID)
		if err != nil {
			return err
		}
		if err := r.messages(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(r.sql("id=? AND conference_id=? AND sequence>?"), id, conferenceID, cutoff).Take(&message).Error; err != nil {
			return mapNotFound(err)
		}
		if message.SenderID != userID && p.Role != conferences.Owner && p.Role != conferences.CoHost {
			return apperrors.ErrForbidden
		}
		if message.CallID != nil {
			return apperrors.New(apperrors.ErrForbidden, "call history cannot be deleted as a message")
		}
		if message.DeletedAt == nil {
			if err := r.messages(tx).Where("id=?", id).Updates(map[string]any{"text": "", "deleted_at": gorm.Expr("clock_timestamp()"), "updated_at": gorm.Expr("clock_timestamp()"), "version": gorm.Expr("version+1")}).Error; err != nil {
				return err
			}
		}
		if r.direct {
			if err := tx.Exec("DELETE FROM conversation_message_pins WHERE conversation_id=? AND message_id=?", conferenceID, id).Error; err != nil {
				return err
			}
		}
		item, err = r.loadMessage(tx, conferenceID, id, userID)
		return err
	})
	if err != nil {
		return chat.Message{}, err
	}
	return item, nil
}

// ReadState возвращает сохраненную границу прочитанных сообщений участника и количество видимых
// непрочитанных сообщений.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Полученное состояние чтения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) ReadState(ctx context.Context, userID, conferenceID string) (chat.ReadState, error) {
	var state chat.ReadState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := r.authorize(tx, userID, conferenceID, false, false)
		if err != nil {
			return err
		}
		state, err = r.readState(tx, userID, conferenceID)
		state.ParticipantID = p.ID
		return err
	})
	return state, err
}

// MarkRead перемещает сохраненную границу прочитанных сообщений, не позволяя поздним или
// повторным запросам сдвигать ее назад.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - messageID: Идентификатор сообщения в рамках авторизованного диалога.
//
// @returns:
//   - result1: Полученное состояние чтения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) MarkRead(ctx context.Context, userID, conferenceID, messageID string) (chat.ReadState, error) {
	var state chat.ReadState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := r.authorize(tx, userID, conferenceID, true, false)
		if err != nil {
			return err
		}
		var message chat.Message
		cutoff, err := r.historyCutoff(tx, userID, conferenceID)
		if err != nil {
			return err
		}
		if err := r.messages(tx).Select("id,sequence").Where(r.sql("id=? AND conference_id=? AND sequence>?"), messageID, conferenceID, cutoff).Take(&message).Error; err != nil {
			return mapNotFound(err)
		}
		if r.direct {
			err = tx.Table("conversation_members").Where("conversation_id=? AND user_id=? AND last_read_sequence<?", conferenceID, userID, message.Sequence).Updates(map[string]any{"last_read_message_id": messageID, "last_read_sequence": message.Sequence, "updated_at": gorm.Expr("clock_timestamp()")}).Error
		} else {
			err = tx.Exec(`INSERT INTO chat_read_states(conference_id,user_id,last_read_message_id,last_read_sequence) VALUES(?,?,?,?) ON CONFLICT(conference_id,user_id) DO UPDATE SET last_read_message_id=EXCLUDED.last_read_message_id,last_read_sequence=EXCLUDED.last_read_sequence,updated_at=clock_timestamp() WHERE chat_read_states.last_read_sequence<EXCLUDED.last_read_sequence`, conferenceID, userID, messageID, message.Sequence).Error
		}
		if err != nil {
			return err
		}
		state, err = r.readState(tx, userID, conferenceID)
		state.ParticipantID = p.ID
		return err
	})
	if err != nil {
		return chat.ReadState{}, err
	}
	return state, nil
}

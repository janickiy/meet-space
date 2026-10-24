// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

// NewDirectChatRepository изменяет хранилище и права доступа, предоставляя общий доступ ко всем
// операциям с сообщениями и файлами.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий чатов.
func NewDirectChatRepository(db *gorm.DB) *ChatRepository {
	r := NewChatRepository(db)
	r.direct = true
	return r
}

// sql выбирает таблицу SQL или область видимости, используемую этим адаптером репозитория.
//
// @Parameters:
//   - s: Проверенное отношение SQL, управляемое сервером, или скалярный входной параметр.
//
// @returns:
//   - result1: Отношение SQL или оператор, выбранное сервером в области видимости.
func (r *ChatRepository) sql(s string) string {
	if !r.direct {
		return s
	}
	s = strings.ReplaceAll(s, "conference_id", "conversation_id")
	return strings.ReplaceAll(s, "chat_messages", "conversation_messages")
}

// messages возвращает отношение сообщений для области видимости диалога в репозитории.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//
// @returns:
//   - result1: Созданная или загруженная база данных.
func (r *ChatRepository) messages(tx *gorm.DB) *gorm.DB {
	if r.direct {
		return tx.Model(&chat.Message{}).Table("conversation_messages").Omit("ConferenceID", "SenderName")
	}
	// CallID относится только к conversation_messages; SELECTs, основанный на модели, не должен
	// проецировать его из chat_messages, в котором нет такого столбца.
	return tx.Model(&chat.Message{}).Omit("ConversationID", "SenderName", "CallID")
}

// attachments возвращает отношение вложений для области видимости диалога в репозитории.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//
// @returns:
//   - result1: Созданная или загруженная база данных.
func (r *ChatRepository) attachments(tx *gorm.DB) *gorm.DB {
	if r.direct {
		return tx.Model(&chat.Attachment{}).Table("conversation_attachments").Omit("ConferenceID", "SenderName")
	}
	return tx.Model(&chat.Attachment{}).Omit("ConversationID", "SenderName")
}

// readTable возвращает таблицу состояний чтения, связанную с областью видимости чата.
//
// @returns:
//   - result1: Таблица состояний чтения для выбранной области видимости диалога.
func (r *ChatRepository) readTable() string {
	if r.direct {
		return "conversation_members"
	}
	return "chat_read_states"
}

// messageScope считывает область действия конференции или беседы из сохраненного сообщения.
//
// @Parameters:
//   - m: Подтвержденный манифест резервного копирования или состояние медиафайлов в
//     соответствии с их типом.
//
// @returns:
//   - result1: Идентификатор конференции или беседы, сохраненный в сообщении.
func (r *ChatRepository) messageScope(m chat.Message) string {
	if r.direct {
		return m.ConversationID
	}
	return m.ConferenceID
}

// setMessageScope устанавливает поля области действия сообщения для выбранного режима
// репозитория.
//
// @Parameters:
//   - m: Подтвержденный манифест резервного копирования или состояние медиафайлов в
//     соответствии с их типом.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
func (r *ChatRepository) setMessageScope(m *chat.Message, id string) {
	if r.direct {
		m.ConferenceID = ""
		m.ConversationID = id
	}
}

// setAttachmentScope устанавливает поля области действия вложений для выбранного режима
// репозитория.
//
// @Parameters:
//   - a: Сравниваемое значение вложения.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
func (r *ChatRepository) setAttachmentScope(a *chat.Attachment, id string) {
	if r.direct {
		a.ConferenceID = ""
		a.ConversationID = id
	}
}

// authorize проверяет текущие идентификационные данные и сохраненное состояние доступа перед
// определением области действия чата.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - locked: Указывает, должен ли запрос блокировать выбранную строку перед изменением.
//   - write: Выполняет ли этот доступ мутацию и требует ли авторизации на запись.
//
// @returns:
//   - result1: Полученный участник.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) authorize(tx *gorm.DB, user, id string, locked, write bool) (conferences.Participant, error) {
	if !r.direct {
		return authorizeChat(tx, user, id, locked, write)
	}
	if err := authorizeDirect(tx, user, id, locked); err != nil {
		return conferences.Participant{}, err
	}
	var visible int64
	if err := tx.Table("conversation_members").Where("conversation_id=? AND user_id=? AND hidden_at IS NULL", id, user).Count(&visible).Error; err != nil {
		return conferences.Participant{}, err
	}
	if visible != 1 {
		return conferences.Participant{}, apperrors.ErrForbidden
	}
	// В прямых беседах нет модератора.  ID используется только для обращения к потоку учетных
	// записей читателя.
	return conferences.Participant{ID: user, Role: conferences.ParticipantRole}, nil
}

// historyCutoff загружает границу истории для конкретного участника, используемую после очистки
// прямого чата.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) historyCutoff(tx *gorm.DB, user, id string) (int64, error) {
	if !r.direct {
		return 0, nil
	}
	var cutoff int64
	err := tx.Table("conversation_members").Select("history_cleared_through").Where("conversation_id=? AND user_id=?", id, user).Scan(&cutoff).Error
	return cutoff, err
}

// authorizeDirect проверяет, имеет ли учетная запись доступ к прямому чату.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - locked: Указывает, должен ли запрос блокировать выбранную строку перед изменением.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func authorizeDirect(tx *gorm.DB, user, id string, locked bool) error {
	var row struct{ ID string }
	q := tx.Table("conversations").Select("conversations.id").Where("conversations.id=? AND type IN('direct','group') AND deleted_at IS NULL", id)
	strength := "SHARE"
	if locked {
		strength = "UPDATE"
	}
	q = q.Clauses(clause.Locking{Strength: strength, Table: clause.Table{Name: "conversations"}})
	if err := q.Take(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrForbidden
		}
		return err
	}
	// Проверка членства в новом заявлении после получения блокировки диалога.  Все групповые
	// мутации блокируют эту же строку перед изменением активности участников.
	var count int64
	if err := tx.Table("conversation_members m").Joins("JOIN users u ON u.id=m.user_id").Where("m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL AND u.guest_conference_id IS NULL AND u.password_hash<>''", id, user).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return apperrors.ErrForbidden
	}
	return nil
}

// attachmentScope определяет область видимости чата, в которой разрешена операция с
// прикреплёнными файлами.
//
// @Parameters:
//   - a: Сравниваемое значение вложения.
//
// @returns:
//   - result1: Идентификатор конференции или диалога, сохранённый во вложении.
func (r *ChatRepository) attachmentScope(a chat.Attachment) string {
	if r.direct {
		return a.ConversationID
	}
	return a.ConferenceID
}

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/personal"

	"gorm.io/gorm"
)

// WithConversationDelivery подключает доставку сообщений к компоненту до того, как будут
// обработаны его операции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - deliver: Обратный вызов доставки вызывается только до тех пор, пока действителен
//     получатель или авторизация ресурса.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalRepository) WithConversationDelivery(ctx context.Context, id, user string, deliver func() error) (bool, error) {
	allowed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ ID string }
		result := tx.Raw("SELECT id FROM conversations WHERE id=? AND deleted_at IS NULL FOR SHARE", id).Scan(&row)
		if result.Error != nil || row.ID == "" {
			return result.Error
		}
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM conversation_members m
			JOIN users u ON u.id=m.user_id
			WHERE m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL
			AND u.guest_conference_id IS NULL AND u.password_hash<>''`, id, user).Scan(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return nil
		}
		allowed = true
		return deliver()
	})
	return allowed, err
}

// WithConversationMessageDelivery подключает доставку сообщений к компоненту до того, как будут
// обработаны его операции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - messageID: Идентификатор сообщения в рамках авторизованного диалога.
//   - deliver: Обратный вызов доставки вызывается только до тех пор, пока действителен
//     получатель или авторизация ресурса.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalRepository) WithConversationMessageDelivery(ctx context.Context, id, user, messageID string, deliver func(bool, int64) error) (bool, error) {
	allowed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ ID string }
		result := tx.Raw("SELECT id FROM conversations WHERE id=? AND deleted_at IS NULL FOR SHARE", id).Scan(&row)
		if result.Error != nil || row.ID == "" {
			return result.Error
		}
		var state struct {
			NotificationsEnabled  bool
			HistoryClearedThrough int64
		}
		result = tx.Raw(`SELECT m.notifications_enabled,m.history_cleared_through
			FROM conversation_members m JOIN users u ON u.id=m.user_id
			JOIN conversation_messages x ON x.conversation_id=m.conversation_id
			WHERE m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL
			AND m.hidden_at IS NULL AND u.guest_conference_id IS NULL AND u.password_hash<>''
			AND x.id=? AND x.sequence>m.history_cleared_through`, id, user, messageID).Scan(&state)
		if result.Error != nil || result.RowsAffected != 1 {
			return result.Error
		}
		allowed = true
		return deliver(state.NotificationsEnabled, state.HistoryClearedThrough)
	})
	return allowed, err
}

// WithConversationActionDelivery привязывает доставку действий в диалоге к компоненту до того,
// как будут выполнены его операции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - deliver: Обратный вызов доставки вызывается только до тех пор, пока действителен
//     получатель или авторизация ресурса.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalRepository) WithConversationActionDelivery(ctx context.Context, id, user, kind string, deliver func(*personal.Conversation) error) (bool, error) {
	if kind != "conversation.preferences.updated" && kind != "conversation.history.cleared" && kind != "conversation.hidden" {
		return false, errors.New("unsupported personal conversation event")
	}
	allowed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation struct{ ID, Type string }
		result := tx.Raw("SELECT id,type FROM conversations WHERE id=? AND type IN('direct','group') AND deleted_at IS NULL FOR SHARE", id).Scan(&conversation)
		if result.Error != nil || conversation.ID == "" {
			return result.Error
		}
		if conversation.Type == "group" && kind != "conversation.preferences.updated" {
			return nil
		}
		var member struct {
			HiddenAt              *time.Time
			HistoryClearedThrough int64
		}
		result = tx.Raw(`SELECT m.hidden_at,m.history_cleared_through FROM conversation_members m JOIN users u ON u.id=m.user_id
			WHERE m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL
			AND u.guest_conference_id IS NULL AND u.password_hash<>''`, id, user).Scan(&member)
		if result.Error != nil || result.RowsAffected != 1 {
			return result.Error
		}
		if kind == "conversation.hidden" {
			if member.HiddenAt == nil {
				return nil
			}
			allowed = true
			return deliver(&personal.Conversation{ID: id, Type: "direct", HistoryClearedThrough: member.HistoryClearedThrough})
		}
		if member.HiddenAt != nil {
			return nil
		}
		item, err := personalView(tx, user, id)
		if err != nil {
			return err
		}
		allowed = true
		return deliver(&item)
	})
	return allowed, err
}

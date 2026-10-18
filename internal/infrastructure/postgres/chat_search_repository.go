package postgres

import (
	"context"
	"strings"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"gorm.io/gorm"
)

// SearchMessages сохраняет действие конференции API при использовании общих правил видимости.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - scopeID: Идентификатор конференции или диалога, выбранный в зависимости от режима
//     репозитория.
//   - query: Текст для поиска или валидированные фильтры запросов.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Результирующая страница с сообщениями.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) SearchMessages(ctx context.Context, userID, scopeID, query, cursor string, limit int) (chat.MessagePage, error) {
	page, err := r.SearchMessagePage(ctx, userID, scopeID, query, cursor, limit)
	return chat.MessagePage{Items: page.Items, NextCursor: page.NextCursor}, err
}

// SearchMessagePage выполняет поиск по буквальному тексту в истории, видимой в данный момент
// для этого читателя.  Блокировка диалога SHARE в авторизованном режиме сериализует удаление
// групп и очистку истории.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - scopeID: Идентификатор конференции или диалога, выбранный в зависимости от режима
//     репозитория.
//   - query: Текст для поиска или валидированные фильтры запросов.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) SearchMessagePage(ctx context.Context, userID, scopeID, query, cursor string, limit int) (chat.Page, error) {
	page := chat.Page{Items: []chat.Message{}}
	query, err := chat.NormalizeSearchQuery(query)
	if err != nil {
		return page, err
	}
	if limit < 1 || limit > 50 {
		return page, apperrors.ErrInvalidInput
	}
	before, err := parseChatCursor(scopeID, cursor)
	if err != nil {
		return page, err
	}
	escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(query)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.authorize(tx, userID, scopeID, false, false); err != nil {
			return err
		}
		cutoff, err := r.historyCutoff(tx, userID, scopeID)
		if err != nil {
			return err
		}
		q := r.messageQuery(tx).Where(r.sql("chat_messages.conference_id=? AND chat_messages.sequence>? AND chat_messages.deleted_at IS NULL AND chat_messages.text ILIKE ? ESCAPE '!'"), scopeID, cutoff, "%"+escaped+"%")
		if before > 0 {
			q = q.Where(r.sql("chat_messages.sequence<?"), before)
		}
		if err := q.Order(r.sql("chat_messages.sequence DESC")).Limit(limit + 1).Find(&page.Items).Error; err != nil {
			return err
		}
		if len(page.Items) > limit {
			page.Items = page.Items[:limit]
			page.NextCursor = chatCursor(scopeID, page.Items[len(page.Items)-1].Sequence)
		}
		if err := r.decorateMessages(tx, page.Items, cutoff); err != nil {
			return err
		}
		if err := r.markImportant(tx, userID, scopeID, page.Items); err != nil {
			return err
		}
		state, err := r.readState(tx, userID, scopeID)
		page.UnreadCount, page.LastReadMessageID = state.UnreadCount, state.LastReadMessageID
		return err
	})
	return page, err
}

// ChatMessageContext возвращает до 25 сообщений из целевого списка и 25 более новых сообщений в
// хронологическом порядке с помощью обычного курсора для просмотра старых сообщений,
// привязанного к видимому окну.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - scopeID: Идентификатор конференции или диалога, выбранный в зависимости от режима
//     репозитория.
//   - messageID: Идентификатор сообщения в рамках авторизованного диалога.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ChatRepository) ChatMessageContext(ctx context.Context, userID, scopeID, messageID string) (chat.Page, error) {
	page := chat.Page{Items: []chat.Message{}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.authorize(tx, userID, scopeID, false, false); err != nil {
			return err
		}
		cutoff, err := r.historyCutoff(tx, userID, scopeID)
		if err != nil {
			return err
		}
		var target chat.Message
		if err := r.messages(tx).Where(r.sql("conference_id=? AND id=? AND deleted_at IS NULL AND sequence>?"), scopeID, messageID, cutoff).Take(&target).Error; err != nil {
			return mapNotFound(err)
		}
		older := []chat.Message{}
		if err := r.messageQuery(tx).Where(r.sql("chat_messages.conference_id=? AND chat_messages.sequence<=? AND chat_messages.sequence>?"), scopeID, target.Sequence, cutoff).
			Order(r.sql("chat_messages.sequence DESC")).Limit(26).Find(&older).Error; err != nil {
			return err
		}
		if len(older) > 25 {
			older = older[:25]
			page.NextCursor = chatCursor(scopeID, older[len(older)-1].Sequence)
		}
		newer := []chat.Message{}
		if err := r.messageQuery(tx).Where(r.sql("chat_messages.conference_id=? AND chat_messages.sequence>?"), scopeID, target.Sequence).
			Order(r.sql("chat_messages.sequence ASC")).Limit(25).Find(&newer).Error; err != nil {
			return err
		}
		for i := len(older) - 1; i >= 0; i-- {
			page.Items = append(page.Items, older[i])
		}
		page.Items = append(page.Items, newer...)
		if err := r.decorateMessages(tx, page.Items, cutoff); err != nil {
			return err
		}
		if err := r.markImportant(tx, userID, scopeID, page.Items); err != nil {
			return err
		}
		state, err := r.readState(tx, userID, scopeID)
		page.UnreadCount, page.LastReadMessageID = state.UnreadCount, state.LastReadMessageID
		return err
	})
	return page, err
}

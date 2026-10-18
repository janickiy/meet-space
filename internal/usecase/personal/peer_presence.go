package personal

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	domain "github.com/janickiy/meet-space/internal/domain/personal"
)

// ConversationReader возвращает проекцию диалога, авторизованную для субъекта.
type ConversationReader interface {
	// Get загружает запрошенный диалог в пределах авторизованной области действия вызывающего
	// абонента.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Полученная беседа.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Get(context.Context, string, string) (domain.Conversation, error)
}

// PresenceReader считывает интерактивное состояние без создания или продления сеанса.
type PresenceReader interface {
	// Online возвращает текущее состояние присутствия для запрошенных идентификаторов учетных
	// записей.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификаторы учетных записей или репозиторий учетных записей в
	//   соответствии с их типом.
	//
	// @returns:
	//   - result1: Результирующая карта [строка]bool.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Online(context.Context, []string) (map[string]bool, error)
}

// ReadPeerPresence авторизует прямой диалог перед поиском своего однорангового узла, выбранного
// сервером. Вызывающий абонент предоставляет нормализованные данные участника и диалога UUIDs.
// Неизвестное присутствие недоступно, никогда не отключается от сети; отмена проверяется до и
// после каждого чтения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conversations: Авторизованный считыватель сообщений.
//   - presence: Распределенный модуль чтения или записи данных о присутствии учетной записи.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Результирующее присутствие пользователя.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func ReadPeerPresence(ctx context.Context, conversations ConversationReader, presence PresenceReader, actor, id string) (domain.PeerPresence, error) {
	if ctx.Err() != nil {
		return domain.PeerPresence{}, apperrors.ErrUnavailable
	}
	item, err := conversations.Get(ctx, actor, id)
	if err != nil {
		return domain.PeerPresence{}, err
	}
	if item.ID != id || item.Type != "direct" || item.Peer == nil {
		return domain.PeerPresence{}, apperrors.ErrForbidden
	}
	peer, err := chat.UUID(item.Peer.ID)
	if err != nil || peer == actor {
		return domain.PeerPresence{}, apperrors.ErrForbidden
	}
	if ctx.Err() != nil || presence == nil {
		return domain.PeerPresence{}, apperrors.ErrUnavailable
	}
	lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	statuses, err := presence.Online(lookup, []string{peer})
	if err != nil || lookup.Err() != nil || ctx.Err() != nil {
		return domain.PeerPresence{}, apperrors.ErrUnavailable
	}
	online, known := statuses[peer]
	if !known {
		return domain.PeerPresence{}, apperrors.ErrUnavailable
	}
	return domain.PeerPresence{ConversationID: id, PeerID: peer, Online: online}, nil
}

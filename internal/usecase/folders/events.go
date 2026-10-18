package folders

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

type Bus interface {
	// Publish — при публикации сохраненные изменения в закрытой папке публикуются через настроенный
	// транспорт событий или команд.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Publish(context.Context, string, realtime.Envelope) error
}

// Events — события передают только идентификаторы аннулирования в глобальный канал владельца папки.
// Другие пользователи не должны становиться получателями информации о принадлежности элемента.
type Events struct{ Bus Bus }

// PublishFolder публикует зафиксированные изменения в папке только в канале реального времени
// аккаунта владельца.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - folderID: Идентификатор личной папки действующего аккаунта.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (e *Events) PublishFolder(ctx context.Context, userID, kind, folderID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	user, err := uuid.Parse(userID)
	if err != nil || user == uuid.Nil || e == nil || e.Bus == nil {
		return errors.New("invalid folder event recipient or bus")
	}
	data := map[string]string{}
	switch kind {
	case "folder.created", "folder.updated", "folder.deleted", "folder.items.updated":
		id, err := uuid.Parse(folderID)
		if err != nil || id == uuid.Nil {
			return errors.New("invalid folder event identifier")
		}
		data["folderId"] = id.String()
	case "folder.reordered":
		if folderID != "" {
			return errors.New("reorder event must not contain an item")
		}
	default:
		return errors.New("unsupported folder event")
	}
	return e.Bus.Publish(ctx, user.String(), realtime.Event(kind, "", data))
}

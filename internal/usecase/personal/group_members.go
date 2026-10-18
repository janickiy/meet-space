package personal

import (
	"context"
	"time"

	domain "github.com/janickiy/meet-space/internal/domain/personal"
)

// GroupMemberReader авторизует актора перед возвратом текущих участников группы.
type GroupMemberReader interface {
	// GroupMembers возвращает список участников группы, доступных для действующей учетной
	// записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор учетной записи, выполняющей операцию.
	//   - arg3: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Собранные значения участников.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GroupMembers(context.Context, string, string) ([]domain.Member, error)
}

// ReadGroupMembers дополняет список авторизованных участников опциональным отображением
// присутствия. В отличие от прямого запроса на одноранговое присутствие, список участников
// остается полезным, когда присутствие недоступно: статусы "неизвестно" сохраняют свое
// существующее значение nil.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - groups: Авторизованный пользователь для чтения членов группы.
//   - presence: Распределенный модуль чтения или записи данных о присутствии учетной записи.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Собранные значения участников.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func ReadGroupMembers(ctx context.Context, groups GroupMemberReader, presence PresenceReader, actor, id string) ([]domain.Member, error) {
	items, err := groups.GroupMembers(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if presence != nil && len(items) > 0 {
		ids := make([]string, 0, len(items))
		for _, member := range items {
			ids = append(ids, member.ID)
		}
		lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
		online, presenceErr := presence.Online(lookup, ids)
		cancel()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if presenceErr == nil {
			for i := range items {
				if value, known := online[items[i].ID]; known {
					items[i].Online = &value
				}
			}
		}
	}
	return items, nil
}

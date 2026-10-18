package recorder

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/janickiy/meet-space/internal/domain/records"
)

type RetentionRepository interface {
	// ExpireNextRecording удаляет одну подходящую запись, вызывая очистку хранилища с учетом
	// прав собственности репозитория.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификаторы, которые уже были использованы при повторном извлечении.
	//   - arg3: Обратный вызов, удаляющий объекты хранения, принадлежащие авторизованной
	//   записи.
	//
	// @returns:
	//   - result1: Возвращает количество, размер или числовое значение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ExpireNextRecording(context.Context, []int64, func(context.Context, records.Record) error) (int64, error)
}

type RecordingRemover interface {
	// RemoveRecording удаляет только проверенные префиксы приватных объектов, относящиеся к
	// выбранной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Запись и сохраненные метаданные ее жизненного цикла.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RemoveRecording(context.Context, records.Record) error
}

// RunRetention удаляет артефакты с истекшим сроком хранения при запуске и каждую минуту.  Если
// удаление не удалось, его можно повторить; завершенные метаданные сохраняются для аудита.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - storage: Зависимость от приватного объекта или хранилища файлов.
func RunRetention(ctx context.Context, repo RetentionRepository, storage RecordingRemover) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		count, err := SweepRetention(ctx, repo, storage)
		if err != nil && ctx.Err() == nil {
			slog.Error("recording retention failed", "event_type", "recording.retention_failed", "error", err)
		} else if count > 0 {
			slog.Info("expired recordings removed", "event_type", "recording.retention_completed", "count", count)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// SweepRetention ограничивает каждую операцию хранения и каждый проход.  Блокировка строк
// предотвращает одновременное удаление одной и той же записи двумя репликами API.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - storage: Зависимость от приватного объекта или хранилища файлов.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func SweepRetention(ctx context.Context, repo RetentionRepository, storage RecordingRemover) (int, error) {
	count := 0
	var attempted []int64
	var failures error
	for len(attempted) < 100 && ctx.Err() == nil {
		operation, cancel := context.WithTimeout(ctx, 2*time.Minute)
		id, err := repo.ExpireNextRecording(operation, attempted, storage.RemoveRecording)
		cancel()
		if id == 0 {
			return count, errors.Join(failures, err)
		}
		attempted = append(attempted, id)
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		count++
	}
	return count, errors.Join(failures, ctx.Err())
}

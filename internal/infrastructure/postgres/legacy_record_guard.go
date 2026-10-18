package postgres

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// LegacyConferenceAllowed проверяет, может ли устаревшая операция записи получить доступ к
// указанной записи конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) LegacyConferenceAllowed(ctx context.Context, id string) error {
	var count int64
	if err := r.db.WithContext(ctx).Table("conferences").Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return apperrors.New(apperrors.ErrForbidden, "use authenticated conference recording endpoints")
	}
	return nil
}

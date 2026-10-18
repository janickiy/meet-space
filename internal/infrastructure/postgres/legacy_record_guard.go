package postgres

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// LegacyConferenceAllowed checks whether a legacy recording operation may access the referenced
// conference recording.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

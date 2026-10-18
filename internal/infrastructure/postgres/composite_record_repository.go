package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/janickiy/meet-space/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ListActiveComposite возвращает активные составные записи, чтобы работник мог сверить их или
// восстановить процесс обработки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Собранные значения записей.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) ListActiveComposite(ctx context.Context) ([]records.Record, error) {
	var result []records.Record
	err := r.db.WithContext(ctx).Where("mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status IN ?", []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}).Order("created_at ASC").Limit(100).Find(&result).Error
	return result, err
}

// ClaimComposite запрашивает аренду составной записи с версионным доступом для одного
// работника.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//   - workerID: Идентификатор работника, которому принадлежит эта операция.
//   - ttl: Срок действия, назначенный для временного значения или записи о владельце.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) ClaimComposite(ctx context.Context, id, token, workerID string, ttl time.Duration) (bool, error) {
	result := r.db.WithContext(ctx).Model(&records.Record{}).
		Where("uuid = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status IN ?", id, []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}).
		Where("recorder_token IS NULL OR recorder_lease_until < clock_timestamp()").
		Updates(map[string]any{"recorder_token": token, "recorder_lease_until": gorm.Expr("clock_timestamp() + (? * interval '1 millisecond')", ttl.Milliseconds()), "worker_id": workerID})
	return result.RowsAffected == 1, result.Error
}

// RenewComposite продлевает аренду составной записи только в том случае, если её работник и
// версия аренды по-прежнему совпадают.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//   - ttl: Срок действия, назначенный для временного значения или записи о владельце.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) RenewComposite(ctx context.Context, id, token string, ttl time.Duration) (bool, error) {
	result := r.db.WithContext(ctx).Model(&records.Record{}).
		Where("uuid = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND recorder_token = ? AND recorder_lease_until > clock_timestamp() AND status IN ?", id, token, []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}).
		Update("recorder_lease_until", gorm.Expr("clock_timestamp() + (? * interval '1 millisecond')", ttl.Milliseconds()))
	return result.RowsAffected == 1, result.Error
}

// ReleaseComposite освобождает только ту аренду составной записи, которой он владеет в данный
// момент.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) ReleaseComposite(ctx context.Context, id, token string) error {
	return r.db.WithContext(ctx).Model(&records.Record{}).Where("uuid = ? AND recorder_token = ?", id, token).Updates(map[string]any{"recorder_token": nil, "recorder_lease_until": nil}).Error
}

// TransitionComposite изменяет состояние составной записи только для текущей аренды.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//   - status: Состояние жизненного цикла или статус ответа на запись.
//   - cause: Основная ошибка, из-за которой операция завершилась неудачно.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) TransitionComposite(ctx context.Context, id, token, status string, cause error) error {
	var from []string
	updates := map[string]any{"status": status}
	switch status {
	case records.StatusRecording:
		from = []string{records.StatusStarting}
		updates["started_at"] = gorm.Expr("clock_timestamp()")
	case records.StatusFinalizing:
		from = []string{records.StatusStopping, records.StatusFinalizing, records.StatusUploading}
	case records.StatusUploading:
		from = []string{records.StatusFinalizing, records.StatusUploading}
	case records.StatusFailed:
		from = []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}
		updates["ended_at"] = gorm.Expr("clock_timestamp()")
		if cause != nil {
			updates["error_message"] = cause.Error()
		}
	default:
		return records.ErrRecordStateChanged
	}
	result := r.db.WithContext(ctx).Model(&records.Record{}).Where("uuid = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND recorder_token = ? AND recorder_lease_until > clock_timestamp() AND status IN ?", id, token, from).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return records.ErrRecordStateChanged
	}
	return nil
}

// SaveCompositeArtifacts коммитит составные артефакты только для той генерации записи, которой
// по-прежнему владеет работник.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//   - final: состояние файла записи, обработанное в ходе этой операции.
//   - preview: Необязательные метаданные файла предварительного просмотра, связанные с записью.
//   - segments: Упорядоченная расшифровка или сегменты медиаданных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) SaveCompositeArtifacts(ctx context.Context, id, token string, final records.RecordFile, preview *records.RecordFile, segments []records.RecordSegment) error {
	return r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			var record records.Record
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND recorder_token = ? AND recorder_lease_until > clock_timestamp() AND status = ?", id, token, records.StatusUploading).First(&record).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return records.ErrRecordStateChanged
				}
				return err
			}
			final.RecordID = record.ID
			for i := range final.Related {
				final.Related[i].RecordID = record.ID
				if err := upsertRecordFile(tx, &final.Related[i]); err != nil {
					return err
				}
			}
			if err := upsertRecordFile(tx, &final); err != nil {
				return err
			}
			updates := map[string]any{"status": records.StatusReady, "storage_bucket": final.Bucket, "storage_object_key": final.ObjectKey, "size_bytes": final.SizeBytes, "duration_sec": final.DurationSec, "ended_at": gorm.Expr("clock_timestamp()"), "error_message": nil}
			var metadata struct {
				TimelineOriginNS int64 `json:"timelineOriginNs"`
			}
			if json.Unmarshal(final.MetadataJSON, &metadata) == nil && metadata.TimelineOriginNS > 0 {
				updates["media_started_at"] = time.Unix(0, metadata.TimelineOriginNS).UTC()
			}
			if preview != nil {
				preview.RecordID = record.ID
				if err := upsertRecordFile(tx, preview); err != nil {
					return err
				}
				updates["preview_object_key"] = preview.ObjectKey
			}
			for i := range segments {
				segments[i].RecordID = record.ID
				if err := upsertRecordSegment(tx, &segments[i]); err != nil {
					return err
				}
			}
			result := tx.Model(&records.Record{}).Where("id = ? AND recorder_token = ? AND recorder_lease_until > clock_timestamp()", record.ID, token).Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return records.ErrRecordStateChanged
			}
			return nil
		})
}

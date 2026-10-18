package postgres

import (
	"context"
	"errors"

	"github.com/janickiy/meet-space/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Неделя начинается с момента завершения, включая время обработки и загрузки.  Для исторических
// записей терминалов без ended_at используется последняя зафиксированная временная метка
// терминала.
const expiredRecording = `status IN ('ready','partial_ready','failed','cancelled')
	AND COALESCE(ended_at,stopped_at,updated_at,created_at) <= clock_timestamp() - INTERVAL '7 days'`

// availableRecordings формирует запрос на запись с фильтрацией доступа, используемый вызывающей
// стороной.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданная или загруженная база данных.
func availableRecordings(db *gorm.DB) *gorm.DB {
	return db.Where("deleted_at IS NULL AND NOT (" + expiredRecording + ")")
}

// ExpireNextRecording блокирует строку посредством идемпотентного удаления объекта.  Маркер
// удаления и ссылки на артефакты фиксируются только после успешного сохранения.  В случае сбоя
// процесса или частичного сбоя при сохранении запись можно будет восстановить.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - attempted: Идентификаторы, которые уже были использованы при повторном извлечении.
//   - remove: Обратный вызов, удаляющий объекты хранения, принадлежащие авторизованной записи.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *RecordRepository) ExpireNextRecording(ctx context.Context, attempted []int64, remove func(context.Context, records.Record) error) (int64, error) {
	var id int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record records.Record
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("deleted_at IS NULL AND (" + expiredRecording + ")").
			Where("recorder_lease_until IS NULL OR recorder_lease_until <= clock_timestamp()")
		if len(attempted) > 0 {
			query = query.Where("id NOT IN ?", attempted)
		}
		err := query.Order("COALESCE(ended_at,stopped_at,updated_at,created_at), id").Take(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		id = record.ID
		return removeRecordingArtifacts(ctx, tx, record, remove)
	})
	return id, err
}

// removeRecordingArtifacts удаляет метаданные артефакта базы данных для удаляемой записи.
//
// Вызывающая сторона удерживает блокировку строки записи. Удаление из хранилища является
// идемпотентной операцией, поэтому в случае частичного сбоя или неудачной фиксации можно
// безопасно повторить попытку удаления по любому из путей.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - tx: Текущая транзакция в базе данных.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//   - remove: Обратный вызов, удаляющий объекты хранения, принадлежащие авторизованной записи.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func removeRecordingArtifacts(ctx context.Context, tx *gorm.DB, record records.Record, remove func(context.Context, records.Record) error) error {
	if err := remove(ctx, record); err != nil {
		return err
	}
	if err := tx.Where("record_id = ?", record.ID).Delete(&records.RecordFile{}).Error; err != nil {
		return err
	}
	if err := tx.Where("record_id = ?", record.ID).Delete(&records.RecordSegment{}).Error; err != nil {
		return err
	}
	return tx.Model(&records.Record{}).Where("id = ?", record.ID).Updates(map[string]any{
		"deleted_at": gorm.Expr("clock_timestamp()"), "storage_object_key": nil,
		"preview_object_key": nil, "size_bytes": nil,
	}).Error
}

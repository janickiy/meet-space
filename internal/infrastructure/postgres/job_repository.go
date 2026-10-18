package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"gorm.io/gorm"
)

// JobRepository сохраняет независимую очередь продуктов без изменения медиакоманд RabbitMQ.
type JobRepository struct{ db *gorm.DB }

// NewJobRepository создает репозиторий заданий с зависимостями и ограничениями, необходимыми
// для выполнения задания.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий заданий.
func NewJobRepository(db *gorm.DB) *JobRepository { return &JobRepository{db: db} }

// Claim — требование атомарно получает текущую арендуемую запись задания или запись о владении.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - lease: Идентификатор текущего резервирования, используемый для изоляции устаревших
//     рабочих процессов.
//
// @returns:
//   - result1: Результирующее задание.
//   - result2: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *JobRepository) Claim(ctx context.Context, kind string, lease time.Duration) (jobs.Job, bool, error) {
	var job jobs.Job
	result := r.db.WithContext(ctx).Raw(`UPDATE background_jobs SET state='processing',attempts=attempts+1,
        lease_token=?::uuid,lease_until=clock_timestamp()+(? * interval '1 millisecond'),updated_at=clock_timestamp()
        WHERE id=(SELECT id FROM background_jobs WHERE kind=? AND available_at<=clock_timestamp()
          AND (state='queued' OR (state='processing' AND lease_until<clock_timestamp()))
          ORDER BY available_at,id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING *`, uuid.NewString(), lease.Milliseconds(), kind).Scan(&job)
	return job, result.RowsAffected != 0, result.Error
}

// Finish фиксирует завершение аренды операции задания.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - state: Текущее состояние или обновлённое валидированное состояние.
//   - code: Ограниченный публичный статус или код ошибки.
//   - retryAt: Следующее допустимое время для повторной попытки; nil означает, что повторная
//     попытка не запланирована.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *JobRepository) Finish(ctx context.Context, job jobs.Job, state, code string, retryAt *time.Time) error {
	query := `UPDATE background_jobs SET state=?,error_code=NULLIF(?,''),updated_at=clock_timestamp(),
        lease_token=NULL,lease_until=NULL,finished_at=clock_timestamp()
        WHERE id=?::uuid AND state='processing' AND lease_token=?::uuid AND lease_until>clock_timestamp()`
	args := []any{state, code, job.ID, job.LeaseToken}
	if retryAt != nil {
		query = `UPDATE background_jobs SET state='queued',error_code=NULLIF(?,''),available_at=?,updated_at=clock_timestamp(),
            lease_token=NULL,lease_until=NULL,finished_at=NULL
            WHERE id=?::uuid AND state='processing' AND lease_token=?::uuid AND lease_until>clock_timestamp()`
		args = []any{code, retryAt.UTC(), job.ID, job.LeaseToken}
	}
	result := r.db.WithContext(ctx).Exec(query, args...)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return jobs.ErrLeaseLost
	}
	return nil
}

// Counts возвращает количество элементов в ограниченной очереди, используемое для мониторинга
// операционной деятельности.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Собранные значения количества.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *JobRepository) Counts(ctx context.Context) ([]jobs.Count, error) {
	items := []jobs.Count{}
	err := r.db.WithContext(ctx).Raw(`SELECT kind,state,count(*) AS count FROM background_jobs
        WHERE state IN ('queued','processing','failed') GROUP BY kind,state`).Scan(&items).Error
	return items, err
}

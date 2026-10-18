package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	domain "github.com/janickiy/meet-space/internal/domain/content"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"time"
)

// contentLease проверяет аренду и генерацию задания перед фиксацией результатов обработки
// контента.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func contentLease(tx *gorm.DB, job jobs.Job) error {
	var id string
	if err := tx.Raw(`SELECT id FROM background_jobs WHERE id=? AND state='processing' AND lease_token=? AND lease_until>clock_timestamp() FOR UPDATE`, job.ID, job.LeaseToken).Scan(&id).Error; err != nil {
		return err
	}
	if id == "" {
		return jobs.ErrLeaseLost
	}
	// Блокировка конференции перед обработкой сущностей контента, сопоставление ресурсоемких
	// операций POST и внешних ключей. KEY SHARE позволяет выполнять параллельные рабочие
	// коммиты, предотвращая при этом отзыв доступа.
	var cid string
	if err := tx.Raw("SELECT id FROM conferences WHERE id=? FOR KEY SHARE", job.ConferenceID).Scan(&cid).Error; err != nil {
		return err
	}
	if cid == "" {
		return jobs.ErrSkip
	}
	return nil
}

// workerSource загружает текущие записи и входные данные контента для фонового задания с
// арендой.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - rid: Идентификатор записи, используемый текущей операцией.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Результирующий источник записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func workerSource(tx *gorm.DB, rid, cid string) (domain.RecordingSource, error) {
	var source domain.RecordingSource
	err := tx.Raw(`SELECT uuid AS recording_id,platform_conference_id AS conference_id,storage_object_key AS object_key,COALESCE(duration_sec,0) AS duration_sec,COALESCE(size_bytes,0) AS size_bytes FROM record WHERE uuid=? AND platform_conference_id=? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status='ready' AND deleted_at IS NULL`, rid, cid).Scan(&source).Error
	if err == nil && source.RecordingID == "" {
		err = jobs.ErrSkip
	}
	return source, err
}

// StartTranscript запрашивает создание текущей расшифровки до начала ресурсоемкой обработки
// поставщиком.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: Результирующая расшифровка.
//   - result2: Результирующий источник записи.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ContentRepository) StartTranscript(ctx context.Context, job jobs.Job) (domain.Transcript, domain.RecordingSource, error) {
	var t domain.Transcript
	var source domain.RecordingSource
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := contentLease(tx, job); err != nil {
			return err
		}
		var err error
		source, err = workerSource(tx, job.EntityID, job.ConferenceID)
		if err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO transcripts(conference_id,recording_id,status,generation) VALUES (?,?,'queued',?) ON CONFLICT(recording_id) DO NOTHING`, job.ConferenceID, job.EntityID, job.Version).Error; err != nil {
			return err
		}
		if err = tx.Table("transcripts").Clauses(clause.Locking{Strength: "UPDATE"}).Where("recording_id=? AND conference_id=?", job.EntityID, job.ConferenceID).Take(&t).Error; err != nil {
			return err
		}
		if t.Generation != job.Version || t.Status == domain.Ready {
			return jobs.ErrSkip
		}
		t.Status = domain.Processing
		return tx.Table("transcripts").Where("id=?", t.ID).Updates(map[string]any{"status": domain.Processing, "updated_at": time.Now().UTC(), "error_code": nil, "error_message": nil}).Error
	})
	return t, source, err
}

// readyIntegration проверяет, включен ли и доступен ли запрошенный поставщик контента.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - event: Доверенное событие для публикации или обработки.
//   - entity: Идентификатор сущности или категория сущности, связанные с заданием на обработку.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - rid: Идентификатор записи, используемый текущей операцией.
//   - tid: Идентификатор текущей генерации расшифровки.
//   - sid: Текущий идентификатор сводной информации или сеанса, определяемый операцией.
//   - version: Версия или значение ограждения, используемое для предотвращения устаревания прав
//     собственности.
//   - attempts: Максимально допустимое количество попыток выполнения задания или текущее
//     количество повторных попыток.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func readyIntegration(tx *gorm.DB, event, entity, cid, rid, tid, sid string, version int64, attempts int) error {
	payload, _ := json.Marshal(map[string]string{"event": event, "recordingId": rid, "transcriptId": tid, "summaryId": sid})
	return tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key,max_attempts) VALUES ('integrations.event',?,?,?,?::jsonb,?,?) ON CONFLICT(dedup_key) DO NOTHING`, entity, cid, version, string(payload), "integrations:"+event+":"+entity+":"+formatGeneration(version), attempts).Error
}

// formatGeneration форматирует идентификатор генерации контента, используемый фоновыми
// задачами.
//
// @Parameters:
//   - generation: Идентификатор генерации контента, используемый для отклонения устаревших
//     результатов.
//
// @returns:
//   - result1: Стабильный сериализованный идентификатор генерации контента.
func formatGeneration(generation int64) string { return strconv.FormatInt(generation, 10) }

// SaveTranscript фиксирует выходные данные транскрипта только для текущей генерации записи и
// аренды задания.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - t: Test дескриптор, используемый для утверждений, очистки и сообщений об ошибках.
//   - result: Подтвержденный результат обработки или операции.
//   - provider: Идентификатор провайдера, выбранный оператором, или фиксированная техническая
//     метка провайдера.
//   - ai: Нужно ли запланировать создание сводной таблицы для расшифровки.
//   - attempts: Максимально допустимое количество попыток выполнения задания или текущее
//     количество повторных попыток.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ContentRepository) SaveTranscript(ctx context.Context, job jobs.Job, t domain.Transcript, result domain.TranscriptionResult, provider string, ai bool, attempts int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := contentLease(tx, job); err != nil {
			return err
		}
		if _, err := workerSource(tx, t.RecordingID, t.ConferenceID); err != nil {
			return err
		}
		var current domain.Transcript
		if err := tx.Table("transcripts").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND generation=?", t.ID, job.Version).Take(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return jobs.ErrLeaseLost
			}
			return err
		}
		if current.Status == domain.Ready {
			return nil
		}
		if err := tx.Table("transcript_segments").Where("transcript_id=?", t.ID).Delete(&domain.Segment{}).Error; err != nil {
			return err
		}
		segments := make([]domain.Segment, len(result.Segments))
		for i, seg := range result.Segments {
			seg.ID = uuid.NewString()
			seg.TranscriptID = t.ID
			seg.Ordinal = i
			seg.SpeakerID = nil
			segments[i] = seg
		}
		if len(segments) > 0 {
			if err := tx.Table("transcript_segments").CreateInBatches(segments, 200).Error; err != nil {
				return err
			}
		}
		if err := reconcileLiveTranscript(tx, t); err != nil {
			return err
		}
		if err := tx.Table("transcripts").Where("id=? AND generation=?", t.ID, job.Version).Updates(map[string]any{"status": domain.Ready, "language": result.Language, "provider": provider, "error_code": nil, "error_message": nil, "processed_at": time.Now().UTC(), "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		if ai {
			var summary domain.Summary
			if err := tx.Raw(`INSERT INTO meeting_summaries(conference_id,transcript_id,transcript_generation,status,generation) VALUES (?,?,?,'queued',1)
			 ON CONFLICT(transcript_id) DO UPDATE SET transcript_generation=EXCLUDED.transcript_generation,status='queued',generation=meeting_summaries.generation+1,error_code=NULL,error_message=NULL,updated_at=now()
			 RETURNING *`, t.ConferenceID, t.ID, t.Generation).Scan(&summary).Error; err != nil {
				return err
			}
			if err := insertContentJob(tx, "content.summarize", t.ID, t.ConferenceID, summary.Generation, map[string]any{}, attempts); err != nil {
				return err
			}
		}
		return readyIntegration(tx, "transcript.ready", t.ID, t.ConferenceID, t.RecordingID, t.ID, "", t.Generation, attempts)
	})
}

// StartSummary запрашивает текущее создание сводной таблицы перед вызовом настроенного
// провайдера.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: Полученная сводная таблица.
//   - result2: Собранные значения сегментов.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ContentRepository) StartSummary(ctx context.Context, job jobs.Job) (domain.Summary, []domain.Segment, error) {
	var s domain.Summary
	segments := []domain.Segment{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := contentLease(tx, job); err != nil {
			return err
		}
		var t domain.Transcript
		if err := tx.Table("transcripts").Where("id=? AND conference_id=? AND status='ready'", job.EntityID, job.ConferenceID).Take(&t).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return jobs.ErrSkip
			}
			return err
		}
		if _, err := workerSource(tx, t.RecordingID, job.ConferenceID); err != nil {
			return err
		}
		if err := tx.Table("meeting_summaries").Clauses(clause.Locking{Strength: "UPDATE"}).Where("transcript_id=? AND transcript_generation=? AND generation=?", t.ID, t.Generation, job.Version).Take(&s).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return jobs.ErrSkip
			}
			return err
		}
		if s.Status == domain.Ready {
			return jobs.ErrSkip
		}
		if err := tx.Table("transcript_segments").Where("transcript_id=?", t.ID).Order("ordinal").Limit(10001).Find(&segments).Error; err != nil {
			return err
		}
		s.Status = domain.Processing
		return tx.Table("meeting_summaries").Where("id=?", s.ID).Updates(map[string]any{"status": domain.Processing, "error_code": nil, "error_message": nil, "updated_at": time.Now().UTC()}).Error
	})
	return s, segments, err
}

// SaveSummary сохраняет валидированную сводную таблицу только для текущего создания расшифровки
// и аренды задания.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - s: Состояние сводной таблицы, обработанное в ходе этой операции.
//   - output: Целевой или сгенерированный результат, необходимый для выполнения этой операции.
//   - provider: Идентификатор провайдера, выбранный оператором, или фиксированная техническая
//     метка провайдера.
//   - model: Настроенный идентификатор модели провайдера.
//   - prompt: Версионированный идентификатор модели-инструкции, используемый в сводной таблице.
//   - schema: Ожидаемая версия выходной схемы.
//   - attempts: Максимально допустимое количество попыток выполнения задания или текущее
//     количество повторных попыток.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ContentRepository) SaveSummary(ctx context.Context, job jobs.Job, s domain.Summary, output domain.SummaryOutput, provider, model, prompt, schema string, attempts int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := contentLease(tx, job); err != nil {
			return err
		}
		var t domain.Transcript
		if err := tx.Table("transcripts").Where("id=? AND generation=? AND status='ready'", s.TranscriptID, s.TranscriptGeneration).Take(&t).Error; err != nil {
			return jobs.ErrLeaseLost
		}
		if _, err := workerSource(tx, t.RecordingID, t.ConferenceID); err != nil {
			return err
		}
		data, err := json.Marshal(output)
		if err != nil {
			return err
		}
		search := []string{output.Summary}
		search = append(search, output.KeyPoints...)
		search = append(search, output.Topics...)
		for _, action := range output.ActionItems {
			search = append(search, action.Text)
		}
		res := tx.Table("meeting_summaries").Where("id=? AND generation=? AND transcript_generation=?", s.ID, job.Version, t.Generation).Updates(map[string]any{"status": domain.Ready, "summary": output.Summary, "structured_output": string(data), "search_text": strings.Join(search, "\n"), "provider": provider, "model": model, "prompt_version": prompt, "schema_version": schema, "error_code": nil, "error_message": nil, "processed_at": time.Now().UTC(), "updated_at": time.Now().UTC()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return jobs.ErrLeaseLost
		}
		return readyIntegration(tx, "summary.ready", s.ID, s.ConferenceID, t.RecordingID, t.ID, s.ID, s.Generation, attempts)
	})
}

// FailJob регистрирует сбой задания с ограничением, не раскрывая ответы провайдера и не заменяя
// более новую версию.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - code: Ограниченный публичный статус или код ошибки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ContentRepository) FailJob(ctx context.Context, job jobs.Job, code string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := contentLease(tx, job); err != nil {
			return err
		}
		updates := map[string]any{"status": domain.Failed, "error_code": code, "error_message": "Processing failed; retry is subject to limits", "updated_at": time.Now().UTC()}
		if job.Kind == "content.transcribe" {
			res := tx.Table("transcripts").Where("recording_id=? AND conference_id=? AND generation=? AND status <> 'ready'", job.EntityID, job.ConferenceID, job.Version).Updates(updates)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 || code == "processing_disabled" {
				return nil
			}
			var t domain.Transcript
			if err := tx.Table("transcripts").Where("recording_id=?", job.EntityID).Take(&t).Error; err != nil {
				return err
			}
			return contentFailureEvent(tx, "transcript.failed", t.ID, t.ConferenceID, t.RecordingID, t.ID, "", code, t.Generation, job.MaxAttempts)
		}
		if job.Kind == "content.summarize" {
			res := tx.Table("meeting_summaries").Where("transcript_id=? AND conference_id=? AND generation=? AND status <> 'ready'", job.EntityID, job.ConferenceID, job.Version).Updates(updates)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 || code == "processing_disabled" {
				return nil
			}
			var s domain.Summary
			var t domain.Transcript
			if err := tx.Table("meeting_summaries").Where("transcript_id=?", job.EntityID).Take(&s).Error; err != nil {
				return err
			}
			if err := tx.Table("transcripts").Where("id=?", job.EntityID).Take(&t).Error; err != nil {
				return err
			}
			return contentFailureEvent(tx, "summary.failed", s.ID, s.ConferenceID, t.RecordingID, t.ID, s.ID, code, s.Generation, job.MaxAttempts)
		}
		return jobs.ErrSkip
	})
}

// contentFailureEvent формирует безопасное событие сбоя при обработке контента без указания
// деталей ответа провайдера.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - event: Доверенное событие для публикации или обработки.
//   - entity: Идентификатор сущности или категория сущности, связанные с заданием на обработку.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - rid: Идентификатор записи, используемый текущей операцией.
//   - tid: Идентификатор текущей генерации расшифровки.
//   - sid: Текущий идентификатор сводной информации или сеанса, определяемый операцией.
//   - code: Ограниченный публичный статус или код ошибки.
//   - version: Версия или значение ограждения, используемое для предотвращения устаревания прав
//     собственности.
//   - attempts: Максимально допустимое количество попыток выполнения задания или текущее
//     количество повторных попыток.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func contentFailureEvent(tx *gorm.DB, event, entity, cid, rid, tid, sid, code string, version int64, attempts int) error {
	if attempts < 1 {
		attempts = 5
	}
	data, _ := json.Marshal(map[string]string{"event": event, "recordingId": rid, "transcriptId": tid, "summaryId": sid, "reason": code})
	return tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,version,payload,dedup_key,max_attempts)
	 SELECT 'integrations.event',?::uuid,c.id,c.owner_id,?,?::jsonb,?,? FROM conferences c JOIN record r ON r.platform_conference_id=c.id
	 WHERE c.id=? AND r.uuid=? AND r.deleted_at IS NULL ON CONFLICT(dedup_key) DO NOTHING`, entity, version, string(data), "integrations:"+event+":"+entity+":"+formatGeneration(version), attempts, cid, rid).Error
}

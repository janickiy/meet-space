package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	domain "github.com/janickiy/meet-space/internal/domain/content"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// ContentRepository сохраняет расшифровки и валидированные результаты AI, проверяя доступ к
// историческим конференциям.
type ContentRepository struct{ db *gorm.DB }

// NewContentRepository создает хранилище контента с зависимостями и ограничениями, необходимыми
// для записи стенограмм и кратких обзоров.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданное или загруженное хранилище контента.
func NewContentRepository(db *gorm.DB) *ContentRepository { return &ContentRepository{db: db} }

// contentAccess проверяет членство в конференции и видимость записей перед чтением
// сгенерированного контента.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - rid: Идентификатор записи, используемый текущей операцией.
//
// @returns:
//   - result1: Результирующий источник записи.
//   - result2: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func contentAccess(db *gorm.DB, userID, cid, rid string) (domain.RecordingSource, bool, error) {
	p, err := findMembership(db, cid, userID)
	if err != nil {
		return domain.RecordingSource{}, false, membershipError(err)
	}
	if !p.CanReadHistory() {
		return domain.RecordingSource{}, false, apperrors.ErrForbidden
	}
	var source domain.RecordingSource
	err = db.Raw(`SELECT uuid AS recording_id,platform_conference_id AS conference_id,storage_object_key AS object_key,COALESCE(duration_sec,0) AS duration_sec,COALESCE(size_bytes,0) AS size_bytes FROM record WHERE uuid=? AND platform_conference_id=? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status='ready' AND deleted_at IS NULL`, rid, cid).Scan(&source).Error
	if err == nil && source.RecordingID == "" {
		err = apperrors.ErrNotFound
	}
	return source, p.Role == conferences.Owner || p.Role == conferences.CoHost, err
}

// Transcript возвращает текущие метаданные стенограммы и возможности провайдера без запуска
// процесса расшифровки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - rid: Идентификатор записи, используемый текущей операцией.
//
// @returns:
//   - result1: Созданная или загруженная стенограмма.
//   - result2: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ContentRepository) Transcript(ctx context.Context, userID, cid, rid string) (*domain.Transcript, bool, error) {
	db := r.db.WithContext(ctx)
	_, manage, err := contentAccess(db, userID, cid, rid)
	if err != nil {
		return nil, false, err
	}
	var t domain.Transcript
	err = db.Table("transcripts t").Select("t.*").Joins("JOIN record r ON r.uuid=t.recording_id AND r.deleted_at IS NULL AND r.status='ready'").
		Where("t.recording_id=? AND t.conference_id=?", rid, cid).
		Where("EXISTS(SELECT 1 FROM conference_participants p WHERE p.conference_id=t.conference_id AND p.user_id=? AND p.admission_state='admitted' AND p.status IN ('joined','left'))", userID).Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, manage, nil
	}
	return &t, manage, err
}

// Segments возвращает страницу с ограниченным количеством сохраненных сегментов стенограммы для
// навигации по временным меткам.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - rid: Идентификатор записи, используемый текущей операцией.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//   - offset: Количество результатов, пропущенных до перехода на запрошенную страницу.
//
// @returns:
//   - result1: Результирующая страница с сегментами.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ContentRepository) Segments(ctx context.Context, userID, cid, rid string, limit, offset int) (domain.SegmentPage, error) {
	page := domain.SegmentPage{Items: []domain.Segment{}, Limit: limit, Offset: offset}
	t, _, err := r.Transcript(ctx, userID, cid, rid)
	if err != nil {
		return page, err
	}
	if t == nil {
		return page, apperrors.ErrNotFound
	}
	if t.Status != domain.Ready {
		return page, apperrors.ErrConflict
	}
	// Повторная проверка политики доступа через EXISTS, чтобы удаление участника или записи
	// после поиска метаданных не привело к раскрытию страницы.
	db := r.db.WithContext(ctx).Table("transcript_segments s").Joins("JOIN transcripts t ON t.id=s.transcript_id JOIN record r ON r.uuid=t.recording_id").
		Where("s.transcript_id=? AND t.status='ready' AND r.deleted_at IS NULL", t.ID).
		Where("EXISTS(SELECT 1 FROM conference_participants p WHERE p.conference_id=t.conference_id AND p.user_id=? AND p.admission_state='admitted' AND p.status IN ('joined','left'))", userID)
	if err = db.Count(&page.Total).Error; err != nil {
		return page, err
	}
	err = db.Select("s.*").Order("s.ordinal").Limit(limit).Offset(offset).Find(&page.Items).Error
	return page, err
}

// Summary возвращает текущее сохраненное краткое резюме без обращения к внешнему провайдеру.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - rid: Идентификатор записи, используемый текущей операцией.
//
// @returns:
//   - result1: Созданное или загруженное краткое резюме.
//   - result2: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ContentRepository) Summary(ctx context.Context, userID, cid, rid string) (*domain.Summary, bool, error) {
	t, manage, err := r.Transcript(ctx, userID, cid, rid)
	if err != nil || t == nil {
		return nil, manage, err
	}
	var s domain.Summary
	err = r.db.WithContext(ctx).Table("meeting_summaries m").Select("m.*").Joins("JOIN transcripts t ON t.id=m.transcript_id AND t.generation=m.transcript_generation JOIN record r ON r.uuid=t.recording_id AND r.deleted_at IS NULL AND r.status='ready'").
		Where("m.transcript_id=? AND m.transcript_generation=?", t.ID, t.Generation).
		Where("EXISTS(SELECT 1 FROM conference_participants p WHERE p.conference_id=t.conference_id AND p.user_id=? AND p.admission_state='admitted' AND p.status IN ('joined','left'))", userID).Take(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, manage, nil
	}
	if err == nil {
		err = json.Unmarshal(s.StructuredOutput, &s.SummaryOutput)
	}
	return &s, manage, err
}

// insertContentJob добавляет задание по созданию контента без дублирования существующего
// запроса.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - entity: Идентификатор сущности или категория сущности, связанные с заданием на обработку.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - version: Версия или значение ограждения, используемое для предотвращения устаревания прав
//     собственности.
//   - payload: Структурированная полезная информация о событии или команде.
//   - attempts: Максимально допустимое количество попыток выполнения задания или текущее
//     количество повторных попыток.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func insertContentJob(tx *gorm.DB, kind, entity, cid string, version int64, payload any, attempts int) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key,max_attempts) VALUES (?,?,?,?,?::jsonb,?,?) ON CONFLICT(dedup_key) DO NOTHING`, kind, entity, cid, version, string(data), fmt.Sprintf("%s:%s:%d", kind, entity, version), attempts).Error
}

// QueueTranscript ставит в очередь авторизованную генерацию транскрипции с ограничениями на
// дедупликацию и повторные попытки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - rid: Идентификатор записи, используемый текущей операцией.
//   - maximum: Максимально допустимый размер или количество символов.
//   - cooldown: Минимальный интервал между дорогостоящими запросами на повторную обработку.
//   - attempts: Максимально допустимое количество попыток выполнения задания или текущее
//     количество повторных попыток.
//
// @returns:
//   - result1: Созданная или загруженная стенограмма.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ContentRepository) QueueTranscript(ctx context.Context, userID, cid, rid string, maximum int, cooldown time.Duration, attempts int) (*domain.Transcript, error) {
	var t domain.Transcript
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := findConference(tx, cid, true); err != nil {
			return err
		}
		_, manage, err := contentAccess(tx, userID, cid, rid)
		if err != nil {
			return err
		}
		if !manage {
			return apperrors.ErrForbidden
		}
		err = tx.Table("transcripts").Clauses(clause.Locking{Strength: "UPDATE"}).Where("recording_id=?", rid).Take(&t).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			t = domain.Transcript{ID: uuid.NewString(), ConferenceID: cid, RecordingID: rid, Status: domain.Queued, Language: "auto", Generation: 2, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			if maximum < 1 {
				return apperrors.ErrConflict
			}
			if err = tx.Table("transcripts").Create(&t).Error; err != nil {
				return err
			}
		} else {
			if err != nil {
				return err
			}
			if t.Status != domain.Failed {
				return apperrors.New(apperrors.ErrConflict, "only failed transcription may be retried")
			}
			if t.Generation >= int64(maximum+1) || time.Since(t.UpdatedAt) < cooldown {
				return apperrors.New(apperrors.ErrConflict, "transcription reprocess limit or cooldown reached")
			}
			t.Generation++
			t.Status = domain.Queued
			t.UpdatedAt = time.Now().UTC()
			t.ErrorCode = nil
			t.ErrorMessage = nil
			if err = tx.Table("transcripts").Where("id=?", t.ID).Updates(map[string]any{"status": t.Status, "generation": t.Generation, "updated_at": t.UpdatedAt, "error_code": nil, "error_message": nil}).Error; err != nil {
				return err
			}
		}
		return insertContentJob(tx, "content.transcribe", rid, cid, t.Generation, map[string]any{}, attempts)
	})
	return &t, err
}

// QueueSummary ставит в очередь авторизованную генерацию сводной информации с ограничениями на
// дедупликацию и повторные попытки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - rid: Идентификатор записи, используемый текущей операцией.
//   - maximum: Максимально допустимый размер или количество символов.
//   - cooldown: Минимальный интервал между дорогостоящими запросами на повторную обработку.
//   - attempts: Максимально допустимое количество попыток выполнения задания или текущее
//     количество повторных попыток.
//
// @returns:
//   - result1: Созданное или загруженное краткое резюме.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ContentRepository) QueueSummary(ctx context.Context, userID, cid, rid string, maximum int, cooldown time.Duration, attempts int) (*domain.Summary, error) {
	var s domain.Summary
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := findConference(tx, cid, true); err != nil {
			return err
		}
		_, manage, err := contentAccess(tx, userID, cid, rid)
		if err != nil {
			return err
		}
		if !manage {
			return apperrors.ErrForbidden
		}
		var t domain.Transcript
		if err = tx.Table("transcripts").Clauses(clause.Locking{Strength: "UPDATE"}).Where("recording_id=? AND status='ready'", rid).Take(&t).Error; err != nil {
			return mapNotFound(err)
		}
		err = tx.Table("meeting_summaries").Clauses(clause.Locking{Strength: "UPDATE"}).Where("transcript_id=?", t.ID).Take(&s).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s = domain.Summary{ID: uuid.NewString(), ConferenceID: cid, TranscriptID: t.ID, TranscriptGeneration: t.Generation, Status: domain.Queued, Generation: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), StructuredOutput: json.RawMessage(`{"summary":"","keyPoints":[],"actionItems":[],"topics":[]}`)}
			if err = tx.Table("meeting_summaries").Omit("summary_output").Create(&s).Error; err != nil {
				return err
			}
		} else {
			if err != nil {
				return err
			}
			if s.Status == domain.Queued || s.Status == domain.Processing {
				return apperrors.ErrConflict
			}
			if s.Generation >= int64(maximum+1) || time.Since(s.UpdatedAt) < cooldown {
				return apperrors.New(apperrors.ErrConflict, "summary reprocess limit or cooldown reached")
			}
			s.Generation++
			s.TranscriptGeneration = t.Generation
			s.Status = domain.Queued
			s.ErrorCode = nil
			s.ErrorMessage = nil
			s.UpdatedAt = time.Now().UTC()
			if err = tx.Table("meeting_summaries").Where("id=?", s.ID).Updates(map[string]any{"generation": s.Generation, "transcript_generation": t.Generation, "status": s.Status, "updated_at": s.UpdatedAt, "error_code": nil, "error_message": nil}).Error; err != nil {
				return err
			}
		}
		return insertContentJob(tx, "content.summarize", t.ID, cid, s.Generation, map[string]any{}, attempts)
	})
	return &s, err
}

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConferenceRecordingRepository сохраняет авторизованный жизненный цикл записи конференции и
// состояние исходящих команд через GORM.
type ConferenceRecordingRepository struct{ db *gorm.DB }

// NewConferenceRecordingRepository создает репозиторий записей конференции с зависимостями и
// ограничениями, необходимыми для записи конференции.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий записей конференции.
func NewConferenceRecordingRepository(db *gorm.DB) *ConferenceRecordingRepository {
	return &ConferenceRecordingRepository{db: db}
}

type RecordingOutbox = records.OutboxCommand

// Start запускает запись конференции и подготавливает ресурсы, необходимые для этой операции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - segmentDuration: Запрошенная продолжительность сегмента записи.
//
// @returns:
//   - result1: Результирующая запись.
//   - result2: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRecordingRepository) Start(ctx context.Context, userID, conferenceID string, segmentDuration int) (records.Record, bool, error) {
	return r.StartMode(ctx, userID, conferenceID, segmentDuration, records.ModeComposite)
}

// StartMode запускает запись в выбранном режиме валидированной записи конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - segmentDuration: Запрошенная продолжительность сегмента записи.
//   - mode: Проверенная операция или режим записи.
//
// @returns:
//   - result1: Результирующая запись.
//   - result2: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRecordingRepository) StartMode(ctx context.Context, userID, conferenceID string, segmentDuration int, mode string) (records.Record, bool, error) {
	if !records.ValidConferenceMode(mode) {
		return records.Record{}, false, apperrors.ErrInvalidInput
	}
	var record records.Record
	created := false
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			conference, err := findConference(tx, conferenceID, true)
			if err != nil {
				return err
			}
			actor, err := findMembership(tx, conferenceID, userID)
			if err != nil {
				return membershipError(err)
			}
			if !actor.CanParticipate() {
				return apperrors.ErrForbidden
			}
			if err = requireRecordingAccount(tx, userID); err != nil {
				return err
			}
			if conference.Status != conferences.Active {
				return apperrors.New(apperrors.ErrConflict, "conference is not active")
			}
			err = tx.Where("platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status IN ?", conferenceID, activeRecordingStatuses()).Take(&record).Error
			if err == nil {
				return apperrors.New(apperrors.ErrConflict, "recording is already active")
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			record = records.Record{UUID: uuid.NewString(), ConferenceID: conferenceID, PlatformConferenceID: &conferenceID, RequestedBy: &userID, Mode: mode, SourceType: "conference", TransportType: "sfu", Status: records.StatusStarting, QualityMode: "auto", SegmentDurationSec: segmentDuration, NeedPreview: mode != records.ModeAudioOnly && mode != records.ModeIndividualTracks}
			if err = tx.Create(&record).Error; err != nil {
				return err
			}
			if err = enqueueRecording(tx, record.UUID, "record.start", ""); err != nil {
				return err
			}
			created = true
			return nil
		})
	return record, created, err
}

// Stop останавливает активную операцию записи конференции и освобождает принадлежащие ей
// ресурсы.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: Результирующая запись.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRecordingRepository) Stop(ctx context.Context, userID, conferenceID, recordID string) (records.Record, error) {
	var record records.Record
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			conference, err := findConference(tx, conferenceID, true)
			if err != nil {
				return err
			}
			actor, err := findMembership(tx, conferenceID, userID)
			if err != nil {
				return membershipError(err)
			}
			if !actor.CanReadHistory() {
				return apperrors.ErrForbidden
			}
			if err = requireRecordingAccount(tx, userID); err != nil {
				return err
			}
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus')", recordID, conferenceID).Take(&record).Error; err != nil {
				return mapNotFound(err)
			}
			owner := conference.OwnerID == userID && actor.Role == conferences.Owner
			if !owner && (record.RequestedBy == nil || *record.RequestedBy != userID) {
				return apperrors.ErrForbidden
			}
			reason := "participant_requested"
			if owner {
				reason = "owner_requested"
			}
			if records.IsTerminalStatus(record.Status) || record.Status == records.StatusFinalizing || record.Status == records.StatusUploading {
				return nil
			}
			if record.Status != records.StatusStopping {
				if err = tx.Model(&record).Updates(map[string]any{"status": records.StatusStopping, "stopped_at": time.Now().UTC(), "ended_reason": reason}).Error; err != nil {
					return err
				}
			}
			return enqueueRecording(tx, record.UUID, "record.stop", reason)
		})
	return record, err
}

// requireRecordingAccount отклоняет идентификационные данные гостей перед операциями записи
// только с использованием учетной записи.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func requireRecordingAccount(tx *gorm.DB, userID string) error {
	var user users.User
	err := tx.Select("id", "guest_conference_id").Where("id = ?", userID).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.ErrForbidden
	}
	if err != nil {
		return err
	}
	if user.GuestConferenceID != nil {
		return apperrors.ErrForbidden
	}
	return nil
}

// Accessible загружает запись только в рамках полномочий действующего аккаунта в рамках
// конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: Результирующая запись.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRecordingRepository) Accessible(ctx context.Context, userID, conferenceID, recordID string) (records.Record, error) {
	if err := r.authorizeRead(ctx, userID, conferenceID); err != nil {
		return records.Record{}, err
	}
	var record records.Record
	err := r.db.WithContext(ctx).Scopes(availableRecordings).Where("uuid = ? AND platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus')", recordID, conferenceID).Take(&record).Error
	return record, mapNotFound(err)
}

// List возвращает ограниченную страницу записей конференции с использованием заданных
// параметров разбивки на страницы и фильтров.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//   - offset: Количество результатов, пропущенных до перехода на запрошенную страницу.
//
// @returns:
//   - result1: Собранные значения записей.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRecordingRepository) List(ctx context.Context, userID, conferenceID string, limit, offset int) ([]records.Record, error) {
	if err := r.authorizeRead(ctx, userID, conferenceID); err != nil {
		return nil, err
	}
	items := []records.Record{}
	err := r.db.WithContext(ctx).Scopes(availableRecordings).Where("platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus')", conferenceID).Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, err
}

// authorizeRead проверяет, может ли действующий аккаунт прочитать запрошенную запись.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRecordingRepository) authorizeRead(ctx context.Context, userID, conferenceID string) error {
	if _, err := findConference(r.db.WithContext(ctx), conferenceID, false); err != nil {
		return err
	}
	participant, err := findMembership(r.db.WithContext(ctx), conferenceID, userID)
	if err != nil {
		return membershipError(err)
	}
	if !participant.CanReadHistory() {
		return apperrors.ErrForbidden
	}
	return nil
}

// activeRecordingStatuses возвращает сохраненные состояния, которые по-прежнему относятся к
// активной записи.
//
// @returns:
//   - result1: Собранные строковые значения.
func activeRecordingStatuses() []string {
	return []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}
}

// enqueueRecording добавляет команду жизненного цикла записи в исходящий пакет транзакций.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - recordID: Внешняя запись UUID.
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//   - reason: Ограниченный жизненный цикл или причина сбоя.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func enqueueRecording(tx *gorm.DB, recordID, command, reason string) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RecordingOutbox{RecordID: recordID, CommandType: command, Reason: reason}).Error
}

// stopConferenceRecordings помечает активные записи конференции как подлежащие остановке и
// помещает их команды в очередь.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func stopConferenceRecordings(tx *gorm.DB, conferenceID string) error {
	var rows []records.Record
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status IN ?", conferenceID, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping}).Find(&rows).Error; err != nil {
		return err
	}
	for _, record := range rows {
		if record.Status != records.StatusStopping {
			if err := tx.Model(&record).Updates(map[string]any{"status": records.StatusStopping, "stopped_at": time.Now().UTC(), "ended_reason": "conference_finished"}).Error; err != nil {
				return err
			}
		}
		if err := enqueueRecording(tx, record.UUID, "record.stop", "conference_finished"); err != nil {
			return err
		}
	}
	return nil
}

// ClaimCommand запрашивает одну команду исходящего пакета транзакций для записи вместе с
// текущим состоянием записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Результирующий исходящий пакет транзакций для записи.
//   - result2: Результирующая запись.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRecordingRepository) ClaimCommand(ctx context.Context) (RecordingOutbox, records.Record, error) {
	var out RecordingOutbox
	var record records.Record
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			err := tx.Raw(`SELECT o.* FROM recording_outbox o WHERE o.published_at IS NULL
		AND (o.claimed_until IS NULL OR o.claimed_until < now())
		AND NOT EXISTS (SELECT 1 FROM recording_outbox prev WHERE prev.record_id=o.record_id AND prev.id<o.id AND prev.published_at IS NULL)
		ORDER BY o.id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&out).Error
			if err != nil {
				return err
			}
			if out.ID == 0 {
				return apperrors.ErrNotFound
			}
			token := uuid.NewString()
			until := time.Now().UTC().Add(15 * time.Second)
			out.ClaimToken = &token
			out.ClaimedUntil = &until
			if err = tx.Model(&out).Updates(map[string]any{"claim_token": token, "claimed_until": until}).Error; err != nil {
				return err
			}
			return tx.Where("uuid = ?", out.RecordID).Take(&record).Error
		})
	return out, record, err
}

// CompleteCommand подтверждает команду исходящего пакета транзакций для записи только в том
// случае, если запрос на запись актуален.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRecordingRepository) CompleteCommand(ctx context.Context, command RecordingOutbox) error {
	return r.db.WithContext(ctx).Model(&RecordingOutbox{}).Where("id = ? AND claim_token = ?", command.ID, command.ClaimToken).Updates(map[string]any{"published_at": time.Now().UTC(), "claimed_until": nil}).Error
}

// RetryCommand повторно отправляет запрос на запись с использованием политики ограниченных
// повторных попыток.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceRecordingRepository) RetryCommand(ctx context.Context, command RecordingOutbox) error {
	return r.db.WithContext(ctx).Model(&RecordingOutbox{}).Where("id = ? AND claim_token = ?", command.ID, command.ClaimToken).Updates(map[string]any{"claimed_until": time.Now().UTC().Add(time.Second)}).Error
}

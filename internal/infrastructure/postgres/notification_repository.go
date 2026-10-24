// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/notifications"
	"gorm.io/gorm"
)

// NotificationRepository сохраняет уведомления на уровне аккаунта через GORM.
type NotificationRepository struct {
	db                      *gorm.DB
	legacyRemindersDisabled bool
}

// DisableLegacyReminders отключает путь к замещающим напоминаниям, чтобы предотвратить
// дублирование уведомлений.
//
// @returns:
//   - result1: Создан или загружен репозиторий уведомлений.
func (r *NotificationRepository) DisableLegacyReminders() *NotificationRepository {
	r.legacyRemindersDisabled = true
	return r
}

// NewNotificationRepository создает репозиторий уведомлений с зависимостями и ограничениями,
// необходимыми для уведомлений.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Создан или загружен репозиторий уведомлений.
func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// List возвращает ограниченную страницу записей уведомлений с использованием заданных
// параметров разбивки на страницы и фильтров.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *NotificationRepository) List(ctx context.Context, userID, cursor string, limit int) (domain.Page, error) {
	page := domain.Page{Items: []domain.Notification{}}
	if limit < 1 || limit > 100 {
		return page, apperrors.ErrInvalidInput
	}
	c, err := domain.DecodeCursor(cursor, userID)
	if err != nil {
		return page, err
	}
	q := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Where(notificationVisibilitySQL("notifications"))
	if c != nil {
		q = q.Where("(created_at, id) < (?, ?::uuid)", c.At, c.ID)
	}
	if err := q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&page.Items).Error; err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		next := domain.EncodeCursor(page.Items[limit-1])
		page.NextCursor = &next
	}
	err = r.db.WithContext(ctx).Model(&domain.Notification{}).Where("user_id = ? AND read_at IS NULL", userID).
		Where(notificationVisibilitySQL("notifications")).Count(&page.UnreadCount).Error
	return page, err
}

// Read — при идемпотентном чтении уведомление, принадлежащее учетной записи, помечается как
// прочитанное, и возвращается его сохраненное состояние.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Результирующее уведомление.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *NotificationRepository) Read(ctx context.Context, userID, id string) (domain.Notification, error) {
	var item domain.Notification
	result := r.db.WithContext(ctx).Raw("UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id = ? AND user_id = ? AND ("+notificationVisibilitySQL("notifications")+") RETURNING *", id, userID).Scan(&item)
	if result.Error != nil {
		return item, result.Error
	}
	if result.RowsAffected == 0 {
		return item, apperrors.ErrNotFound
	}
	return item, nil
}

// Generate создает ограниченный пакет постоянных уведомлений из сохраненных расписаний и
// транзакционных событий.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *NotificationRepository) Generate(ctx context.Context) error {
	// Независимые от часового пояса микросекунды, прошедшие с начала эпохи, являются частью
	// ключа дедупликации для предстоящих конференций.
	soon := `WITH candidates AS (
            SELECT p.user_id, 'soon:' || c.id::text || ':' || (extract(epoch from c.scheduled_at)*1000000)::bigint::text AS dedup_key,
              jsonb_build_object('conferenceId',c.id,'scheduledAt',c.scheduled_at) AS payload
            FROM conferences c JOIN conference_participants p ON p.conference_id=c.id
            WHERE c.kind='meeting' AND c.status='scheduled' AND c.scheduled_at > now() AND c.scheduled_at <= now()+interval '15 minutes'
			  AND p.user_id IS NOT NULL AND p.admission_state IN ('admitted','waiting') AND p.status NOT IN ('kicked','rejected')
			  AND NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=c.id AND cp.user_id=p.user_id AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))
              AND NOT EXISTS(SELECT 1 FROM notifications n WHERE n.user_id=p.user_id AND n.dedup_key='soon:' || c.id::text || ':' || (extract(epoch from c.scheduled_at)*1000000)::bigint::text)
            ORDER BY c.scheduled_at,c.id,p.id LIMIT 100)
          INSERT INTO notifications(id,user_id,type,payload,dedup_key) SELECT gen_random_uuid(),user_id,'conference.soon',payload,dedup_key FROM candidates ON CONFLICT(user_id,dedup_key) DO NOTHING`
	if !r.legacyRemindersDisabled {
		if err := r.db.WithContext(ctx).Exec(soon).Error; err != nil {
			return err
		}
	}
	// job содержит данные о задании, используемые для уведомлений.
	type job struct {
		ID                  int64
		Kind                string
		ConferenceID        string
		RecordingID         *string
		CursorParticipantID *string
		CreatedAt           time.Time
	}
	err := r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			jobs := []job{}
			if err := tx.Raw(`SELECT id,kind,conference_id,recording_id,cursor_participant_id,created_at FROM notification_jobs WHERE processed_at IS NULL ORDER BY available_at,id LIMIT 100 FOR UPDATE SKIP LOCKED`).Scan(&jobs).Error; err != nil {
				return err
			}
			budget := 1000 // Ограниченное количество получателей, а также количество исходных событий.
			for _, item := range jobs {
				if budget == 0 {
					break
				}
				done := true
				updates := map[string]any{}
				if item.Kind == "admission.decided" {
					if err := tx.Exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key,created_at)
                    SELECT gen_random_uuid(),user_id,kind,jsonb_build_object('conferenceId',conference_id,'admissionState',admission_state),
                      'admission:' || entity_id::text || ':' || entity_version::text,created_at
                    FROM notification_jobs j WHERE id=? AND NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=j.conference_id AND cp.user_id=j.user_id AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))
					ON CONFLICT(user_id,dedup_key) DO NOTHING`, item.ID).Error; err != nil {
						return err
					}
					budget--
				} else {
					limit := min(100, budget)
					ids := []string{}
					q := tx.Table("conference_participants p").Joins("JOIN record r ON r.uuid = ? AND r.platform_conference_id = p.conference_id AND r.mode IN ('composite','audio_only','individual_tracks','screen_focus') AND r.status IN ('ready','partial_ready') AND r.deleted_at IS NULL", item.RecordingID).
						Where("p.conference_id = ? AND p.user_id IS NOT NULL AND p.admission_state='admitted' AND p.status IN ('joined','left')", item.ConferenceID).
						Where("COALESCE((SELECT pref.recording FROM notification_preferences pref WHERE pref.user_id=p.user_id),TRUE)").
						Where("NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))").
						Where("p.created_at <= ? AND (p.admission_decided_at IS NULL OR p.admission_decided_at <= ?)", item.CreatedAt, item.CreatedAt)
					if item.CursorParticipantID != nil {
						q = q.Where("p.id > ?", *item.CursorParticipantID)
					}
					if err := q.Order("p.id").Limit(limit).Pluck("p.id", &ids).Error; err != nil {
						return err
					}
					if len(ids) > 0 {
						// Повторная проверка текущего доступа в запросе INSERT, в том числе на
						// случай удаления конференции или участника, опережающего выбор ранее
						// ограниченного числа кандидатов.
						if err := tx.Exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key,created_at)
                        SELECT gen_random_uuid(),p.user_id,j.kind,jsonb_build_object('conferenceId',j.conference_id,'recordingId',j.recording_id),
                          'recording:' || j.recording_id::text,j.created_at
                        FROM notification_jobs j JOIN record r ON r.uuid=j.recording_id
                        JOIN conference_participants p ON p.conference_id=j.conference_id
                        WHERE j.id=? AND p.id IN ? AND p.user_id IS NOT NULL AND p.admission_state='admitted' AND p.status IN ('joined','left')
                          AND p.created_at <= j.created_at AND (p.admission_decided_at IS NULL OR p.admission_decided_at <= j.created_at)
                          AND r.mode IN ('composite','audio_only','individual_tracks','screen_focus') AND r.status IN ('ready','partial_ready') AND r.deleted_at IS NULL
                          AND COALESCE((SELECT pref.recording FROM notification_preferences pref WHERE pref.user_id=p.user_id),TRUE)
						  AND NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))
                        ON CONFLICT(user_id,dedup_key) DO NOTHING`, item.ID, ids).Error; err != nil {
							return err
						}
						updates["cursor_participant_id"] = ids[len(ids)-1]
					}
					budget -= len(ids)
					done = len(ids) < limit
				}
				if done {
					updates["processed_at"] = time.Now().UTC()
				} else {
					updates["available_at"] = time.Now().UTC()
				}
				if err := tx.Table("notification_jobs").Where("id=?", item.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		return err
	}
	return r.generateChat(ctx)
}

// generateChat последовательно извлекает по одному сохраненному сообщению из ограниченных
// возобновляемых пакетов.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *NotificationRepository) generateChat(ctx context.Context) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		type chatJob struct {
			MessageID           string
			ConferenceID        string
			CursorParticipantID *string
		}
		jobs := []chatJob{}
		if err := tx.Raw(`SELECT message_id,conference_id,cursor_participant_id FROM chat_notification_jobs
			WHERE completed_at IS NULL ORDER BY created_at,message_id LIMIT 10 FOR UPDATE SKIP LOCKED`).Scan(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			ids := []string{}
			q := tx.Table("conference_participants p").Joins("JOIN chat_messages m ON m.conference_id=p.conference_id AND m.id=? AND m.deleted_at IS NULL", job.MessageID).
				Joins("JOIN users u ON u.id=p.user_id AND u.guest_conference_id IS NULL AND u.password_hash<>''").
				Where("p.conference_id=? AND p.user_id IS NOT NULL AND p.user_id<>m.sender_user_id AND p.admission_state='admitted' AND p.status IN ('joined','left')", job.ConferenceID).
				Where("p.created_at<=m.created_at AND (p.admission_decided_at IS NULL OR p.admission_decided_at<=m.created_at)").
				Where("NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))")
			if job.CursorParticipantID != nil {
				q = q.Where("p.id>?", *job.CursorParticipantID)
			}
			if err := q.Order("p.id").Limit(100).Pluck("p.id", &ids).Error; err != nil {
				return err
			}
			if len(ids) > 0 {
				if err := tx.Exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key,created_at)
					SELECT gen_random_uuid(),p.user_id,'chat.message',jsonb_build_object('conferenceId',m.conference_id,'messageId',m.id,
					'isReply',COALESCE((SELECT reply.sender_user_id=p.user_id FROM chat_messages reply WHERE reply.id=m.reply_to_id AND reply.conference_id=m.conference_id AND reply.deleted_at IS NULL),FALSE)),
					'chat:'||m.id::text,m.created_at
					FROM chat_messages m JOIN conference_participants p ON p.conference_id=m.conference_id
					JOIN users u ON u.id=p.user_id AND u.guest_conference_id IS NULL AND u.password_hash<>''
					WHERE m.id=? AND m.deleted_at IS NULL AND p.id IN ? AND p.user_id IS NOT NULL AND p.user_id<>m.sender_user_id
					AND p.admission_state='admitted' AND p.status IN ('joined','left')
					AND p.created_at<=m.created_at AND (p.admission_decided_at IS NULL OR p.admission_decided_at<=m.created_at)
					AND NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))
					ON CONFLICT(user_id,dedup_key) DO NOTHING`, job.MessageID, ids).Error; err != nil {
					return err
				}
			}
			if len(ids) < 100 {
				if err := tx.Table("chat_notification_jobs").Where("message_id=?", job.MessageID).Update("completed_at", time.Now().UTC()).Error; err != nil {
					return err
				}
			} else if err := tx.Table("chat_notification_jobs").Where("message_id=?", job.MessageID).Update("cursor_participant_id", ids[len(ids)-1]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Pending возвращает сохраненные уведомления, публикация событий по которым еще не завершена.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Собранные значения уведомлений.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *NotificationRepository) Pending(ctx context.Context) ([]domain.Notification, error) {
	items := []domain.Notification{}
	if err := r.db.WithContext(ctx).Exec(`UPDATE notifications n SET published_at=clock_timestamp()
		WHERE published_at IS NULL AND NOT (` + notificationVisibilitySQL("n") + `)`).Error; err != nil {
		return nil, err
	}
	err := r.db.WithContext(ctx).Where("published_at IS NULL").Where(notificationVisibilitySQL("notifications")).Order("created_at,id").Limit(100).Find(&items).Error
	return items, err
}

// Publishable проверяет текущие настройки комнаты непосредственно перед событием SSE.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *NotificationRepository) Publishable(ctx context.Context, id string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("notifications n").Where("n.id=? AND n.published_at IS NULL", id).
		Where(notificationVisibilitySQL("n")).Count(&count).Error
	return count > 0, err
}

// Visible — визуально перепроверяет право собственности, отключение звука и текущий уровень доступа на
// момент доставки SSE; PublishedAt может быть уже установлен производителем на момент получения
// данных из Redis.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *NotificationRepository) Visible(ctx context.Context, userID, id string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("notifications n").Where("n.id=? AND n.user_id=?", id, userID).
		Where(notificationVisibilitySQL("n")).Count(&count).Error
	return count == 1, err
}

// Published записывает информацию о том, что сохранённое уведомление было опубликовано или
// удалено.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *NotificationRepository) Published(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).Model(&domain.Notification{}).Where("id=? AND published_at IS NULL", id).Update("published_at", time.Now().UTC()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

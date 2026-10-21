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
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	d "github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/domain/users"
	u "github.com/janickiy/meet-space/internal/usecase/integrations"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConferenceInvitationRepository struct{ db *gorm.DB }

// NewConferenceInvitationRepository создаёт репозиторий приглашений на конференцию с
// зависимостями и ограничениями, необходимыми для приглашения на конференцию.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий приглашений на конференцию.
func NewConferenceInvitationRepository(db *gorm.DB) *ConferenceInvitationRepository {
	return &ConferenceInvitationRepository{db: db}
}

// invitationActor проверяет права доступа как к аккаунту, так и к комнате, прежде чем вернуть
// данные из каталога.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - lock: Режим блокировки или требование блокировки для операции с базой данных.
//
// @returns:
//   - result1: Результирующая конференция.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func invitationActor(tx *gorm.DB, actor, conference string, lock bool) (d.Conference, error) {
	var c d.Conference
	q := tx.Where("id=? AND kind='meeting'", conference)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.Take(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = apperrors.ErrNotFound
		}
		return c, err
	}
	var permanent int64
	if err := tx.Model(&users.User{}).Where("id=? AND guest_conference_id IS NULL AND password_hash<>''", actor).Count(&permanent).Error; err != nil {
		return c, err
	}
	if permanent == 0 {
		return c, apperrors.ErrForbidden
	}
	if c.OwnerID != actor {
		var allowed int64
		if err := tx.Model(&d.Participant{}).Where("conference_id=? AND user_id=? AND role='co_host' AND status='joined' AND admission_state='admitted'", conference, actor).Count(&allowed).Error; err != nil {
			return c, err
		}
		if c.Status != d.Active || allowed == 0 {
			return c, apperrors.ErrForbidden
		}
	}
	if c.Status != d.Created && c.Status != d.Scheduled && c.Status != d.Active {
		return c, apperrors.ErrConflict
	}
	return c, nil
}

// SearchInvitationUsers возвращает ограниченный список зарегистрированных кандидатов на
// получение приглашения, доступных действующему организатору.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - query: Текст для поиска или валидированные фильтры запросов.
//
// @returns:
//   - result1: Собранные значения пользователей, которым было отправлено приглашение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceInvitationRepository) SearchInvitationUsers(ctx context.Context, actor, conference, query string) ([]d.InvitationUser, error) {
	items := []d.InvitationUser{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := invitationActor(tx, actor, conference, false); err != nil {
			return err
		}
		// Экранирование операторов LIKE: входные данные представляют собой литеральную часть, а
		// не подстановочный знак каталога.
		query = strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query)
		return tx.Model(&users.User{}).Select("id,email,display_name").Where("guest_conference_id IS NULL AND password_hash<>'' AND (email ILIKE ? OR display_name ILIKE ?)", "%"+query+"%", "%"+query+"%").Order("email,id").Limit(20).Scan(&items).Error
	})
	return items, err
}

// Invite создаёт или обновляет приглашения на конференцию для подтверждённых получателей.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Собранные значения результатов приглашения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *ConferenceInvitationRepository) Invite(ctx context.Context, actor, conference string, request d.InvitationRequest) ([]d.InvitationResult, error) {
	items := []d.InvitationResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := invitationActor(tx, actor, conference, true); err != nil {
			return err
		}
		// Сериализация данных в комнате предотвращает повторную отправку приглашений из-за
		// одновременных действий нескольких организаторов.
		if err := tx.Exec("SET LOCAL meetrix.explicit_invitation='true'").Error; err != nil {
			return err
		}
		recipients := map[string]*users.User{}
		for _, email := range request.Emails {
			recipients[email] = nil
		}
		var accounts []users.User
		if len(request.UserIDs) > 0 {
			if err := tx.Where("id IN ? AND guest_conference_id IS NULL AND password_hash<>''", request.UserIDs).Find(&accounts).Error; err != nil {
				return err
			}
			if len(accounts) != len(request.UserIDs) {
				return apperrors.New(apperrors.ErrInvalidInput, "selected users must have permanent accounts")
			}
			for i := range accounts {
				account := accounts[i]
				recipients[account.Email] = &account
			}
		}
		if len(request.Emails) > 0 {
			var matches []users.User
			if err := tx.Where("email IN ? AND guest_conference_id IS NULL AND password_hash<>''", request.Emails).Find(&matches).Error; err != nil {
				return err
			}
			for i := range matches {
				account := matches[i]
				recipients[account.Email] = &account
			}
		}
		emails := make([]string, 0, len(recipients))
		for email := range recipients {
			emails = append(emails, email)
		}
		sort.Strings(emails)
		for _, email := range emails {
			// Адреса аккаунтов определяются после проверки API; проверьте их тоже, не изменяя
			// зарегистрированный аккаунт и не отправляя в очередь письма, которые не будут
			// доставлены.
			if err := d.ValidateInvitationEmail(email); err != nil {
				return err
			}
			account := recipients[email]
			var userID *string
			if account != nil {
				userID = &account.ID
				var membership d.Participant
				err := tx.Where("conference_id=? AND user_id=?", conference, account.ID).Take(&membership).Error
				if err == nil && membership.IsRejectedOrKicked() {
					return apperrors.ErrForbidden
				}
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if errors.Is(err, gorm.ErrRecordNotFound) {
					name := account.Email
					if account.DisplayName != nil && *account.DisplayName != "" {
						name = *account.DisplayName
					}
					if utf8.RuneCountInString(name) > 100 {
						name = string([]rune(name)[:100])
					}
					membership = d.Participant{ID: uuid.NewString(), ConferenceID: conference, UserID: userID, DisplayName: name, Role: d.ParticipantRole, Status: d.Left, AdmissionState: d.AdmissionAdmitted}
					if err := tx.Create(&membership).Error; err != nil {
						return err
					}
				}
			}
			if userID != nil {
				var left int64
				if err := tx.Table("conference_chat_preferences").Where("conference_id=? AND user_id=? AND left_at IS NOT NULL", conference, *userID).Count(&left).Error; err != nil {
					return err
				}
				if left > 0 {
					// Только бывший участник может восстановить доступ к чату, явно
					// присоединившись к нему.
					items = append(items, d.InvitationResult{Email: email, UserID: userID, Status: "left_chat"})
					continue
				}
			}
			invitation := d.Invitation{ID: uuid.NewString(), ConferenceID: conference, RequestedBy: actor, Email: email, UserID: userID, Status: "queued"}
			result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "conference_id"}, {Name: "email"}}, DoNothing: true}).Create(&invitation)
			if result.Error != nil {
				return result.Error
			}
			status := "queued"
			if result.RowsAffected == 0 {
				invitation = d.Invitation{}
				if err := tx.Where("conference_id=? AND email=?", conference, email).Take(&invitation).Error; err != nil {
					return err
				}
				status = "already_invited"
				if invitation.Status == "failed" {
					reset := tx.Exec(`UPDATE background_jobs SET state='queued',attempts=0,available_at=now(),finished_at=NULL,error_code=NULL WHERE kind='integrations.invitation' AND entity_id=? AND state='failed'`, invitation.ID)
					if reset.Error != nil {
						return reset.Error
					}
					if reset.RowsAffected == 1 {
						if err := tx.Model(&d.Invitation{}).Where("id=?", invitation.ID).Updates(map[string]any{"status": "queued", "error_code": ""}).Error; err != nil {
							return err
						}
						status = "queued"
					}
				}
			} else {
				if err := tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,payload,dedup_key) VALUES('integrations.invitation',?,?,?,'{}'::jsonb,?)`, invitation.ID, conference, userID, "meeting-invitation:"+invitation.ID).Error; err != nil {
					return err
				}
				if userID != nil {
					if err := tx.Exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key)
						SELECT gen_random_uuid(),?,'conference.invited',jsonb_build_object('conferenceId',?::uuid,'invitationId',?::uuid,'scheduledAt',(SELECT scheduled_at FROM conferences WHERE id=?)),?
						WHERE NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=?::uuid AND cp.user_id=?::uuid AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))
						ON CONFLICT(user_id,dedup_key) DO NOTHING`, *userID, conference, invitation.ID, conference, "meeting-invitation:"+invitation.ID, conference, *userID).Error; err != nil {
						return err
					}
				}
			}
			items = append(items, d.InvitationResult{ID: invitation.ID, Email: invitation.Email, UserID: invitation.UserID, Status: status})
		}
		return nil
	})
	return items, err
}

// InvitationDelivery выполняет проверку авторизации и аренды непосредственно перед SMTP.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: В результате доставки приглашения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) InvitationDelivery(ctx context.Context, job jobs.Job) (u.InvitationDelivery, error) {
	var delivery u.InvitationDelivery
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := integrationLease(tx, job); err != nil {
			return err
		}
		if err := tx.Where("id=? AND conference_id=?", job.EntityID, job.ConferenceID).Take(&delivery.Invitation).Error; err != nil {
			return err
		}
		if err := tx.Table("conferences").Where("id=?", job.ConferenceID).Take(&delivery.Conference).Error; err != nil {
			return err
		}
		if err := tx.Table("users").Select("COALESCE(NULLIF(display_name,''),email)").Where("id=?", delivery.Conference.OwnerID).Scan(&delivery.Organizer).Error; err != nil {
			return err
		}
		if delivery.Invitation.Status != "queued" || (delivery.Conference.Status != "created" && delivery.Conference.Status != "scheduled" && delivery.Conference.Status != "active") {
			return nil
		}
		if delivery.Invitation.UserID != nil {
			var allowed int64
			if err := tx.Table("conference_participants p").Joins("JOIN users u ON u.id=p.user_id AND u.guest_conference_id IS NULL AND u.password_hash<>''").Where("p.conference_id=? AND p.user_id=? AND p.status NOT IN ('kicked','rejected') AND p.admission_state IN ('admitted','waiting')", job.ConferenceID, *delivery.Invitation.UserID).
				Where("NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))").Count(&allowed).Error; err != nil {
				return err
			}
			if allowed == 0 {
				return nil
			}
		}
		delivery.Allowed = true
		return nil
	})
	return delivery, err
}

// CompleteInvitation запись о доставке приглашения сохраняется только для текущего
// идентификатора задания.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - status: Состояние жизненного цикла или статус ответа на запись.
//   - code: Ограниченный публичный статус или код ошибки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) CompleteInvitation(ctx context.Context, job jobs.Job, status, code string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := integrationLease(tx, job); err != nil {
			return err
		}
		return tx.Exec(`UPDATE conference_invitations SET status=?,error_code=?,sent_at=CASE WHEN ?='sent' THEN clock_timestamp() ELSE sent_at END WHERE id=? AND conference_id=? AND status='queued'`, status, code, status, job.EntityID, job.ConferenceID).Error
	})
}

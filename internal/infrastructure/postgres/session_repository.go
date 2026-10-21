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
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"gorm.io/gorm"
)

// SessionRepository сохраняет историю физических сеансов и состояние авторизации в комнате с
// помощью GORM.
type SessionRepository struct{ db *gorm.DB }

// NewSessionRepository создаёт репозиторий сеансов с зависимостями и ограничениями,
// необходимыми для сеанса.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий сеансов.
func NewSessionRepository(db *gorm.DB) *SessionRepository { return &SessionRepository{db: db} }

// authorizeSession проверяет, может ли участник открыть активный физический сеанс для
// конференции.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - locked: Указывает, должен ли запрос блокировать выбранную строку перед изменением.
//
// @returns:
//   - result1: Полученный участник.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func authorizeSession(db *gorm.DB, conferenceID, userID string, locked bool) (conferences.Participant, error) {
	return authorizeRoomSession(db, conferenceID, userID, locked, "", false)
}

// authorizeRoomSession проверяет сохранённое членство в комнате и состояние конференции для
// физического сеанса.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - locked: Указывает, должен ли запрос блокировать выбранную строку перед изменением.
//   - clientID: Идентификатор физического браузера или сокетного клиента.
//   - requireClient: Проверяет, требуется ли привязка физического клиента для авторизации.
//
// @returns:
//   - result1: Полученный участник.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func authorizeRoomSession(db *gorm.DB, conferenceID, userID string, locked bool, clientID string, requireClient bool) (conferences.Participant, error) {
	c, err := findMediaRoom(db, conferenceID, locked)
	if err != nil {
		return conferences.Participant{}, err
	}
	if c.Kind == "direct_call" {
		// Само по себе членство в конференции не даёт доступа к другому устройству и не
		// позволяет возобновить прерванный вызов.  Подключиться могут только два клиента,
		// которые инициировали или приняли вызов.
		q := db.Table("direct_calls").Where("conference_id=? AND status='accepted' AND caller_lease_until>clock_timestamp() AND callee_lease_until>clock_timestamp()", conferenceID)
		if requireClient {
			if clientID == "" {
				return conferences.Participant{}, apperrors.ErrForbidden
			}
			q = q.Where("(caller_user_id=? AND caller_client_id=?) OR (callee_user_id=? AND accepted_by_client_id=?)", userID, clientID, userID, clientID)
		} else {
			q = q.Where("caller_user_id=? OR callee_user_id=?", userID, userID)
		}
		var count int64
		if err = q.Count(&count).Error; err != nil {
			return conferences.Participant{}, err
		}
		if count != 1 {
			return conferences.Participant{}, apperrors.ErrForbidden
		}
	}
	p, err := findMembership(db, conferenceID, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return p, apperrors.ErrForbidden
	}
	if err != nil {
		return p, err
	}
	if c.Status != conferences.Created && c.Status != conferences.Active {
		return p, apperrors.New(apperrors.ErrConflict, "conference is closed")
	}
	if !p.CanParticipate() {
		return p, apperrors.New(apperrors.ErrForbidden, "join the conference before connecting")
	}
	var left int64
	if err := db.Table("conference_chat_preferences").Where("conference_id=? AND user_id=? AND left_at IS NOT NULL", conferenceID, userID).Count(&left).Error; err != nil {
		return p, err
	}
	if left > 0 {
		return p, apperrors.ErrForbidden
	}
	return p, nil
}

// Authorize проверяет текущие идентификационные данные и сохранённое состояние доступа перед
// началом сеанса.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Полученный участник.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *SessionRepository) Authorize(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	return authorizeSession(r.db.WithContext(ctx), conferenceID, userID, false)
}

// AuthorizeClient проверяет доступ к конференции для указанной учётной записи и
// идентификационных данных физического клиента.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - clientID: Идентификатор физического браузера или сокетного клиента.
//
// @returns:
//   - result1: Полученный участник.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *SessionRepository) AuthorizeClient(ctx context.Context, conferenceID, userID, clientID string) (conferences.Participant, error) {
	return authorizeRoomSession(r.db.WithContext(ctx), conferenceID, userID, false, clientID, true)
}

// Open открывает доступ к медиафайлам, записям или сеансам, необходимым для работы этого
// компонента.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *SessionRepository) Open(ctx context.Context, session realtime.Session) error {
	return r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			p, err := authorizeRoomSession(tx, session.ConferenceID, session.UserID, true, session.ClientID, true)
			if err != nil {
				return err
			}
			if p.ID != session.ParticipantID {
				return apperrors.ErrForbidden
			}
			return tx.Create(&session).Error
		})
}

// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - connectionID: Идентификатор физического подключения к медиа.
//   - seen: Контрольная временная метка подтвержденной активности транспорта.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *SessionRepository) Close(ctx context.Context, connectionID string, seen time.Time) error {
	// Идемпотентность: PostgreSQL изменяется только при открытии и закрытии, а не при каждом
	// pong-сеансе.
	return r.db.WithContext(ctx).Transaction(

		func(tx *gorm.DB) error {
			var session realtime.Session
			if err := tx.Where("connection_id = ?", connectionID).Take(&session).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			// Используйте тот же порядок блокировки, что и при обновлении медиаданных,
			// открытии, удалении участников и завершении конференции.
			if _, err := findMediaRoom(tx, session.ConferenceID, true); err != nil {
				return err
			}
			return tx.Model(&realtime.Session{}).Where("connection_id = ? AND status = 'connected'", connectionID).
				Updates(map[string]any{"status": "disconnected", "microphone_enabled": false, "camera_enabled": false, "screen_sharing": false, "last_seen_at": gorm.Expr("GREATEST(connected_at, ?)", seen), "disconnected_at": gorm.Expr("GREATEST(connected_at, ?, NOW())", seen)}).Error
		})
}

// Stale возвращает данные об исторических физических сеансах, которые остаются открытыми после
// достижения порога активности.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - before: Порог, после достижения которого выбираются результаты или состояние.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//
// @returns:
//   - result1: Собранные значения сеанса.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *SessionRepository) Stale(ctx context.Context, before time.Time, cursor string) ([]realtime.Session, error) {
	var rows []realtime.Session
	query := r.db.WithContext(ctx).Where("status = 'connected' AND connected_at < ?", before)
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}
	err := query.Order("id").Limit(500).Find(&rows).Error
	return rows, err
}

// Roster загружает данные о конференции и членстве в ней, необходимые для авторизации текущего
// снимка состояния комнаты.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Результирующий статус.
//   - result2: Собранные данные об участниках.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *SessionRepository) Roster(ctx context.Context, conferenceID string) (conferences.Status, []conferences.Participant, error) {
	c, err := findMediaRoom(r.db.WithContext(ctx), conferenceID, false)
	if err != nil {
		return "", nil, err
	}
	rows := []conferences.Participant{}
	err = r.db.WithContext(ctx).Table("conference_participants p").Select(`p.*, EXISTS (
		SELECT 1 FROM conference_chat_preferences cp
		WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND cp.left_at IS NOT NULL
	) AS chat_left`).Where("p.conference_id = ?", conferenceID).Order("p.created_at, p.id").Scan(&rows).Error
	return c.Status, rows, err
}

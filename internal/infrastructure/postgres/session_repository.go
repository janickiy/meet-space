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

// SessionRepository реализует постоянное хранение физических сессий и событий комнаты через GORM.
//   - db: подключение или текущая транзакция GORM, задающая контекст доступа к базе.
type SessionRepository struct{ db *gorm.DB }

// NewSessionRepository создаёт и связывает зависимости компонента SessionRepository, используемого в присутствии участников и доставке realtime-событий.
//
// @args
//   - db (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//
// @return:
//   - результат 1 (*SessionRepository): созданный компонент с переданными зависимостями.
func NewSessionRepository(db *gorm.DB) *SessionRepository { return &SessionRepository{db: db} }

// authorizeSession проверяет допуск участника к живому соединению данной конференции.
//
// @args
//   - db (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - locked (bool): указывает, требуется ли чтение с блокировкой строки для согласованного изменения.
//
// @return:
//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func authorizeSession(db *gorm.DB, conferenceID, userID string, locked bool) (conferences.Participant, error) {
	return authorizeRoomSession(db, conferenceID, userID, locked, "", false)
}

func authorizeRoomSession(db *gorm.DB, conferenceID, userID string, locked bool, clientID string, requireClient bool) (conferences.Participant, error) {
	c, err := findMediaRoom(db, conferenceID, locked)
	if err != nil {
		return conferences.Participant{}, err
	}
	if c.Kind == "direct_call" {
		// A membership alone cannot grant access to another device or revive an
		// ended call. Only the two clients that started/accepted may connect.
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

// Authorize проверяет право пользователя участвовать в операции до работы с защищёнными ресурсами.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *SessionRepository) Authorize(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	return authorizeSession(r.db.WithContext(ctx), conferenceID, userID, false)
}

func (r *SessionRepository) AuthorizeClient(ctx context.Context, conferenceID, userID, clientID string) (conferences.Participant, error) {
	return authorizeRoomSession(r.db.WithContext(ctx), conferenceID, userID, false, clientID, true)
}

// Open создаёт историческую физическую сессию только для действующего допущенного участника.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - session (realtime.Session): историческая физическая сессия или состояние текущего соединения.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *SessionRepository) Open(ctx context.Context, session realtime.Session) error {
	return r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
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

// Close фиксирует завершение исторической физической сессии.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - connectionID (string): идентификатор физического медиа-соединения.
//   - seen (time.Time): временная отметка seen; указатель допускает отсутствие значения.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *SessionRepository) Close(ctx context.Context, connectionID string, seen time.Time) error {
	// Идемпотентно: PostgreSQL обновляется только при открытии и закрытии, а не на каждый pong.
	return r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			var session realtime.Session
			if err := tx.Where("connection_id = ?", connectionID).Take(&session).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			// Порядок блокировок совпадает с обновлением медиа, Open, удалением участника и завершением.
			if _, err := findMediaRoom(tx, session.ConferenceID, true); err != nil {
				return err
			}
			return tx.Model(&realtime.Session{}).Where("connection_id = ? AND status = 'connected'", connectionID).
				Updates(map[string]any{"status": "disconnected", "microphone_enabled": false, "camera_enabled": false, "screen_sharing": false, "last_seen_at": gorm.Expr("GREATEST(connected_at, ?)", seen), "disconnected_at": gorm.Expr("GREATEST(connected_at, ?, NOW())", seen)}).Error
		})
}

// Stale находит незакрытые исторические сессии старше контрольной границы.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - before (time.Time): временная отметка before; указатель допускает отсутствие значения.
//   - cursor (string): непрозрачная граница продолжения предыдущей страницы.
//
// @return:
//   - результат 1 ([]realtime.Session): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *SessionRepository) Stale(ctx context.Context, before time.Time, cursor string) ([]realtime.Session, error) {
	var rows []realtime.Session
	query := r.db.WithContext(ctx).Where("status = 'connected' AND connected_at < ?", before)
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}
	err := query.Order("id").Limit(500).Find(&rows).Error
	return rows, err
}

// Roster читает конференцию и членства для авторизации и построения снимка комнаты.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 (conferences.Status): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 ([]conferences.Participant): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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

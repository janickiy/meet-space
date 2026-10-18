package calls

import "time"

const RingTimeout = 45 * time.Second
const LeaseTimeout = 45 * time.Second

type Peer struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type Call struct {
	ID                 string     `json:"id"`
	ConversationID     string     `json:"conversationId"`
	ConferenceID       *string    `json:"conferenceId"`
	CallerUserID       string     `json:"callerUserId"`
	CalleeUserID       string     `json:"calleeUserId"`
	CallerClientID     string     `json:"callerClientId"`
	AcceptedByClientID *string    `json:"acceptedByClientId"`
	ClientRequestID    string     `json:"-"`
	Status             string     `json:"status"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	ExpiresAt          time.Time  `json:"expiresAt"`
	AcceptedAt         *time.Time `json:"acceptedAt"`
	EndedAt            *time.Time `json:"endedAt"`
	CallerLeaseUntil   *time.Time `json:"-"`
	CalleeLeaseUntil   *time.Time `json:"-"`
	Caller             Peer       `json:"caller" gorm:"-"`
	Callee             Peer       `json:"callee" gorm:"-"`
	DurationSec        int64      `json:"durationSec" gorm:"-"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Call) TableName() string { return "direct_calls" }

// Active сообщает, находится ли прямой вызов в нетерминальном состоянии.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (c Call) Active() bool { return c.Status == "ringing" || c.Status == "accepted" }

// IsParticipant проверяет, является ли учетная запись одним из двух участников прямого вызова.
//
// @Parameters:
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (c Call) IsParticipant(user string) bool {
	return user == c.CallerUserID || user == c.CalleeUserID
}

// OwnsMedia проверяет, принадлежат ли учетная запись и физический клиент активному
// медиасоединению прямого вызова.
//
// @Parameters:
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - client: Идентификация медиа-клиента браузера или зависимость от транспорта, определяемая
//     его типом.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (c Call) OwnsMedia(user, client string) bool {
	return client != "" && ((user == c.CallerUserID && client == c.CallerClientID) ||
		(user == c.CalleeUserID && c.AcceptedByClientID != nil && client == *c.AcceptedByClientID))
}

// Expired проверяет, не истек ли срок действия прямого вызова.
//
// @Parameters:
//   - now: Время, используемое для расчета срока действия или продолжительности.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (c Call) Expired(now time.Time) bool {
	if c.Status == "ringing" {
		return !now.Before(c.ExpiresAt)
	}
	return c.Status == "accepted" && ((c.CallerLeaseUntil != nil && !now.Before(*c.CallerLeaseUntil)) || (c.CalleeLeaseUntil != nil && !now.Before(*c.CalleeLeaseUntil)))
}

// SetDuration вычисляет неотрицательную продолжительность принятого вызова, исходя из
// сохраненного времени окончания или текущего времени.
//
// @Parameters:
//   - now: Время, используемое для расчета срока действия или продолжительности.
func (c *Call) SetDuration(now time.Time) {
	if c.AcceptedAt == nil {
		return
	}
	until := now
	if c.EndedAt != nil {
		until = *c.EndedAt
	}
	c.DurationSec = max(0, int64(until.Sub(*c.AcceptedAt)/time.Second))
}

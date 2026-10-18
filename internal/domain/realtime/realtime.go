package realtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/conferences"
)

// Session — сессия хранит данные об одном физическом соединении WebSocket.  Членство участника
// сохраняется после закрытия сессии.
type Session struct {
	ID             string     `json:"id" gorm:"type:uuid;primaryKey"`
	ConferenceID   string     `json:"conferenceId" gorm:"type:uuid"`
	ParticipantID  string     `json:"participantId" gorm:"type:uuid"`
	UserID         string     `json:"userId" gorm:"type:uuid"`
	ConnectionID   string     `json:"connectionId" gorm:"type:uuid"`
	ClientID       string     `json:"clientId,omitempty" gorm:"-"`
	Status         string     `json:"status"`
	ConnectedAt    time.Time  `json:"connectedAt"`
	DisconnectedAt *time.Time `json:"disconnectedAt,omitempty"`
	LastSeenAt     time.Time  `json:"lastSeenAt"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Session) TableName() string { return "participant_sessions" }

// Identity — идентификационные данные содержат подтверждённые данные о пользователе, сроке действия токена
// и, при необходимости, данные о продолжительном сеансе аутентификации.
type Identity struct {
	UserID        string    `json:"userId"`
	ExpiresAt     time.Time `json:"expiresAt"`
	AuthSessionID string    `json:"authSessionId,omitempty"`
	ClientID      string    `json:"clientId,omitempty"`
}

// Envelope — это событие WebSocket с указанием версии.  ReplyTo сопоставляет ответ с запросом.
type Envelope struct {
	Version      int             `json:"version"`
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	ConferenceID string          `json:"conferenceId"`
	Timestamp    time.Time       `json:"timestamp"`
	Data         json.RawMessage `json:"data"`
	ReplyTo      string          `json:"replyTo,omitempty"`
}

// Event создает серверную оболочку с новыми временными метками ID и UTC.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - data: Входные байты или структурированная полезная информация о событии.
//
// @returns:
//   - result1: Результирующий конверт.
func Event(kind, conferenceID string, data any) Envelope {
	raw, _ := json.Marshal(data)
	return Envelope{Version: 1, ID: uuid.NewString(), Type: kind, ConferenceID: conferenceID, Timestamp: time.Now().UTC(), Data: raw}
}

// Presence содержит общую информацию о доступности участников и количестве активных физических
// подключений.
type Presence struct {
	conferences.ParticipantView
	Online        bool     `json:"online"`
	Connections   int      `json:"connections"`
	ConnectionIDs []string `json:"connectionIds"`
}

// State содержит начальные данные конференции и список видимых участников для комнаты.
type State struct {
	ConnectionID  string             `json:"connectionId"`
	ParticipantID string             `json:"participantId"`
	Status        conferences.Status `json:"status"`
	Participants  []Presence         `json:"participants"`
}

// Signal содержит ограниченную полезную информацию о сигнале, доверенная идентификация которого
// присвоена сервером.
type Signal struct {
	TargetConnectionID  string          `json:"targetConnectionId"`
	SDP                 string          `json:"sdp,omitempty"`
	Candidate           json.RawMessage `json:"candidate,omitempty"`
	SenderConnectionID  string          `json:"senderConnectionId,omitempty"`
	SenderParticipantID string          `json:"senderParticipantId,omitempty"`
}

// ICEConfig содержит серверы ICE, необходимые для установления соединения WebRTC.
type ICEConfig struct {
	ICEServers         []ICEServer `json:"iceServers"`
	ICETransportPolicy string      `json:"iceTransportPolicy,omitempty"`
	ExpiresAt          int64       `json:"expiresAt,omitempty"`
}

// ICEServer содержит конечные точки одного из сервисов ICE и необязательные учетные данные.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// Bus — шина передает доверенное событие и его получателя между экземплярами API.
type Bus struct {
	Kind          string    `json:"kind"`
	Session       *Session  `json:"session,omitempty"`
	ConferenceID  string    `json:"conferenceId"`
	ParticipantID string    `json:"participantId,omitempty"`
	ConnectionID  string    `json:"connectionId,omitempty"`
	Event         *Envelope `json:"event,omitempty"`
}

// Subscription — подписка получает события между экземплярами до тех пор, пока не будет отменена или не будет
// вызвана функция Close.
type Subscription interface {
	// Receive ожидает следующего события, используя контекст вызывающей стороны.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: Результирующая шина.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Receive(context.Context) (Bus, error)
	// Close освобождает подписку и разблокирует Receive.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Close() error
}

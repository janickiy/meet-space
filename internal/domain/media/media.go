package media

import (
	"encoding/json"
	"time"

	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

// Ограничения на передачу медиаданных не зависят от настраиваемых ограничений на ретрансляцию в
// реальном времени. Сорок слотов для приема или публикации могут превышать 48 KiB в рамках
// допустимого предложения Firefox.
const MaxSDPBytes = 128 << 10

// Экранирование JSON может увеличить количество SDP; внутренние команды и ответы остаются
// ограниченными.
const MaxCommandBytes = 1 << 20
const MaxICEBytes = 4096

// Binding — привязка связывает одноранговый медиасервер с проверенным сеансом WebSocket. Данные,
// передаваемые браузером, не могут содержать эту информацию.
type Binding struct {
	ConferenceID           string    `json:"conferenceId"`
	ParticipantID          string    `json:"participantId"`
	SessionID              string    `json:"sessionId"`
	ConnectionID           string    `json:"connectionId"`
	UserID                 string    `json:"userId"`
	AuthorizationExpiresAt time.Time `json:"authorizationExpiresAt"`
}

// Route содержит адрес владельца медиакомнаты и версию ограждения для аутентифицированных
// внутренних вызовов.
type Route struct {
	WorkerID string `json:"workerId"`
	Endpoint string `json:"endpoint"`
	LeaseID  string `json:"leaseId"`
}

// Worker описывает присутствие медиаворкера в распределенном реестре.
type Worker struct {
	Draining bool   `json:"draining,omitempty"`
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
}

// PeerView представляет собой публичное описание физического медиасоединения.
type PeerView struct {
	MediaPeerID   string `json:"mediaPeerId"`
	ConferenceID  string `json:"conferenceId"`
	ParticipantID string `json:"participantId"`
	SessionID     string `json:"sessionId"`
	ConnectionID  string `json:"connectionId"`
}

type Kind string
type Source string

const (
	KindAudio         Kind   = "audio"
	KindVideo         Kind   = "video"
	SourceMicrophone  Source = "microphone"
	SourceCamera      Source = "camera"
	SourceVideoScreen Source = "video/screen"
	SourceAudioScreen Source = "audio/screen"
)

// Track описывает опубликованный медиатрек и его источник.
type Track struct {
	ID            string `json:"id"`
	StreamID      string `json:"streamId"`
	MediaPeerID   string `json:"mediaPeerId"`
	ParticipantID string `json:"participantId"`
	Kind          Kind   `json:"kind"`
	Source        Source `json:"source"`
}

// Publication сопоставляет SDP MID с источником. TrackID идентифицирует генерацию захвата в
// браузере, а не серверный трек.
type Publication struct {
	MID     string `json:"mid"`
	Source  Source `json:"source"`
	TrackID string `json:"trackId,omitempty"`
}

// ParticipantPolicy содержит сохраненные разрешения для источника медиаданных и монотонную
// версию модерации.
type ParticipantPolicy struct {
	Version           int64 `json:"version"`
	MicrophoneBlocked bool  `json:"microphoneBlocked"`
	CameraBlocked     bool  `json:"cameraBlocked"`
	ScreenBlocked     bool  `json:"screenBlocked"`
	Kicked            bool  `json:"kicked"`
}

// Allows сообщает, разрешает ли текущая политика выполнение запрошенной операции или
// использование источника.
//
// @Parameters:
//   - source: Медиа или источник контента, связанный с текущей операцией.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (p ParticipantPolicy) Allows(source Source) bool {
	if p.Kicked {
		return false
	}
	switch source {
	case SourceMicrophone:
		return !p.MicrophoneBlocked
	case SourceCamera:
		return !p.CameraBlocked
	case SourceVideoScreen:
		return !p.ScreenBlocked && !p.CameraBlocked
	case SourceAudioScreen:
		return !p.ScreenBlocked && !p.MicrophoneBlocked && !p.CameraBlocked
	default:
		return false
	}
}

// SourceKind нормализует метку источника медиа на поддерживаемый тип микрофона, камеры или
// экрана.
//
// @Parameters:
//   - source: Медиа или источник контента, связанный с текущей операцией.
//
// @returns:
//   - result1: Результирующий тип.
func SourceKind(source Source) Kind {
	switch source {
	case SourceMicrophone, SourceAudioScreen:
		return KindAudio
	case SourceCamera, SourceVideoScreen:
		return KindVideo
	default:
		return ""
	}
}

// Command содержит данные команд, используемые для управления медиакомнатой с аутентификацией.
type Command struct {
	RequestID     string             `json:"requestId"`
	Binding       Binding            `json:"binding"`
	Route         Route              `json:"route"`
	MediaPeerID   string             `json:"mediaPeerId,omitempty"`
	NegotiationID string             `json:"negotiationId,omitempty"`
	SDP           string             `json:"sdp,omitempty"`
	Candidate     json.RawMessage    `json:"candidate,omitempty"`
	TrackID       string             `json:"trackId,omitempty"`
	Ticket        string             `json:"ticket,omitempty"`
	Publications  []Publication      `json:"publications"`
	Policy        *ParticipantPolicy `json:"policy,omitempty"`
	ConferenceID  string             `json:"conferenceId,omitempty"`
	ParticipantID string             `json:"participantId,omitempty"`
}

// VideoCaptureTarget содержит ограничения захвата для выбранного профиля качества видео.
type VideoCaptureTarget struct {
	MaxWidth     int `json:"maxWidth"`
	MaxHeight    int `json:"maxHeight"`
	MaxFrameRate int `json:"maxFrameRate"`
}

// Result содержит результат операции с медиафайлами и метаданные связанных с ней компонентов.
type Result struct {
	MediaPeerID        string               `json:"mediaPeerId"`
	WorkerID           string               `json:"workerId,omitempty"`
	MaxPeers           int                  `json:"maxPeers,omitempty"`
	ICEServers         []realtime.ICEServer `json:"iceServers,omitempty"`
	ICETransportPolicy string               `json:"iceTransportPolicy,omitempty"`
	ICEExpiresAt       int64                `json:"iceExpiresAt,omitempty"`
	Tracks             []Track              `json:"tracks,omitempty"`
	NegotiationID      string               `json:"negotiationId,omitempty"`
	SDP                string               `json:"sdp,omitempty"`
	VideoCapture       VideoCaptureTarget   `json:"videoCapture"`
	Policy             *ParticipantPolicy   `json:"policy,omitempty"`
}

// Signal содержит ограниченную полезную информацию для передачи сигналов; сервер присваивает ей
// свой доверенный идентификатор.
type Signal struct {
	MediaPeerID   string          `json:"mediaPeerId,omitempty"`
	NegotiationID string          `json:"negotiationId,omitempty"`
	SDP           string          `json:"sdp,omitempty"`
	Candidate     json.RawMessage `json:"candidate,omitempty"`
	TrackID       string          `json:"trackId,omitempty"`
	Publications  []Publication   `json:"publications,omitempty"`
}

// PolicyForParticipant проецирует авторизованное членство в систему на разрешения для
// версионных медиафайлов. Доступность конференции контролируется отдельно ее владельцем.
//
// @Parameters:
//   - p: Состояние участника, обработанное в ходе этой операции.
//
// @returns:
//   - result1: Результирующая политика в отношении участника.
func PolicyForParticipant(p conferences.Participant) ParticipantPolicy {
	return ParticipantPolicy{Version: p.MediaPolicyVersion, MicrophoneBlocked: p.MicrophoneBlocked, CameraBlocked: p.CameraBlocked, ScreenBlocked: p.ScreenBlocked, Kicked: !p.CanParticipate()}
}

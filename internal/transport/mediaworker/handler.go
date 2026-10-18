// Пакет mediaworker предоставляет только аутентифицированные внутренние сигналы.  RTP никогда
// не проходит через эти обработчики HTTP.
package mediaworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/pion/webrtc/v4"
)

// Engine определяет заменяемый контракт зависимостей для медиаворкера.
type Engine interface {
	// Join создает или возобновляет авторизованное членство или подключение к медиакомнате,
	// сохраняя текущие идентификационные данные.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
	//
	// @returns:
	//   - result1: Полученное представление одноранговых узлов.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Join(context.Context, media.Binding) (media.PeerView, error)
	// Offer обрабатывает предложение SDP для текущего подключенного соединения WebRTC.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Идентификатор, связывающий это предложение с его циклом согласования.
	//   - arg4: Недоверенное сериализованное значение для декодирования или проверки.
	//
	// @returns:
	//   - result1: Полученное описание сеанса.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Offer(context.Context, string, string, string) (webrtc.SessionDescription, error)
	// Ready принимает подтверждение того, что клиент применил ответ SDP, и активирует треки
	// только для текущего согласования.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Идентификатор, связывающий это предложение с его циклом согласования.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Ready(context.Context, string, string) error
	// ICE применяет валидированный кандидат ICE к текущему медиасоединению.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Сессия кандидата или запись ICE для проверки перед использованием.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ICE(context.Context, string, *webrtc.ICECandidateInit) error
	// Unpublish прекращает публикацию указанного авторизованного источника медиаданных.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Идентификатор опубликованного медиа-трека.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Unpublish(context.Context, string, string) error
	// Leave отменяет активное членство или подключение к мультимедиа, сохраняя при этом
	// сохранённую историю разговоров или конференций.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Leave(context.Context, string) error
	// LeaveConnection отключает только указанное физическое медиасоединение.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор физического подключения к медиа.
	LeaveConnection(context.Context, string)
	// PeerBinding возвращает идентификационные данные сеанса на стороне сервера, привязанные к
	// одноранговому соединению.
	//
	// @Parameters:
	//   - arg1: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Полученная привязка.
	//   - result2: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	PeerBinding(string) (media.Binding, bool)
	// Bindings возвращает текущие идентификационные данные однорангового узла для запрошенной
	// медиакомнаты.
	//
	// @returns:
	//   - result1: Собранные значения привязок.
	Bindings() []media.Binding
	// Tracks возвращает моментальный снимок видимых опубликованных треков запрашиваемой
	// комнаты.
	//
	// @Parameters:
	//   - arg1: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Собранные значения треков.
	Tracks(string) []media.Track
	// CloseConference закрывает медиаузлы и принадлежащие ресурсы одной конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CloseConference(context.Context, string) error
	// Shutdown останавливает выполнение новых задач и ожидает завершения закрытия ресурсов,
	// принадлежащих компоненту.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Shutdown(context.Context) error
}

// Registry определяет заменяемый контракт зависимостей для медиаворкера.
type Registry interface {
	// RegisterWorker сохраняет текущий адрес медиа-воркера, его мощность и ограниченный срок
	// присутствия.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Текущие метаданные медиа-воркера и информация о владельце.
	//   - arg3: Срок действия, назначенный для временного значения или записи о
	//   владельце.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RegisterWorker(context.Context, media.Worker, time.Duration) error
	// GetOwner загружает текущую версию медиакомнаты с распределенным владельцем и защитой от
	// несанкционированного доступа.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Результирующий маршрут.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GetOwner(context.Context, string) (media.Route, error)
	// Renew продлевает аренду медиаворкера только до тех пор, пока совпадают его владелец и
	// идентификатор ограждения.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Текущий владелец медиа-комнаты и версия ограждения.
	//   - arg4: Срок действия, назначенный для временного значения или записи о
	//   владельце.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Renew(context.Context, string, media.Route, time.Duration) error
	// Release освобождает только тот ресурс медиаворкера, который принадлежит соответствующей
	// операции или токену.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Текущий владелец медиа-комнаты и версия ограждения.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Release(context.Context, string, media.Route) error
	// RemoveWorker удаляет данные о присутствии указанного медиа-воркера из реестра.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор работника, которому принадлежит эта операция.
	//   - arg3: Конечная точка сервиса, настраиваемая оператором.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RemoveWorker(context.Context, string, string) error
}

// Sessions — сессии определяют заменяемый контракт зависимостей для медиаворкера.
type Sessions interface {
	// Get загружает запрошенный медиаворкер в авторизованную область действия вызывающей
	// стороны.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Результирующий сеанс.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Get(context.Context, string) (realtime.Session, error)
}

// Tickets — билеты определяют заменяемый контракт зависимостей для медиаворкера.
type Tickets interface {
	// Verify проверяет предоставленные учётные данные или подписанную привязку, прежде чем
	// подтвердить личность вызывающего абонента.
	//
	// @Parameters:
	//   - arg1: Недоверенное сериализованное значение для декодирования или проверки.
	//
	// @returns:
	//   - result1: Полученная привязка.
	//   - result2: Результирующий маршрут.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Verify(string) (media.Binding, media.Route, error)
}

// offerCache сохраняет согласованный результат SDP, чтобы можно было безопасно обрабатывать
// повторяющиеся предложения.
type offerCache struct {
	mu     sync.Mutex
	id     string
	hash   [32]byte
	answer string
}

// roomGate сериализует операции управления для одной медиакомнаты.
type roomGate struct {
	token chan struct{}
	refs  int
}

// roomLease содержит локальное подтверждение права собственности на медиаворкера и информацию о
// сроке его действия.
type roomLease struct {
	route    media.Route
	deadline time.Time
}

// Handler — обработчик адаптирует запросы HTTP для медиаворкера, проверяет входные данные и формирует
// ответ.
type Handler struct {
	registrationMu sync.Mutex
	draining       bool
	cfg            config.MediaConfig
	registry       Registry
	sessions       Sessions
	tickets        Tickets
	engine         Engine
	logger         *slog.Logger
	metrics        func() any
	ready          atomic.Bool
	gate           chan struct{}
	mu             sync.Mutex
	leases         map[string]roomLease
	offers         map[string]*offerCache
	roomGates      map[string]*roomGate
	workerDeadline time.Time
	fencing        bool
	closing        bool
}

// NewHandler создает обработчик с зависимостями и ограничениями, необходимыми для медиаворкера.
//
// @Parameters:
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//   - registry: Реестр зависимостей или метрик, используемый для поиска или регистрации
//     компонента.
//   - sessions: Зависимость авторизации физического сеанса.
//   - tickets: Одноразовый или подписанный эмитент и верификатор медиа-билетов.
//   - engine: Медиадвижок, которому принадлежат комната и ресурсы одноранговых узлов.
//   - metrics: Обратный вызов, возвращающий ограниченные операционные показатели.
//   - logger: Ограниченный диагностический логгер.
//
// @returns:
//   - result1: Сконструированный или загруженный обработчик.
func NewHandler(cfg config.MediaConfig, registry Registry, sessions Sessions, tickets Tickets, engine Engine, metrics func() any, logger *slog.Logger) *Handler {
	return &Handler{cfg: cfg, registry: registry, sessions: sessions, tickets: tickets, engine: engine, metrics: metrics, logger: logger, gate: make(chan struct{}, 128), leases: make(map[string]roomLease), offers: make(map[string]*offerCache), roomGates: make(map[string]*roomGate)}
}

// lockRoom сериализует управляющую операцию для одной медиакомнаты и возвращает обратный вызов
// для разблокировки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Обратный вызов, используемый для завершения операции или освобождения
//     ресурсов.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Handler) lockRoom(ctx context.Context, id string) (func(), error) {
	if ctx.Err() != nil {
		return nil, media.ErrUnavailable
	}
	h.mu.Lock()
	gate := h.roomGates[id]
	if gate == nil {
		gate = &roomGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		h.roomGates[id] = gate
	}
	gate.refs++
	h.mu.Unlock()

	releaseRef := func() {
		h.mu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(h.roomGates, id)
		}
		h.mu.Unlock()
	}
	select {
	case <-ctx.Done():
		releaseRef()
		return nil, media.ErrUnavailable
	case <-gate.token:

		return func() { gate.token <- struct{}{}; releaseRef() }, nil
	}
}

// Routes создает внутренний маршрутизатор HTTP медиа-воркера с проверкой аутентификации и
// готовности.
//
// @returns:
//   - result1: Результирующий обработчик.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz",

		func(w http.ResponseWriter, _ *http.Request) {
			if !h.Ready() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	mux.HandleFunc("GET /internal/media/metrics", h.auth(

		func(w http.ResponseWriter, _ *http.Request) { h.json(w, http.StatusOK, h.metrics()) }))
	for _, operation := range []string{"join", "offer", "ready", "ice", "leave", "unpublish", "policy", "close"} {
		op := operation
		mux.HandleFunc("POST /internal/media/"+op, h.auth(

			func(w http.ResponseWriter, r *http.Request) { h.command(w, r, op) }))
	}
	mux.HandleFunc("POST /internal/media/egress", h.auth(h.egress))
	return mux
}

// auth проверяет защищенные учетные данные для запроса API к медиа-воркеру.
//
// @Parameters:
//   - next: Следующий обработчик HTTP или более новое событие, в зависимости от его типа.
//
// @returns:
//   - result1: Обработчик HTTP для регистрации этой операции.
func (h *Handler) auth(next http.HandlerFunc) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == r.Header.Get("Authorization") || subtle.ConstantTimeCompare([]byte(provided), []byte(h.cfg.InternalSecret)) != 1 {
			h.fail(w, media.ErrUnauthorized)
			return
		}
		next(w, r)
	}
}

// strictJSON декодирует ограниченный JSON с отбрасыванием неизвестных полей и завершающих
// данных.
//
// @Parameters:
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//   - target: Целевое состояние, учетная запись или декодированное значение в соответствии с
//     типом.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func strictJSON(raw []byte, target any) error {
	if !utf8.Valid(raw) {
		return media.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return media.ErrInvalid
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return media.ErrInvalid
	}
	return nil
}

// validUUID проверяет, что идентификатор является ненулевым UUID, прежде чем использовать его
// для хранения или маршрутизации.
//
// @Parameters:
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func validUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil && parsed.String() == id
}

// sameBinding сравнивает полные идентификационные данные на стороне сервера для двух привязок
// мультимедиа.
//
// @Parameters:
//   - a: Сравниваемое значение привязки.
//   - b: Сравниваемое значение привязки.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func sameBinding(a, b media.Binding) bool {
	return a.ConferenceID == b.ConferenceID && a.ParticipantID == b.ParticipantID && a.SessionID == b.SessionID && a.ConnectionID == b.ConnectionID && a.UserID == b.UserID && a.AuthorizationExpiresAt.Equal(b.AuthorizationExpiresAt)
}

// sameRoute сравнивает владельца медиакомнаты и версию ограждения в двух записях маршрутизации.
//
// @Parameters:
//   - a: Сравниваемое значение маршрута.
//   - b: Сравниваемое значение маршрута.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func sameRoute(a, b media.Route) bool {
	return a.WorkerID == b.WorkerID && a.Endpoint == b.Endpoint && a.LeaseID == b.LeaseID
}

// active проверяет, есть ли у активного ресурса необходимые идентификационные данные для сеанса
// или владения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - binding: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Handler) active(ctx context.Context, binding media.Binding) error {
	if !binding.AuthorizationExpiresAt.After(time.Now()) {
		return media.ErrUnauthorized
	}
	s, err := h.sessions.Get(ctx, binding.ConnectionID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, media.ErrUnauthorized) || errors.Is(err, media.ErrPeerNotFound) {
			return media.ErrUnauthorized
		}
		return media.ErrUnavailable
	}
	if s.ID != binding.SessionID || s.ConferenceID != binding.ConferenceID || s.ParticipantID != binding.ParticipantID || s.ConnectionID != binding.ConnectionID || s.UserID != binding.UserID || s.Status != "connected" {
		return media.ErrUnauthorized
	}
	return nil
}

// command — команда строго декодирует и выполняет разрешенную внутреннюю команду управления мультимедиа.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - r: Входящий запрос HTTP.
//   - operation: Фиксированный провайдер из списка разрешенных или операция протокола.
func (h *Handler) command(w http.ResponseWriter, r *http.Request, operation string) {
	select {
	case h.gate <- struct{}{}:
		defer func() { <-h.gate }()
	default:
		h.fail(w, media.ErrLimit)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, media.MaxCommandBytes)
	raw, err := io.ReadAll(r.Body)
	var cmd media.Command
	if err != nil || strictJSON(raw, &cmd) != nil || !validUUID(cmd.RequestID) || r.Header.Get("X-Request-ID") != cmd.RequestID {
		h.fail(w, media.ErrInvalid)
		return
	}
	w.Header().Set("X-Request-ID", cmd.RequestID)
	if operation == "policy" || operation == "close" {
		h.serviceCommand(w, r, operation, cmd)
		return
	}
	for _, id := range []string{cmd.Binding.ConferenceID, cmd.Binding.ParticipantID, cmd.Binding.SessionID, cmd.Binding.ConnectionID, cmd.Binding.UserID, cmd.Route.LeaseID} {
		if !validUUID(id) {
			h.fail(w, media.ErrInvalid)
			return
		}
	}
	if cmd.Route.WorkerID != h.cfg.WorkerID || cmd.Route.Endpoint != h.cfg.WorkerInternalURL {
		h.fail(w, media.ErrOwnership)
		return
	}
	if operation != "join" && !validUUID(cmd.MediaPeerID) {
		h.fail(w, media.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.OperationTimeout)
	defer cancel()
	if operation == "join" || operation == "leave" {
		unlock, err := h.lockRoom(ctx, cmd.Binding.ConferenceID)
		if err != nil {
			h.fail(w, err)
			return
		}
		defer unlock()
	}
	if operation == "leave" {
		// Истечение срока действия авторизации или потеря права собственности не должны
		// препятствовать очистке ресурсов.
		if binding, ok := h.engine.PeerBinding(cmd.MediaPeerID); ok && !sameBinding(binding, cmd.Binding) {
			h.fail(w, media.ErrUnauthorized)
			return
		}
		err = h.engine.Leave(ctx, cmd.MediaPeerID)
		h.mu.Lock()
		delete(h.offers, cmd.MediaPeerID)
		h.mu.Unlock()
		if err != nil {
			h.fail(w, err)
			return
		}
		// При периодической очистке освобождается пустая комната, находящаяся под той же
		// блокировкой.
		h.json(w, http.StatusOK, media.Result{MediaPeerID: cmd.MediaPeerID})
		return
	}
	if !h.ready.Load() {
		h.fail(w, media.ErrUnavailable)
		return
	}
	owner, err := h.registry.GetOwner(ctx, cmd.Binding.ConferenceID)
	if err != nil || !sameRoute(owner, cmd.Route) {
		h.fail(w, media.ErrOwnership)
		return
	}
	if err := h.active(ctx, cmd.Binding); err != nil {
		h.fail(w, err)
		return
	}
	if operation == "join" {
		if cmd.Policy != nil && (cmd.Policy.Version < 0 || cmd.Policy.Kicked) {
			h.fail(w, media.ErrPolicy)
			return
		}
		binding, route, err := h.tickets.Verify(cmd.Ticket)
		if err != nil || !sameBinding(binding, cmd.Binding) || !sameRoute(route, cmd.Route) {
			h.fail(w, media.ErrUnauthorized)
			return
		}
		renewedAt := time.Now()
		if err := h.registry.Renew(ctx, binding.ConferenceID, route, h.cfg.OwnershipTTL); err != nil {
			h.fail(w, media.ErrOwnership)
			return
		}
		// При повторном назначении комнаты этому процессу состояние предыдущей аренды не должно
		// использоваться повторно.  Сериализуйте операции входа и выхода из комнаты.
		h.mu.Lock()
		old, existed := h.leases[binding.ConferenceID]
		h.mu.Unlock()
		if existed && !sameRoute(old.route, route) {
			_ = h.engine.CloseConference(ctx, binding.ConferenceID)
		}
		h.mu.Lock()
		admitted := !h.draining && !h.fencing && !h.closing && h.workerDeadline.After(time.Now()) && renewedAt.Add(h.cfg.OwnershipTTL).After(time.Now())
		if admitted {
			h.leases[binding.ConferenceID] = roomLease{route: route, deadline: renewedAt.Add(h.cfg.OwnershipTTL)}
		}
		h.mu.Unlock()
		if !admitted {
			h.fail(w, media.ErrUnavailable)
			return
		}
		// При корректном выходе и повторном входе используется новая сохраненная политика.
		// Примените ее к существующей комнате перед входом и еще раз — к вновь созданной
		// комнате.
		if cmd.Policy != nil {
			engine, ok := h.engine.(interface {
				// SetPolicy обновляет политику для медиаворкера.
				//
				// @Parameters:
				//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность
				//   выполнения операции.
				//   - arg2: Идентификатор конференции, ограничивающий область действия
				//   операции.
				//   - arg3: Идентификатор участника конференции.
				//   - arg4: Эффективная политика в отношении медиаданных на стороне
				//   сервера.
				//
				// @returns:
				//   - result1: nil — в случае успеха; в противном случае — ошибка
				//   валидации, доступа, передачи данных или сохранения.
				SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
			})
			if !ok {
				h.fail(w, media.ErrUnavailable)
				return
			}
			if err = engine.SetPolicy(ctx, binding.ConferenceID, binding.ParticipantID, *cmd.Policy); err != nil {
				h.fail(w, err)
				return
			}
		}
		peer, err := h.engine.Join(ctx, binding)
		if err != nil {
			h.fail(w, err)
			return
		}
		if cmd.Policy != nil {
			if policyEngine, ok := h.engine.(interface {
				// SetPolicy обновляет политику для медиаворкера.
				//
				// @Parameters:
				//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность
				//   выполнения операции.
				//   - arg2: Идентификатор конференции, ограничивающий область действия
				//   операции.
				//   - arg3: Идентификатор участника конференции.
				//   - arg4: Эффективная политика в отношении медиаданных на стороне
				//   сервера.
				//
				// @returns:
				//   - result1: nil — в случае успеха; в противном случае — ошибка
				//   валидации, доступа, передачи данных или сохранения.
				SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
			}); ok {
				if err = policyEngine.SetPolicy(ctx, binding.ConferenceID, binding.ParticipantID, *cmd.Policy); err != nil {
					_ = h.engine.Leave(ctx, peer.MediaPeerID)
					h.fail(w, err)
					return
				}
			} else {
				_ = h.engine.Leave(ctx, peer.MediaPeerID)
				h.fail(w, media.ErrUnavailable)
				return
			}
		}
		if !h.leaseValid(binding.ConferenceID, route) {
			_ = h.engine.Leave(ctx, peer.MediaPeerID)
			h.fail(w, media.ErrOwnership)
			return
		}
		h.logger.Info("media peer admitted", "conference_id", binding.ConferenceID, "participant_id", binding.ParticipantID, "session_id", binding.SessionID, "media_peer_id", peer.MediaPeerID, "worker_id", h.cfg.WorkerID, "request_id", cmd.RequestID)
		policy := cmd.Policy
		if provider, ok := h.engine.(interface {
			// ParticipantPolicy возвращает текущие действующие ограничения на доступ к
			// медиаданным для участника.
			//
			// @Parameters:
			//   - arg1: Идентификатор конференции, ограничивающий область действия
			//   операции.
			//   - arg2: Идентификатор участника конференции.
			//
			// @returns:
			//   - result1: Результирующая политика в отношении участника.
			//   - result2: Указывает, выполняется ли задокументированное условие операции и
			//   произошло ли успешное изменение ее условного состояния.
			ParticipantPolicy(string, string) (media.ParticipantPolicy, bool)
		}); ok {
			if current, exists := provider.ParticipantPolicy(binding.ConferenceID, binding.ParticipantID); exists {
				policy = &current
			}
		}
		ice := h.cfg.TURN.ClientICE(h.cfg.ICE, time.Now())
		h.json(w, http.StatusOK, media.Result{MediaPeerID: peer.MediaPeerID, WorkerID: h.cfg.WorkerID, MaxPeers: h.cfg.MaxPeers, ICEServers: ice.ICEServers, ICETransportPolicy: ice.ICETransportPolicy, ICEExpiresAt: ice.ExpiresAt, Tracks: h.engine.Tracks(peer.MediaPeerID), Policy: policy, VideoCapture: media.VideoCaptureTarget{MaxWidth: h.cfg.VideoMaxWidth, MaxHeight: h.cfg.VideoMaxHeight, MaxFrameRate: h.cfg.VideoMaxFPS}})
		return
	}
	binding, ok := h.engine.PeerBinding(cmd.MediaPeerID)
	if !ok {
		h.fail(w, media.ErrPeerNotFound)
		return
	}
	if !sameBinding(binding, cmd.Binding) {
		h.fail(w, media.ErrUnauthorized)
		return
	}
	result := media.Result{MediaPeerID: cmd.MediaPeerID}
	switch operation {
	case "offer":
		if !validUUID(cmd.NegotiationID) || len(cmd.SDP) == 0 || len(cmd.SDP) > media.MaxSDPBytes {
			h.fail(w, media.ErrInvalid)
			return
		}
		if cmd.Policy != nil {
			policyEngine, ok := h.engine.(interface {
				// SetPolicy обновляет политику для медиаворкера.
				//
				// @Parameters:
				//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность
				//   выполнения операции.
				//   - arg2: Идентификатор конференции, ограничивающий область действия
				//   операции.
				//   - arg3: Идентификатор участника конференции.
				//   - arg4: Эффективная политика в отношении медиаданных на стороне
				//   сервера.
				//
				// @returns:
				//   - result1: nil — в случае успеха; в противном случае — ошибка
				//   валидации, доступа, передачи данных или сохранения.
				SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
			})
			if !ok {
				h.fail(w, media.ErrUnavailable)
				return
			}
			if err = policyEngine.SetPolicy(ctx, binding.ConferenceID, binding.ParticipantID, *cmd.Policy); err != nil {
				h.fail(w, err)
				return
			}
			if cmd.Policy.Kicked {
				h.fail(w, media.ErrPolicy)
				return
			}
		}
		h.mu.Lock()
		cache := h.offers[cmd.MediaPeerID]
		if cache == nil {
			cache = &offerCache{}
			h.offers[cmd.MediaPeerID] = cache
		}
		h.mu.Unlock()
		cache.mu.Lock()
		metadata, _ := json.Marshal(cmd.Publications)
		hash := sha256.Sum256(append(append([]byte(cmd.SDP), 0), metadata...))
		if cache.id == cmd.NegotiationID {
			if cache.hash != hash {
				err = media.ErrNegotiation
			} else {
				result.SDP = cache.answer
			}
		} else {
			var answer webrtc.SessionDescription
			if engine, ok := h.engine.(interface {
				// OfferSources обрабатывает предложение SDP с явными декларациями о
				// медиаисточниках.
				//
				// @Parameters:
				//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность
				//   выполнения операции.
				//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
				//   - arg3: Идентификатор, связывающий это предложение с его циклом
				//   согласования.
				//   - arg4: Недоверенное сериализованное значение для декодирования или
				//   проверки.
				//   - arg5: Проверенные декларации о медиаисточниках пиров.
				//
				// @returns:
				//   - result1: Полученное описание сеанса.
				//   - result2: nil — в случае успеха; в противном случае — ошибка
				//   валидации, доступа, передачи данных или сохранения.
				OfferSources(context.Context, string, string, string, []media.Publication) (webrtc.SessionDescription, error)
			}); ok {
				answer, err = engine.OfferSources(ctx, cmd.MediaPeerID, cmd.NegotiationID, cmd.SDP, cmd.Publications)
			} else if cmd.Publications != nil {
				err = media.ErrInvalid
			} else {
				answer, err = h.engine.Offer(ctx, cmd.MediaPeerID, cmd.NegotiationID, cmd.SDP)
			}
			if err == nil {
				result.SDP = answer.SDP
				cache.id, cache.hash, cache.answer = cmd.NegotiationID, hash, answer.SDP
			}
		}
		cache.mu.Unlock()
		result.NegotiationID = cmd.NegotiationID
	case "ready":
		if !validUUID(cmd.NegotiationID) || cmd.SDP != "" || len(cmd.Candidate) != 0 || cmd.TrackID != "" {
			h.fail(w, media.ErrInvalid)
			return
		}
		h.mu.Lock()
		cache := h.offers[cmd.MediaPeerID]
		h.mu.Unlock()
		if cache == nil {
			h.fail(w, media.ErrNegotiation)
			return
		}
		cache.mu.Lock()
		if cache.id != cmd.NegotiationID {
			err = media.ErrNegotiation
		} else {
			err = h.engine.Ready(ctx, cmd.MediaPeerID, cmd.NegotiationID)
		}
		cache.mu.Unlock()
	case "ice":
		if len(cmd.Candidate) == 0 || len(cmd.Candidate) > media.MaxICEBytes {
			h.fail(w, media.ErrInvalid)
			return
		}
		var candidate *webrtc.ICECandidateInit
		if string(cmd.Candidate) != "null" {
			candidate = &webrtc.ICECandidateInit{}
			if strictJSON(cmd.Candidate, candidate) != nil {
				h.fail(w, media.ErrInvalid)
				return
			}
		}
		err = h.engine.ICE(ctx, cmd.MediaPeerID, candidate)
	case "unpublish":
		if !validUUID(cmd.TrackID) {
			h.fail(w, media.ErrInvalid)
			return
		}
		err = h.engine.Unpublish(ctx, cmd.MediaPeerID, cmd.TrackID)
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.json(w, http.StatusOK, result)
}

// fail регистрирует сбой безопасного медиаворкера и запускает процесс очистки ресурсов,
// принадлежащих этому воркеру.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
func (h *Handler) fail(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, media.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, media.ErrLimit):
		status = http.StatusTooManyRequests
	case errors.Is(err, media.ErrPeerNotFound):
		status = http.StatusNotFound
	case errors.Is(err, media.ErrOwnership), errors.Is(err, media.ErrNegotiation), errors.Is(err, media.ErrScreenConflict):
		status = http.StatusConflict
	case errors.Is(err, media.ErrPolicy):
		status = http.StatusForbidden
	case errors.Is(err, media.ErrUnavailable):
		status = http.StatusServiceUnavailable
	}
	h.json(w, status, map[string]string{"code": media.ErrorCode(err)})
}

// json сериализует полезную нагрузку в ответ JSON с выбранным статусом HTTP.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - status: Состояние жизненного цикла или статус ответа на запись.
//   - data: Входные байты или структурированная полезная информация о событии.
func (h *Handler) json(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// Start запускает медиаворкера и подготавливает ресурсы, принадлежащие этой операции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Ограниченное событие или медиаканал, принадлежащий этой операции.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Handler) Start(ctx context.Context) (<-chan struct{}, error) {
	startedAt := time.Now()
	operation, cancel := context.WithTimeout(ctx, h.cfg.OperationTimeout)
	err := h.registry.RegisterWorker(operation, media.Worker{ID: h.cfg.WorkerID, Endpoint: h.cfg.WorkerInternalURL}, h.cfg.WorkerTTL)
	cancel()
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.workerDeadline = startedAt.Add(h.cfg.WorkerTTL)
	h.updateReadyLocked()
	h.mu.Unlock()
	done := make(chan struct{})
	var workers sync.WaitGroup

	run := func(interval time.Duration, tick func(context.Context)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					h.ready.Store(false)
					return
				case <-ticker.C:
					tick(ctx)
				}
			}
		}()
	}
	run(h.cfg.HeartbeatInterval, h.heartbeat)
	run(h.cfg.SessionCheckInterval, h.sweep)
	fenceInterval := min(h.cfg.WorkerTTL, h.cfg.OwnershipTTL) / 4
	fenceInterval = max(10*time.Millisecond, min(time.Second, fenceInterval))
	run(fenceInterval, h.watchdog)
	go func() { workers.Wait(); close(done) }()
	return done, nil
}

// updateReadyLocked обновляет флаг готовности воркера, удерживая блокировку состояния.
func (h *Handler) updateReadyLocked() {
	now := time.Now()
	valid := !h.fencing && !h.closing && h.workerDeadline.After(now)
	for _, lease := range h.leases {
		if !lease.deadline.After(now) {
			valid = false
			break
		}
	}
	h.ready.Store(valid)
}

// leaseValid проверяет, что комната по-прежнему принадлежит этому воркеру с ожидаемой версией
// ограждения.
//
// @Parameters:
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - route: Текущий владелец медиа-комнаты и версия ограждения.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (h *Handler) leaseValid(conferenceID string, route media.Route) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	lease, ok := h.leases[conferenceID]
	return ok && !h.fencing && !h.closing && h.workerDeadline.After(time.Now()) && lease.deadline.After(time.Now()) && sameRoute(lease.route, route)
}

// watchdog закрывает медиаресурсы, которые утратили авторизацию или право владения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (h *Handler) watchdog(ctx context.Context) {
	h.mu.Lock()
	now := time.Now()
	expired := !h.fencing && !h.closing && !h.workerDeadline.After(now)
	for _, lease := range h.leases {
		if !lease.deadline.After(now) {
			expired = true
			break
		}
	}
	h.mu.Unlock()
	if expired {
		h.fence(ctx, "local_lease_expired")
	}
}

// fence не позволяет зависшему воркеру продолжать обработку медиаданных после утраты права
// владения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - reason: Ограниченный жизненный цикл или причина сбоя.
func (h *Handler) fence(ctx context.Context, reason string) {
	h.mu.Lock()
	if h.fencing || h.closing {
		h.mu.Unlock()
		return
	}
	if reason == "local_lease_expired" {
		now := time.Now()
		expired := !h.workerDeadline.After(now)
		for _, lease := range h.leases {
			if !lease.deadline.After(now) {
				expired = true
				break
			}
		}
		if !expired {
			h.mu.Unlock()
			return
		} // Параллельное обновление завершается первым.
	}
	h.fencing = true
	h.ready.Store(false)
	h.workerDeadline = time.Time{}
	rooms := make(map[string]bool)
	for id := range h.leases {
		rooms[id] = true
	}
	h.leases = make(map[string]roomLease)
	h.mu.Unlock()
	h.logger.Warn("media worker fenced", "worker_id", h.cfg.WorkerID, "event_type", reason)
	for _, binding := range h.engine.Bindings() {
		rooms[binding.ConferenceID] = true
	}
	var closed sync.WaitGroup
	for id := range rooms {
		closed.Add(1)
		go func(id string) { defer closed.Done(); _ = h.engine.CloseConference(ctx, id) }(id)
	}
	closed.Wait()
	h.mu.Lock()
	h.offers = make(map[string]*offerCache)
	h.fencing = false
	// Воркер снова становится готовым к работе только после успешного подтверждения доступности
	// по истечении срока аренды.
	h.ready.Store(false)
	h.mu.Unlock()
}

// heartbeat периодически обновляет информацию о присутствии воркера в сети.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (h *Handler) heartbeat(ctx context.Context) {
	h.registrationMu.Lock()
	defer h.registrationMu.Unlock()
	h.mu.Lock()
	draining := h.draining
	h.mu.Unlock()
	operation, cancel := context.WithTimeout(ctx, h.cfg.OperationTimeout)
	defer cancel()
	startedAt := time.Now()
	if err := h.registry.RegisterWorker(operation, media.Worker{ID: h.cfg.WorkerID, Endpoint: h.cfg.WorkerInternalURL, Draining: draining}, h.cfg.WorkerTTL); err != nil {
		h.fence(operation, "registry_unavailable")
		return
	}
	h.mu.Lock()
	h.workerDeadline = startedAt.Add(h.cfg.WorkerTTL)
	leases := make(map[string]roomLease, len(h.leases))
	for id, lease := range h.leases {
		leases[id] = lease
	}
	h.mu.Unlock()
	for id, lease := range leases {
		renewedAt := time.Now()
		// Обновление присутствия не должно дожидаться медленной блокировки комнаты,
		// удерживаемой во время регистрации или выхода.
		if err := h.registry.Renew(operation, id, lease.route, h.cfg.OwnershipTTL); err != nil {
			if !errors.Is(err, media.ErrOwnership) {
				h.fence(operation, "registry_unavailable")
				return
			}
			unlock, err := h.lockRoom(operation, id)
			if err != nil {
				h.fence(operation, "lease_cleanup_timeout")
				return
			}
			h.mu.Lock()
			current, ok := h.leases[id]
			matches := ok && sameRoute(current.route, lease.route)
			if matches {
				delete(h.leases, id)
			}
			h.mu.Unlock()
			if matches {
				_ = h.engine.CloseConference(operation, id)
				h.logger.Warn("media room ownership lost", "conference_id", id, "worker_id", h.cfg.WorkerID)
			}
			unlock()
			continue
		}
		h.mu.Lock()
		if current, ok := h.leases[id]; ok && sameRoute(current.route, lease.route) {
			current.deadline = renewedAt.Add(h.cfg.OwnershipTTL)
			h.leases[id] = current
		}
		h.mu.Unlock()
	}
	h.releaseEmpty(operation)
	h.mu.Lock()
	h.updateReadyLocked()
	h.mu.Unlock()
}

// sweep удаляет сеансы с истекшим сроком действия и закрывает ресурсы, к которым больше нет
// действительного доступа.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (h *Handler) sweep(ctx context.Context) {
	operation, cancel := context.WithTimeout(ctx, h.cfg.OperationTimeout)
	defer cancel()
	for _, binding := range h.engine.Bindings() {
		if operation.Err() != nil {
			h.fence(operation, "session_sweep_timeout")
			return
		}
		err := h.active(operation, binding)
		if errors.Is(err, media.ErrUnavailable) {
			h.fence(operation, "session_store_unavailable")
			return
		}
		if err != nil {
			unlock, err := h.lockRoom(operation, binding.ConferenceID)
			if err != nil {
				h.fence(operation, "session_cleanup_timeout")
				return
			}
			h.engine.LeaveConnection(operation, binding.ConnectionID)
			unlock()
		}
	}
	h.releaseEmpty(operation)
}

// releaseEmpty освобождает от ответственности владельцев медиа-комнат, которые не имеют
// активных подключений.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (h *Handler) releaseEmpty(ctx context.Context) {
	h.mu.Lock()
	leases := make(map[string]roomLease, len(h.leases))
	for id, lease := range h.leases {
		leases[id] = lease
	}
	for id := range h.offers {
		if _, ok := h.engine.PeerBinding(id); !ok {
			delete(h.offers, id)
		}
	}
	h.mu.Unlock()
	for id, lease := range leases {
		unlock, err := h.lockRoom(ctx, id)
		if err != nil {
			return
		}
		active := false
		if engine, ok := h.engine.(interface {
			// HasConference сообщает, является ли данный менеджер владельцем запрашиваемого
			// конференц-зала.
			//
			// @Parameters:
			//   - arg1: Идентификатор ресурса, к которому обращается данная операция.
			//
			// @returns:
			//   - result1: Указывает, выполняется ли задокументированное условие операции и
			//   произошло ли успешное изменение ее условного состояния.
			HasConference(string) bool
		}); ok {
			active = engine.HasConference(id)
		}
		for _, binding := range h.engine.Bindings() {
			if binding.ConferenceID == id {
				active = true
				break
			}
		}
		if !active {
			h.mu.Lock()
			if current, ok := h.leases[id]; ok && sameRoute(current.route, lease.route) {
				delete(h.leases, id)
			}
			h.mu.Unlock()
			_ = h.registry.Release(ctx, id, lease.route)
		}
		unlock()
	}
}

// Stop останавливает активную работу mediaworker и освобождает принадлежащие ему ресурсы.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Handler) Stop(ctx context.Context) error {
	h.mu.Lock()
	h.closing = true
	h.ready.Store(false)
	h.mu.Unlock()
	err := h.engine.Shutdown(ctx)
	h.releaseEmpty(ctx)
	_ = h.registry.RemoveWorker(ctx, h.cfg.WorkerID, h.cfg.WorkerInternalURL)
	return err
}

// Ready сообщает, можно ли в данный момент использовать компонент или настроенную зависимость.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (h *Handler) Ready() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ready.Load() && !h.draining
}

// BeginDrain прекращает принимать новые задачи, пока не завершатся текущие операции.
func (h *Handler) BeginDrain() {
	h.mu.Lock()
	h.draining = true
	h.mu.Unlock()
	h.heartbeat(context.Background())
}

// ActiveRooms подсчитывает комнаты с активными участниками, чтобы при развертывании можно было
// обнаружить, что передача мультимедиа может быть прервана.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
func (h *Handler) ActiveRooms() int {
	rooms := make(map[string]struct{})
	for _, binding := range h.engine.Bindings() {
		rooms[binding.ConferenceID] = struct{}{}
	}
	return len(rooms) + len(h.gate)
}

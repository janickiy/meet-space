// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package realtime

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
)

// Repository — репозиторий определяет заменяемый контракт зависимостей для физических сеансов и доставки
// событий в режиме реального времени.
type Repository interface {
	// Authorize проверяет текущие идентификационные данные и сохраненный статус доступа перед
	// физическими сеансами и доставкой событий в режиме реального времени.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: Полученный участник.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Authorize(context.Context, string, string) (conferences.Participant, error)
	// Open открывает доступ к медиафайлам, записям или сеансам, необходимым для работы этого
	// компонента.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Постоянное или физическое состояние сессии, определяемое ее типом.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Open(context.Context, domain.Session) error
	// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор физического подключения к медиа.
	//   - arg3: Контрольная временная метка подтвержденной активности транспорта.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Close(context.Context, string, time.Time) error
	// Stale возвращает данные об исторических физических сеансах, которые остаются открытыми
	// после достижения порога активности.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Порог, после достижения которого выбираются результаты или состояние.
	//   - arg3: Непрозрачная граница продолжения с предыдущей страницы.
	//
	// @returns:
	//   - result1: Собранные значения сеанса.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Stale(context.Context, time.Time, string) ([]domain.Session, error)
	// Roster загружает данные о конференции и членстве в ней, необходимые для авторизации
	// текущего снимка состояния комнаты.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Результирующий статус.
	//   - result2: Собранные данные об участниках.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Roster(context.Context, string) (conferences.Status, []conferences.Participant, error)
}

// Store — хранилище определяет заменяемый контракт зависимости для физических сессий и доставки событий
// в реальном времени.
type Store interface {
	// Register регистрирует физическое подключение и время его ограниченного присутствия.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Постоянное или физическое состояние сессии, определяемое ее типом.
	//   - arg3: Срок действия, назначенный для временного значения или записи о
	//   владельце.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Register(context.Context, domain.Session, time.Duration) error
	// Unregister закрывает физическую сессию и обновляет данные о распределенном присутствии
	// участника.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Unregister(context.Context, string) error
	// Функция Touch обновляет данные о присутствии, используя предоставленное время
	// подтвержденной активности на транспортном уровне.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Временная метка подтвержденного на транспортном уровне ответа Pong или
	//   другого подтверждения активности.
	//   - arg4: Срок действия, назначенный для временного значения или записи о
	//   владельце.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Touch(context.Context, string, time.Time, time.Duration) error
	// Get загружает запрошенную физическую сессию в авторизованную область видимости вызывающей
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
	Get(context.Context, string) (domain.Session, error)
	// Missing возвращает соединения, маршрут которых стал недоступен. Ошибка не должна
	// интерпретироваться как отсутствие данных: в этом случае сеансы SQL остаются нетронутыми.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификаторы ресурсов для обработки в рамках ограниченного пакета.
	//
	// @returns:
	//   - result1: Собранные строковые значения.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Missing(context.Context, []string) ([]string, error)
	// Active возвращает физические сессии, время присутствия которых еще не истекло.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, активные сеансы которой должны быть загружены.
	//
	// @returns:
	//   - result1: Собранные значения сеанса.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Active(context.Context, string) ([]domain.Session, error)
	// Prune удаляет физические сессии с истекшим временем присутствия и возвращает состояние,
	// необходимое для сверки данных о присутствии.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Prune(context.Context) error
	// Publish — при публикации сохраненные изменения физического сеанса передаются через настроенный
	// транспорт событий или команд.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Публикация и подписка с ограничением области видимости.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Publish(context.Context, domain.Bus) error
	// Subscribe открывает подписку на события с областью видимости, время жизни которой
	// ограничено предоставленным контекстом.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: Полученная подписка.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Subscribe(context.Context) (domain.Subscription, error)
}

// Socket определяет заменяемый контракт зависимости для физических сеансов и доставки событий в
// режиме реального времени.
type Socket interface {
	// Offer помещает событие в ограниченную очередь сокета в соответствии с его приоритетом и
	// сообщает, было ли оно принято.
	//
	// @Parameters:
	//   - arg1: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	Offer(domain.Envelope) bool
	// Stop останавливает работу активного физического сеанса и освобождает принадлежащие ему
	// ресурсы.
	//
	// @Parameters:
	//   - arg1: Ограниченный код причины, отправляемый при закрытии сокета.
	Stop(string)
}

// DisconnectObserver определяет заменяемый контракт зависимости для физических сеансов и
// доставки событий в режиме реального времени.
type DisconnectObserver interface {
	// Disconnected обрабатывает закрытие физического соединения и освобождает только те
	// ресурсы, которые были связаны с этим соединением.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Постоянное или физическое состояние сессии, определяемое ее типом.
	Disconnected(context.Context, domain.Session)
}

// DisconnectObservers объединяет наблюдатели с ограниченной очисткой, сохраняя при этом
// независимые жизненные циклы подсистем.
type DisconnectObservers []DisconnectObserver

// Disconnected обрабатывает закрытие физического соединения и освобождает только те ресурсы,
// которые были связаны с этим соединением.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
func (observers DisconnectObservers) Disconnected(ctx context.Context, session domain.Session) {
	for _, observer := range observers {
		if observer != nil {
			observer.Disconnected(ctx, session)
		}
	}
}

// localSocket привязывает локальный транспорт к физическому сеансу и состоянию его жизненного
// цикла.
type localSocket struct {
	session domain.Session
	socket  Socket
	ready   bool
}

// Hub координирует работу локальных сокетов, распределенное присутствие и доставку событий с
// фильтрацией доступа с независимыми приоритетами.
type Hub struct {
	repo               Repository
	store              Store
	ttl                time.Duration
	logger             *slog.Logger
	ctx                context.Context
	cancel             context.CancelFunc
	sub                domain.Subscription
	mu                 sync.Mutex
	closing            bool
	unavailable        bool
	generation         uint64
	local              map[string]*localSocket
	sockets            sync.WaitGroup
	workers            sync.WaitGroup
	disconnectObserver DisconnectObserver
	lowEvents          chan domain.Bus
	shutdownOnce       sync.Once // Завершение работы инициируется только один раз, даже если вызывающая сторона повторяет
	// запрос.
	shutdownDone chan struct{}
}

// NewHub создает хаб с зависимостями и ограничениями, необходимыми для физических сессий и
// доставки событий в режиме реального времени.
//
// @Parameters:
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - store: Распределенное хранилище физических сессий и событий.
//   - ttl: Срок действия, назначенный для временного значения или записи о владельце.
//   - logger: Ограниченный диагностический логгер.
//
// @returns:
//   - result1: Созданный или загруженный хаб.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewHub(repo Repository, store Store, ttl time.Duration, logger *slog.Logger) (*Hub, error) {
	ctx, cancel := context.WithCancel(context.Background())
	if logger == nil {
		logger = slog.Default()
	}
	h := &Hub{repo: repo, store: store, ttl: ttl, logger: logger, ctx: ctx, cancel: cancel, local: map[string]*localSocket{}, lowEvents: make(chan domain.Bus, 128)}
	op, c := context.WithTimeout(ctx, 5*time.Second)
	defer c()
	sub, err := store.Subscribe(op)
	if err != nil {
		cancel()
		return nil, err
	}
	h.sub = sub
	h.generation = 1
	h.workers.Add(3)
	go h.receive()
	go h.receiveLowPriority()
	go h.janitor()
	return h, nil
}

// Authorize проверяет текущие идентификационные данные и сохраненный статус доступа перед
// физическими сеансами и доставкой событий в режиме реального времени.
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
func (h *Hub) Authorize(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	return h.repo.Authorize(ctx, conferenceID, userID)
}

// AuthorizeClient дополнительно ограничивает область видимости частных вызовов для
// инициирующего/принимающего клиента.  Обычные репозитории конференций сохраняют существующий
// контракт.
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
func (h *Hub) AuthorizeClient(ctx context.Context, conferenceID, userID, clientID string) (conferences.Participant, error) {
	if repo, ok := h.repo.(interface {
		// AuthorizeClient проверяет доступ к конференции для указанной учётной записи и
		// идентификационных данных физического клиента.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
		//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
		//   доступа.
		//   - arg4: Идентификатор физического браузера или сокетного клиента.
		//
		// @returns:
		//   - result1: Полученный участник.
		//   - result2: nil — в случае успеха; в противном случае — ошибка валидации,
		//   доступа, передачи данных или сохранения.
		AuthorizeClient(context.Context, string, string, string) (conferences.Participant, error)
	}); ok {
		return repo.AuthorizeClient(ctx, conferenceID, userID, clientID)
	}
	return h.repo.Authorize(ctx, conferenceID, userID)
}

// Check — проверка включает в себя активную подписку в рамках готовности API; сам по себе успешный пинг
// Redis не означает, что этот хаб может принимать события конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closing || h.unavailable {
		return apperrors.ErrUnavailable
	}
	return nil
}

// SetDisconnectObserver обновляет наблюдателя за отключением для физических сессий и доставки
// событий в режиме реального времени.
//
// @Parameters:
//   - observer: Наблюдатель получает уведомление после изменения жизненного цикла.
func (h *Hub) SetDisconnectObserver(observer DisconnectObserver) {
	h.mu.Lock()
	h.disconnectObserver = observer
	h.mu.Unlock()
}

// ValidateSession привязывает медиаоперацию к текущему авторизованному физическому сеансу
// WebSocket.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) ValidateSession(ctx context.Context, session domain.Session) error {
	p, err := h.AuthorizeClient(ctx, session.ConferenceID, session.UserID, session.ClientID)
	if err != nil {
		return err
	}
	live, err := h.store.Get(ctx, session.ConnectionID)
	if err != nil {
		return err
	}
	if p.ID != session.ParticipantID || live.ID != session.ID || live.ConferenceID != session.ConferenceID || live.ParticipantID != session.ParticipantID || live.UserID != session.UserID || live.ConnectionID != session.ConnectionID || live.Status != "connected" {
		return apperrors.ErrForbidden
	}
	return nil
}

// Prepare — подготавливает авторизованное состояние физической сессии перед началом согласования
// медиаданных.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Результирующий сеанс.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) Prepare(ctx context.Context, conferenceID, userID string) (domain.Session, error) {
	return h.PrepareClient(ctx, conferenceID, userID, "")
}

// PrepareClient авторизует и подготавливает физическую клиентскую сессию перед регистрацией
// сокета.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - clientID: Идентификатор физического браузера или сокетного клиента.
//
// @returns:
//   - result1: Результирующий сеанс.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) PrepareClient(ctx context.Context, conferenceID, userID, clientID string) (domain.Session, error) {
	h.mu.Lock()
	closing := h.closing
	unavailable := h.unavailable
	h.mu.Unlock()
	if closing {
		return domain.Session{}, apperrors.ErrConflict
	}
	if unavailable {
		return domain.Session{}, apperrors.ErrUnavailable
	}
	p, err := h.AuthorizeClient(ctx, conferenceID, userID, clientID)
	if err != nil {
		return domain.Session{}, err
	}
	now := time.Now().UTC()
	s := domain.Session{ID: uuid.NewString(), ConferenceID: conferenceID, ParticipantID: p.ID, UserID: userID, ClientID: clientID, ConnectionID: uuid.NewString(), Status: "connected", ConnectedAt: now, LastSeenAt: now}
	return s, h.repo.Open(ctx, s)
}

// Abort закрывает физическую сессию, регистрация которой не удалась, и удаляет информацию о ее
// распределенном присутствии.
//
// @Parameters:
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
func (h *Hub) Abort(session domain.Session) {
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = h.repo.Close(ctx, session.ConnectionID, session.LastSeenAt)
}

// Register регистрирует физическое подключение и время его ограниченного присутствия.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//   - socket: Физический транспортный сокет зарегистрирован с проверенной сессией.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) Register(ctx context.Context, session domain.Session, socket Socket) error {
	h.mu.Lock()
	if h.closing || h.unavailable {
		err := apperrors.ErrConflict
		if h.unavailable && !h.closing {
			err = apperrors.ErrUnavailable
		}
		h.mu.Unlock()
		h.Abort(session)
		return err
	}
	h.sockets.Add(1)
	h.local[session.ConnectionID] = &localSocket{session: session, socket: socket}
	h.mu.Unlock()
	if err := h.store.Register(ctx, session, h.ttl); err != nil {
		h.Unregister(session)
		return err
	}
	// Обработка завершения конференции или выхода участника из нее происходит одновременно с
	// обновлением сокета и регистрацией в Redis.
	if _, err := h.AuthorizeClient(ctx, session.ConferenceID, session.UserID, session.ClientID); err != nil {
		h.Unregister(session)
		return err
	}
	state, err := h.state(ctx, session)
	if err == nil && !stateAllows(state, session.ParticipantID) {
		err = apperrors.ErrForbidden
	}
	if err != nil || !socket.Offer(domain.Event("conference.state", session.ConferenceID, stateFor(state, session))) {
		h.Unregister(session)
		if err != nil {
			return err
		}
		return apperrors.ErrConflict
	}
	h.mu.Lock()
	if entry := h.local[session.ConnectionID]; entry != nil {
		entry.ready = true
	}
	h.mu.Unlock()
	h.log(session, "connected")
	return nil
}

// Unregister закрывает физическую сессию и обновляет данные о распределенном присутствии
// участника.
//
// @Parameters:
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
func (h *Hub) Unregister(session domain.Session) {
	h.mu.Lock()
	_, exists := h.local[session.ConnectionID]
	delete(h.local, session.ConnectionID)
	observer := h.disconnectObserver
	h.mu.Unlock()
	if !exists {
		return
	}
	defer h.sockets.Done()
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	seen := session.LastSeenAt
	if live, err := h.store.Get(ctx, session.ConnectionID); err == nil {
		seen = live.LastSeenAt
	}
	if err := h.store.Unregister(ctx, session.ConnectionID); err != nil {
		h.log(session, "redis_cleanup_failed")
	}
	if err := h.repo.Close(ctx, session.ConnectionID, seen); err != nil {
		h.log(session, "history_cleanup_failed")
	}
	// Наблюдатели считают, что сессия уже закрыта. Две вкладки, закрывающиеся одновременно, не
	// могут ошибочно принять друг друга за последнюю активную сессию.
	if observer != nil {
		observer.Disconnected(ctx, session)
	}
	h.log(session, "disconnected")
}

// Функция Touch обновляет данные о присутствии, используя предоставленное время подтвержденной
// активности на транспортном уровне.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) Touch(ctx context.Context, session domain.Session) error {
	if _, err := h.AuthorizeClient(ctx, session.ConferenceID, session.UserID, session.ClientID); err != nil {
		return err
	}
	// Membership Проверки не должны увеличивать пятисекундный интервал проверки доступности.
	// LastSeenAt приходит только после получения подтвержденного транспортного подтверждения
	// pong.
	remaining := h.ttl - time.Since(session.LastSeenAt)
	if session.LastSeenAt.IsZero() || remaining <= 0 || remaining > h.ttl {
		return apperrors.ErrNotFound
	}
	return h.store.Touch(ctx, session.ConnectionID, session.LastSeenAt, h.ttl)
}

// GetActiveSessions возвращает текущие физические сеансы, используемые для авторизации
// медиакоманд.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Собранные значения сеанса.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) GetActiveSessions(ctx context.Context, conferenceID string) ([]domain.Session, error) {
	return h.store.Active(ctx, conferenceID)
}

// Функция Broadcast публикует доверенное событие только для тех участников конференции или
// беседы, которые в данный момент имеют к ней доступ.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - event: Доверенное событие для публикации или обработки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) Broadcast(ctx context.Context, event domain.Envelope) error {
	return h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: event.ConferenceID, Event: &event})
}

// SendToParticipant публикует доверенное событие для физических сеансов выбранного
// авторизованного участника.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - participantID: Идентификатор участника конференции.
//   - event: Доверенное событие для публикации или обработки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) SendToParticipant(ctx context.Context, conferenceID, participantID string, event domain.Envelope) error {
	return h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: conferenceID, ParticipantID: participantID, Event: &event})
}

// SendToConnection публикует доверенное событие в одном авторизованном физическом сокетном
// соединении.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//   - targetID: Идентификатор целевой учетной записи или ресурса.
//   - event: Доверенное событие для публикации или обработки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) SendToConnection(ctx context.Context, session domain.Session, targetID string, event domain.Envelope) error {
	if _, err := h.AuthorizeClient(ctx, session.ConferenceID, session.UserID, session.ClientID); err != nil {
		return err
	}
	sender, err := h.store.Get(ctx, session.ConnectionID)
	if err != nil || sender.ConferenceID != session.ConferenceID || sender.UserID != session.UserID {
		return apperrors.ErrForbidden
	}
	target, err := h.store.Get(ctx, targetID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return apperrors.New(apperrors.ErrNotFound, "target connection is offline")
	}
	if err != nil {
		return err
	}
	if target.ConferenceID != session.ConferenceID {
		return apperrors.ErrForbidden
	}
	if _, err := h.AuthorizeClient(ctx, target.ConferenceID, target.UserID, target.ClientID); err != nil {
		return apperrors.ErrForbidden
	}
	err = h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: session.ConferenceID, ConnectionID: targetID, Event: &event})
	if err == nil {
		h.log(session, event.Type)
	}
	return err
}

// ConferenceChanged уведомляет подключенные сеансы или наблюдателей об изменении состояния
// конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
func (h *Hub) ConferenceChanged(ctx context.Context, conferenceID string) {
	ctx, c := context.WithTimeout(ctx, 3*time.Second)
	defer c()
	if err := h.store.Publish(ctx, domain.Bus{Kind: "changed", ConferenceID: conferenceID}); err != nil {
		h.logger.Warn("realtime event", "conference_id", conferenceID, "event_type", "mutation_publish_failed")
	}
}

// state объединяет физические соединения по сохраненным данным о членстве участников.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//
// @returns:
//   - result1: Результирующее состояние.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) state(ctx context.Context, session domain.Session) (domain.State, error) {
	status, roster, err := h.repo.Roster(ctx, session.ConferenceID)
	if err != nil {
		return domain.State{}, err
	}
	activeSessions, err := h.store.Active(ctx, session.ConferenceID)
	if err != nil {
		return domain.State{}, err
	}
	connectionsByParticipant := map[string][]string{}
	for _, activeSession := range activeSessions {
		connectionsByParticipant[activeSession.ParticipantID] = append(connectionsByParticipant[activeSession.ParticipantID], activeSession.ConnectionID)
	}
	state := domain.State{ConnectionID: session.ConnectionID, ParticipantID: session.ParticipantID, Status: status, Participants: make([]domain.Presence, 0, len(roster))}
	for _, participant := range roster {
		connectionIDs := connectionsByParticipant[participant.ID]
		if !participant.CanParticipate() || connectionIDs == nil {
			connectionIDs = []string{}
		}
		sort.Strings(connectionIDs)
		state.Participants = append(state.Participants, domain.Presence{ParticipantView: participant.View(), Online: len(connectionIDs) > 0, Connections: len(connectionIDs), ConnectionIDs: connectionIDs})
	}
	return state, nil
}

// stateFor скрывает список ожидающих от тех, кто не является модератором, и устанавливает
// идентификатор получателя.
//
// @Parameters:
//   - state: Текущее состояние или обновлённое валидированное состояние.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//
// @returns:
//   - result1: Результирующее состояние.
func stateFor(state domain.State, session domain.Session) domain.State {
	state.ConnectionID, state.ParticipantID = session.ConnectionID, session.ParticipantID
	isModerator := false
	for _, participant := range state.Participants {
		if participant.ID == session.ParticipantID {
			isModerator = participant.Role == conferences.Owner || participant.Role == conferences.CoHost
			break
		}
	}
	if isModerator {
		return state
	}
	visible := make([]domain.Presence, 0, len(state.Participants))
	for _, participant := range state.Participants {
		if participant.CanReadHistory() {
			visible = append(visible, participant)
		}
	}
	state.Participants = visible
	return state
}

// stateAllows проверяет, позволяет ли текущий снимок состояния конференции физическому сеансу
// по-прежнему участвовать в конференции.
//
// @Parameters:
//   - state: Текущее состояние или обновлённое валидированное состояние.
//   - participantID: Идентификатор участника конференции.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func stateAllows(state domain.State, participantID string) bool {
	if state.Status != conferences.Created && state.Status != conferences.Active {
		return false
	}
	for _, p := range state.Participants {
		if p.ID == participantID {
			return p.CanParticipate() && !p.ChatLeft
		}
	}
	return false
}

// receive считывает входящие события физического сеанса и перенаправляет их по соответствующему
// пути доставки с ограничением.
func (h *Hub) receive() {
	defer h.workers.Done()
	sub, generation := h.sub, h.generation // Устанавливается до запуска этого воркера.
	for {
		bus, err := sub.Receive(h.ctx)
		if err != nil {
			if h.ctx.Err() != nil {
				return
			}
			h.logger.Warn("realtime event", "event_type", "broker_unavailable")
			h.breakBroker(generation)
			_ = sub.Close()
			var ok bool
			sub, generation, ok = h.recoverSubscription()
			if !ok {
				return
			}
			continue
		}
		if bus.Kind == "event" && bus.Event != nil && domain.LowPriorityEvent(bus.Event.Type) {
			select {
			case h.lowEvents <- bus:
			default:
			} // Сохраненная история восстанавливает события, потерянные из-за переполнения
			// очереди.
		} else {
			h.deliver(bus)
		}
	}
}

// breakBroker делает недействительными только те генерации, работа которых была прервана.
// Медленный результат Prune для старого соединения не должен приводить к закрытию исправного
// соединения.  Уведомления о закрытии сети и сокета обрабатываются вне блокировки реестра.
//
// @Parameters:
//   - generation: Идентификатор генерации контента, используемый для отклонения устаревших
//     результатов.
func (h *Hub) breakBroker(generation uint64) {
	h.mu.Lock()
	if h.closing || h.unavailable || h.generation != generation {
		h.mu.Unlock()
		return
	}
	h.unavailable = true
	sub := h.sub
	entries := make([]*localSocket, 0, len(h.local))
	for _, entry := range h.local {
		entries = append(entries, entry)
	}
	h.mu.Unlock()
	for _, entry := range entries {
		entry.socket.Stop("broker_unavailable")
	}
	_ = sub.Close() // Прерывает прием, чтобы его единственный обработчик начал восстановление.
}

// recoverSubscription выполняется в существующем обработчике приема.  Каждая попытка имеет свой
// тайм-аут, и следующая подписка подтверждается до открытия доступа.
//
// @returns:
//   - result1: Полученная подписка.
//   - result2: Возвращает количество, размер или числовое значение.
//   - result3: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (h *Hub) recoverSubscription() (domain.Subscription, uint64, bool) {
	backoff := 100 * time.Millisecond
	for {
		timer := time.NewTimer(backoff)
		select {
		case <-h.ctx.Done():
			timer.Stop()
			return nil, 0, false
		case <-timer.C:
		}
		timer.Stop()
		ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
		sub, err := h.store.Subscribe(ctx)
		cancel()
		if err != nil {
			backoff = min(backoff*2, 2*time.Second)
			continue
		}
		h.mu.Lock()
		if h.closing || h.ctx.Err() != nil {
			h.mu.Unlock()
			_ = sub.Close()
			return nil, 0, false
		}
		h.sub = sub
		h.generation++
		generation := h.generation
		h.unavailable = false
		h.mu.Unlock()
		h.logger.Info("realtime event", "event_type", "broker_recovered")
		return sub, generation, true
	}
}

// receiveLowPriority доставляет сообщения чата и реакции через независимую низкоприоритетную
// очередь с ограниченным размером.
func (h *Hub) receiveLowPriority() {
	defer h.workers.Done()
	for {
		select {
		case <-h.ctx.Done():
			return
		case bus := <-h.lowEvents:
			h.deliver(bus)
		}
	}
}

// entries копирует записи локальных сокетов, удерживая блокировку реестра.
//
// @Parameters:
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Собранные значения локальных сокетов.
func (h *Hub) entries(conferenceID string) []*localSocket {
	h.mu.Lock()
	defer h.mu.Unlock()
	rows := make([]*localSocket, 0, len(h.local))
	for _, entry := range h.local {
		if entry.ready && (conferenceID == "" || entry.session.ConferenceID == conferenceID) {
			rows = append(rows, entry)
		}
	}
	return rows
}

// deliver проверяет текущие права доступа получателя непосредственно перед доставкой
// сохраненного события или команды.
//
// @Parameters:
//   - bus: Публикация и подписка с ограничением области видимости.
func (h *Hub) deliver(bus domain.Bus) {
	ctx, c := context.WithTimeout(h.ctx, 5*time.Second)
	defer c()
	if bus.Kind == "expired" && bus.Session != nil {
		_ = h.repo.Close(ctx, bus.Session.ConnectionID, bus.Session.LastSeenAt)
		h.mu.Lock()
		observer := h.disconnectObserver
		h.mu.Unlock()
		if observer != nil {
			observer.Disconnected(ctx, *bus.Session)
		}
	}
	entries := h.entries(bus.ConferenceID)
	if len(entries) == 0 {
		return
	}
	if bus.Kind == "event" && bus.Event != nil {
		// Перепроверяет получателей на соответствие текущему сохраненному членству, а не кэшу
		// сокетов.  Удаление или завершение работы может быть зафиксировано до того, как будет
		// доставлено событие изменения состояния.
		kind := bus.Event.Type
		restricted := strings.HasPrefix(kind, "chat.") || strings.HasPrefix(kind, "recording.") || strings.HasPrefix(kind, "reaction.") || strings.HasPrefix(kind, "participant.")
		allowed := map[string]bool{}
		if restricted {
			status, roster, err := h.repo.Roster(ctx, bus.ConferenceID)
			if err != nil {
				return
			}
			for _, p := range roster {
				ok := p.CanParticipate() && !p.ChatLeft
				if status == conferences.Finished || status == conferences.Cancelled {
					ok = false
				}
				if kind == "participant.waiting" || kind == "participant.rejected" {
					ok = ok && p.CanAdmit()
				}
				allowed[p.ID] = ok
			}
		}
		for _, entry := range entries {
			s := entry.session
			if restricted && !allowed[s.ParticipantID] {
				continue
			}
			if (bus.ConnectionID == "" || bus.ConnectionID == s.ConnectionID) && (bus.ParticipantID == "" || bus.ParticipantID == s.ParticipantID) {
				if !entry.socket.Offer(*bus.Event) {
					entry.socket.Stop("slow_client")
				}
			}
		}
		return
	}
	if len(entries) == 0 {
		return
	}
	// Используйте по одному снимку состояния конференции для каждого события, а не обращайтесь
	// к базе данных и Redis для каждого сокета.
	state, err := h.state(ctx, entries[0].session)
	if err != nil {
		for _, e := range entries {
			e.socket.Stop("state_unavailable")
		}
		return
	}
	joined := map[string]bool{}
	for _, p := range state.Participants {
		joined[p.ID] = p.CanParticipate() && !p.ChatLeft
	}
	for _, entry := range entries {
		if state.Status == conferences.Finished || state.Status == conferences.Cancelled || !joined[entry.session.ParticipantID] {
			entry.socket.Stop("membership_closed")
			continue
		}
		kind := "conference.state"
		if bus.Kind == "connected" {
			kind = "participant.connected"
		} else if bus.Kind == "disconnected" || bus.Kind == "expired" {
			kind = "participant.disconnected"
		}
		if !entry.socket.Offer(domain.Event(kind, bus.ConferenceID, stateFor(state, entry.session))) {
			entry.socket.Stop("slow_client")
		}
	}
}

// janitor — уборщик периодически удаляет данные о присутствии, срок действия которых истёк, и закрывает
// сокеты, авторизация которых была отозвана.
func (h *Hub) janitor() {
	defer h.workers.Done()
	interval := h.ttl / 4
	if interval > 250*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	cursor := ""
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			ctx, c := context.WithTimeout(h.ctx, 5*time.Second)
			h.mu.Lock()
			generation := h.generation
			h.mu.Unlock()
			if err := h.store.Prune(ctx); err != nil {
				h.breakBroker(generation)
			}
			// Также восстанавливаются сбои, произошедшие между открытием сеанса SQL и
			// регистрацией данных о присутствии в Redis.
			rows, err := h.repo.Stale(ctx, time.Now().Add(-2*h.ttl), cursor)
			if err == nil {
				h.closeMissingSessions(ctx, rows)
				if len(rows) < 500 {
					cursor = ""
				} else {
					cursor = rows[len(rows)-1].ID
				}
			}
			c()
		}
	}
}

// closeMissingSessions проверяет одну ограниченную страницу SQL в пакете запросов Redis.
// Действующие маршруты не требуют декодирования JSON; SQL изменяется только после успешной
// проверки данных о присутствии, что позволяет избежать сбоев, характерных для предыдущего
// цикла GET.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - rows: Для согласования используется сохранённый или локальный снимок состояния.
func (h *Hub) closeMissingSessions(ctx context.Context, rows []domain.Session) {
	if len(rows) == 0 {
		return
	}
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ConnectionID
	}
	missing, err := h.store.Missing(ctx, ids)
	if err != nil || len(missing) == 0 {
		return
	}
	absent := make(map[string]struct{}, len(missing))
	for _, id := range missing {
		absent[id] = struct{}{}
	}
	for _, s := range rows {
		if _, ok := absent[s.ConnectionID]; ok {
			_ = h.repo.Close(ctx, s.ConnectionID, s.LastSeenAt)
		}
	}
}

// stopSockets закрывает выбранные физические сокеты после изменения их состояния доступа или
// состояния конференции.
//
// @Parameters:
//   - reason: Ограниченный жизненный цикл или причина сбоя.
func (h *Hub) stopSockets(reason string) {
	h.mu.Lock()
	h.closing = true
	entries := make([]*localSocket, 0, len(h.local))
	for _, e := range h.local {
		entries = append(entries, e)
	}
	h.mu.Unlock()
	for _, e := range entries {
		e.socket.Stop(reason)
	}
}

// Shutdown останавливает выполнение новых задач и ожидает завершения закрытия ресурсов,
// принадлежащих компоненту.
func (h *Hub) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = h.ShutdownContext(ctx)
}

// ShutdownContext перестаёт принимать новые сокеты и ожидает завершения подписок и задач, пока
// не истечёт время ожидания завершения работы.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (h *Hub) ShutdownContext(ctx context.Context) error {
	h.shutdownOnce.Do(func() {
		h.shutdownDone = make(chan struct{})
		go func() {
			defer close(h.shutdownDone)
			h.stopSockets("server_shutdown")
			h.cancel()
			h.mu.Lock()
			sub := h.sub
			h.mu.Unlock()
			_ = sub.Close()
			h.sockets.Wait()
			h.workers.Wait()
		}()
	})
	select {
	case <-h.shutdownDone:
		return nil
	case <-ctx.Done():
		h.cancel()
		return ctx.Err()
	}
}

// LocalCount возвращает количество локально зарегистрированных сокетов, находящихся под
// блокировкой реестра.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
func (h *Hub) LocalCount() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.local) }

// log — записывает в журнал ограниченную диагностику компонента без использования приватных
// сигнальных данных.
//
// @Parameters:
//   - s: Состояние сеанса, обработанное в ходе этой операции.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
func (h *Hub) log(s domain.Session, kind string) {
	h.logger.Info("realtime event", "conference_id", s.ConferenceID, "participant_id", s.ParticipantID, "user_id", s.UserID, "connection_id", s.ConnectionID, "event_type", kind)
}

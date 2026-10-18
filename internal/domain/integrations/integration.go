package integrations

import (
	"context"
	"time"
)

// Preferences — настройки позволяют выбрать категории персональных уведомлений и внешние каналы; отправка
// уведомлений по электронной почте и push-уведомлений по умолчанию отключена.
type Preferences struct {
	UserID     string    `json:"-" gorm:"type:uuid;primaryKey"`
	Invitation bool      `json:"invitation"`
	Reminder   bool      `json:"reminder"`
	Recording  bool      `json:"recording"`
	Summary    bool      `json:"summary"`
	Email      bool      `json:"email"`
	Push       bool      `json:"push"`
	UpdatedAt  time.Time `json:"-"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Preferences) TableName() string { return "notification_preferences" }

// DefaultPreferences возвращает начальные настройки уведомлений для учетной записи.
//
// @Parameters:
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Результирующие настройки.
func DefaultPreferences(userID string) Preferences {
	return Preferences{UserID: userID, Invitation: true, Reminder: true, Recording: true, Summary: true}
}

// Allows сообщает, разрешает ли текущая политика выполнение запрошенной операции или
// использование источника.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (p Preferences) Allows(kind string) bool {
	switch kind {
	case "conference.invited", "conference.rescheduled", "conference.cancelled":
		return p.Invitation
	case "conference.soon":
		return p.Reminder
	case "recording.ready":
		return p.Recording
	case "transcript.ready", "summary.ready":
		return p.Summary
	case "admission.decided", "processing.failed":
		return true
	}
	return false
}

// Device сохраняет данные о регистрации устройства; зашифрованный токен и отпечаток устройства
// никогда не включаются в ответы API.
type Device struct {
	ID               string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID           string     `json:"-"`
	Platform         string     `json:"platform"`
	Provider         string     `json:"provider"`
	TokenCiphertext  string     `json:"-"`
	TokenFingerprint string     `json:"-"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	RevokedAt        *time.Time `json:"revokedAt,omitempty"`
	DisabledAt       *time.Time `json:"disabledAt,omitempty"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (Device) TableName() string { return "push_devices" }

// DeviceRequest принимает токен от доверенного приложения на устройстве, а не от произвольного
// провайдера URLs.
type DeviceRequest struct {
	Platform string `json:"platform"`
	Token    string `json:"token"`
}

// CalendarConnection привязывает внешний календарь к своему владельцу; секреты OAuth доступны
// только на сервере.
type CalendarConnection struct {
	ID                string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID            string     `json:"-"`
	Provider          string     `json:"provider"`
	CalendarID        string     `json:"calendarId"`
	Status            string     `json:"status"`
	AccessCiphertext  string     `json:"-"`
	RefreshCiphertext string     `json:"-"`
	Scopes            string     `json:"-"`
	ExpiresAt         *time.Time `json:"expiresAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (CalendarConnection) TableName() string { return "calendar_connections" }

// CalendarMapping сохраняет по одному внешнему событию для каждой пары «конференция —
// подключение».
type CalendarMapping struct {
	ID                 string     `json:"-" gorm:"type:uuid;primaryKey"`
	ConferenceID       string     `json:"conferenceId"`
	ConnectionID       string     `json:"-"`
	Provider           string     `json:"provider"`
	ExternalCalendarID string     `json:"externalCalendarId"`
	ExternalEventID    string     `json:"externalEventId"`
	SyncStatus         string     `json:"syncStatus"`
	SourceVersion      int64      `json:"-"`
	LastSyncedAt       *time.Time `json:"lastSyncedAt,omitempty"`
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (CalendarMapping) TableName() string { return "calendar_event_mappings" }

// OAuthState сохраняет кратковременное одноразовое состояние и зашифрованный верификатор PKCE.
type OAuthState struct {
	ID                 string
	UserID             string
	Provider           string
	StateHash          string
	VerifierCiphertext string
	ExpiresAt          time.Time
	UsedAt             *time.Time
}

// TableName возвращает явное имя таблицы базы данных, чтобы GORM не преобразовывала его в
// другую форму множественного числа.
//
// @returns:
//   - result1: Точное имя таблицы базы данных, используемое GORM.
func (OAuthState) TableName() string { return "calendar_oauth_states" }

// EmailMessage содержит минимальные данные для доставки и идентификатор, который не меняется
// при повторных попытках.
type EmailMessage struct {
	IdempotencyKey string `json:"idempotencyKey"`
	To             string `json:"to"`
	Subject        string `json:"subject"`
	HTML           string `json:"html"`
	Text           string `json:"text"`
}

// PushMessage содержит ссылку на интерфейс и безопасный текст события, но не стенограммы
// конференций.
type PushMessage struct {
	IdempotencyKey string `json:"idempotencyKey"`
	Token          string `json:"token"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	URL            string `json:"url"`
}

// CalendarEvent представляет собой внешнее событие без импорта специфичных для конкретного
// провайдера токенов SDK.
type CalendarEvent struct {
	IdempotencyKey string    `json:"idempotencyKey"`
	ID             string    `json:"id,omitempty"`
	CalendarID     string    `json:"calendarId"`
	Title          string    `json:"title"`
	JoinURL        string    `json:"joinUrl"`
	StartsAt       time.Time `json:"startsAt"`
	EndsAt         time.Time `json:"endsAt"`
	Attendees      []string  `json:"attendees,omitempty"`
	SourceVersion  int64     `json:"sourceVersion"`
}

// CalendarCredentials использует токены OAuth только внутри сервера, а не в общедоступных JSON
// или логах.
type CalendarCredentials struct {
	AccessToken  string     `json:"-"`
	RefreshToken string     `json:"-"`
	ExpiresAt    *time.Time `json:"-"`
	Scopes       string     `json:"-"`
}

// EmailProvider изолирует доставку с помощью стабильного ключа сообщения. Шлюз HTTP выполняет
// дедупликацию; SMTP сохраняет Message-ID, но может повторно отправить сообщение после
// получения подтверждения о его доставке.
type EmailProvider interface {
	// Send доставляет ограниченное электронное письмо со стабильным идентификатором сообщения;
	// настроенное транспортное средство определяет возможность дедупликации при повторных
	// попытках доставки.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Сохраненное или отправленное сообщение в чате.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Send(context.Context, EmailMessage) error
}

// PushProvider изолирует доставку на платформе от специфичных для конкретного провайдера
// токенов SDKs.
type PushProvider interface {
	// Send доставляет минимальное уведомление на выбранное зарегистрированное устройство без
	// расшифровки текста и без использования секретов провайдера.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Сохраненное или отправленное сообщение в чате.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Send(context.Context, PushMessage) error
}

// CalendarProvider определяет создание, обновление, отмену и поиск событий; CreateEvent должен
// соответствовать ключу идемпотентности.
type CalendarProvider interface {
	// CreateEvent создает идемпотентное календарное событие с помощью настроенного провайдера.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Значение учетных данных календаря, требуемое контрактом.
	//   - arg3: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: Результирующее календарное событие.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CreateEvent(context.Context, CalendarCredentials, CalendarEvent) (CalendarEvent, error)
	// UpdateEvent обновляет сопоставленное внешнее календарное событие для текущего расписания
	// конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Значение учетных данных календаря, требуемое контрактом.
	//   - arg3: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: Результирующее календарное событие.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	UpdateEvent(context.Context, CalendarCredentials, CalendarEvent) (CalendarEvent, error)
	// CancelEvent отменяет внешнее календарное событие, связанное с сохранённым сопоставлением.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Значение учетных данных календаря, требуемое контрактом.
	//   - arg3: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CancelEvent(context.Context, CalendarCredentials, CalendarEvent) error
	// GetEvent считывает сопоставленное внешнее календарное событие для сверки.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Значение учетных данных календаря, требуемое контрактом.
	//   - arg3: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: Результирующее календарное событие.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GetEvent(context.Context, CalendarCredentials, CalendarEvent) (CalendarEvent, error)
}

// OAuthProvider выполняет авторизацию, обновление и отзыв доступа на стороне сервера с
// использованием ограниченных областей, настраиваемых оператором.
type OAuthProvider interface {
	// AuthorizeURL формирует перенаправление авторизации провайдера с одноразовым состоянием и
	// запросом S256 PKCE, не раскрывая клиентский секрет.
	//
	// @Parameters:
	//   - arg1: Текущее состояние или обновлённое валидированное состояние.
	//   - arg2: Запрос S256 PKCE отправляется провайдеру авторизации.
	//
	// @returns:
	//   - result1: Авторизация провайдера URL без клиентского секрета.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	AuthorizeURL(string, string) (string, error)
	// Exchange обменивает одноразовый код авторизации и верификатор PKCE на учетные данные
	// провайдера на сервере.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Ограниченный публичный статус или код ошибки.
	//   - arg3: Верификатор токена или приватный верификатор PKCE в зависимости от его
	//   типа.
	//
	// @returns:
	//   - result1: Результирующие учетные данные календаря.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Exchange(context.Context, string, string) (CalendarCredentials, error)
	// Refresh обновляет краткосрочные учетные данные для доступа, используя текущий провайдер
	// или сеанс обновления учетной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Недоверенное сериализованное значение для декодирования или проверки.
	//
	// @returns:
	//   - result1: Результирующие учетные данные календаря.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Refresh(context.Context, string) (CalendarCredentials, error)
	// Revoke отзывает сохраненные права доступа к провайдеру без изменения локальных настроек
	// доставки учетной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Подписанные учетные данные или токен владения операцией для проверки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Revoke(context.Context, string) error
}

// Capabilities определяет фактически настроенные адаптеры; если адаптер не используется, это не
// считается успешной внешней доставкой.
type Capabilities struct {
	Email                   string `json:"email"`
	Push                    string `json:"push"`
	Calendar                string `json:"calendar"`
	CalendarOAuthConfigured bool   `json:"calendarOAuthConfigured"`
	MockConnectAllowed      bool   `json:"mockConnectAllowed"`
}

// Providers группирует заменяемые адаптеры и доступные методы подключения.
type Providers struct {
	Email        EmailProvider
	Push         PushProvider
	Calendar     CalendarProvider
	OAuth        OAuthProvider
	Capabilities Capabilities
}

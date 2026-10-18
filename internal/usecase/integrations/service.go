package integrations

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	d "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	n "github.com/janickiy/meet-space/internal/domain/notifications"
)

// Options содержит общедоступный фронтенд URL, общий тайм-аут провайдера и конечные смещения
// напоминаний UTC.
type Options struct {
	PublicURL          string
	ReminderOffsets    []time.Duration
	ProviderTimeout    time.Duration
	MockConnectAllowed bool
}

// ConferenceSnapshot содержит текущие поля, безопасные для внешней синхронизации, за
// исключением путей к медиафайлам.
type ConferenceSnapshot struct {
	ID, OwnerID, Title, InviteCode, Status string
	ScheduledAt                            *time.Time
	PlannedDurationMin                     *int
	IntegrationVersion                     int64
}

// Delivery содержит разрешённые факты об уведомлениях и адреса получателей, но не содержимое
// расшифровки.
type Delivery struct {
	Notification n.Notification
	Email        string
	Preferences  d.Preferences
	Allowed      bool
}

// Repository — репозиторий определяет надёжные операции интеграции; при асинхронной записи в рамках
// транзакции необходимо проверять аренду задания.
type Repository interface {
	// Preferences считывает сохраненные настройки доставки или переписки действующего аккаунта.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: Результирующие настройки.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Preferences(context.Context, string) (d.Preferences, error)
	// SavePreferences проверяет и сохраняет настройки доставки аккаунта, не инициируя доставку.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Значение предпочтений, требуемое по контракту.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SavePreferences(context.Context, d.Preferences) error
	// Devices возвращает зарегистрированные устройства доставки аккаунта, не раскрывая их
	// конфиденциальные учетные данные.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Возвращать ли только те устройства доставки, которые не были отозваны.
	//
	// @returns:
	//   - result1: Собранные значения устройств.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Devices(context.Context, string, bool) ([]d.Device, error)
	// SaveDevice сохраняет устройство доставки, принадлежащее аккаунту, с защищенным токеном
	// провайдера.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Устройство доставки, принадлежащее аккаунту, и его защищенные метаданные
	//   провайдера.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SaveDevice(context.Context, d.Device) error
	// RevokeDevice отзывает только выбранное устройство доставки действующего аккаунта.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RevokeDevice(context.Context, string, string) error
	// Connections возвращает подключения провайдера действующего аккаунта с исключенными из
	// общего доступа конфиденциальными учетными данными.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: Собранные значения подключений календаря.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Connections(context.Context, string) ([]d.CalendarConnection, error)
	// SaveConnection сохраняет подтвержденное подключение к провайдеру, принадлежащее учетной
	// записи, и зашифрованные учетные данные.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Контекст запроса и ответа Gin, включая аутентифицированные
	//   идентификационные данные.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SaveConnection(context.Context, d.CalendarConnection) error
	// RefreshConnection обновляет учетные данные провайдера только для ожидаемого состояния
	// подключения.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Контекст запроса и ответа Gin, включая аутентифицированные
	//   идентификационные данные.
	//   - arg3: Предыдущие учетные данные или идентификационные данные, использованные
	//   для отклонения устаревшего обновления.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RefreshConnection(context.Context, d.CalendarConnection, string) error
	// RevokeConnection отзывает подключение к провайдеру, принадлежащее учетной записи, и
	// делает недействительной его дальнейшую синхронизацию.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RevokeConnection(context.Context, string, string) error
	// SaveOAuthState сохраняет кратковременное одноразовое состояние OAuth, привязанное к
	// учетной записи и провайдеру.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Текущее состояние или обновлённое валидированное состояние.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SaveOAuthState(context.Context, d.OAuthState) error
	// TakeOAuthState атомарно использует соответствующее одноразовое состояние OAuth, чтобы
	// предотвратить повторный вызов функции обратного вызова.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Настроенный адаптер внешнего провайдера.
	//   - arg4: Ранее вычисленный дайджест, используемый для проверки подлинности или
	//   целостности.
	//
	// @returns:
	//   - result1: Результирующее состояние авторизации.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	TakeOAuthState(context.Context, string, string, string) (d.OAuthState, error)
	// CalendarMappings возвращает сохраненные сопоставления между конференциями и календарями
	// для действующей учетной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Собранные значения сопоставлений между календарями.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CalendarMappings(context.Context, string, string) ([]d.CalendarMapping, error)
	// Conference загружает сохраненные данные о конференциях, необходимые для интеграции и
	// синхронизации.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Результирующий снимок конференции.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Conference(context.Context, string) (ConferenceSnapshot, error)
	// MappingsForSync возвращает сопоставления между календарями, подходящие для текущего
	// формирования расписания.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Собранные значения сопоставлений между календарями.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	MappingsForSync(context.Context, string) ([]d.CalendarMapping, error)
	// SaveMapping сохраняет сопоставление событий провайдера только до тех пор, пока активна
	// аренда задания и версия расписания.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//   - arg3: Подтвержденный манифест резервного копирования или состояние медиафайлов
	//   в соответствии с их типом.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SaveMapping(context.Context, jobs.Job, d.CalendarMapping) error
	// FailCalendars записывает информацию о сбое при ограниченной синхронизации для затронутых
	// сопоставлений календарей.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//   - arg3: Ограниченный публичный статус или код ошибки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	FailCalendars(context.Context, jobs.Job, string) error
	// AcquireCalendar запрашивает текущую аренду для синхронизации календаря для одного
	// воркера.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Обратный вызов, используемый для завершения операции или освобождения
	//   ресурсов.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	AcquireCalendar(context.Context, string) (func(), error)
	// Fanout создаёт задания по доставке с дедупликацией для получателей, разрешённых
	// сохранёнными настройками.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//   - arg3: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Fanout(context.Context, jobs.Job, string) error
	// ScheduleReminders создаёт задания по напоминаниям для конференций, попадающих в
	// настроенное окно напоминаний.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Настраивает смещение напоминаний относительно запланированного времени
	//   проведения конференции.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ScheduleReminders(context.Context, []time.Duration) error
	// Delivery загружает метаданные уведомлений и доставки для конкретного получателя для
	// арендованного задания.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//
	// @returns:
	//   - result1: Результирующая доставка.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Delivery(context.Context, jobs.Job) (Delivery, error)
	// CompleteDelivery записывает результат доставки только для текущего арендованного задания.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//   - arg3: Состояние жизненного цикла или статус ответа на запись.
	//   - arg4: Ограниченный публичный статус или код ошибки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CompleteDelivery(context.Context, jobs.Job, string, string) error
	// ScheduleCalendarSync ставит в очередь задания по дедупликации календарей для текущей
	// версии расписания конференций.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ScheduleCalendarSync(context.Context, string) error
}

// TokenCipher шифрует секретный ключ и привязывает его к определённой учётной записи и
// соединению.
type TokenCipher interface {
	// Encrypt шифрует приватное значение провайдера с помощью случайного одноразового номера и
	// аутентифицированной привязки владельца.
	//
	// @Parameters:
	//   - arg1: Значение для проверки, нормализации или преобразования.
	//   - arg2: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
	//
	// @returns:
	//   - result1: Аутентифицированное зашифрованное значение провайдера.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Encrypt(string, string) (string, error)
	// Decrypt аутентифицирует сохраненный зашифрованный текст и привязку владельца, после чего
	// возвращает приватное значение.
	//
	// @Parameters:
	//   - arg1: Значение для проверки, нормализации или преобразования.
	//   - arg2: Подтвержденная идентификация учетной записи, сеанса и медиа-подключения.
	//
	// @returns:
	//   - result1: Аутентифицированное значение провайдера в виде открытого текста только
	//   для использования на стороне сервера.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Decrypt(string, string) (string, error)
}

// Service — сервис подключает сохранённые события к внешним адаптерам, не дожидаясь ответа провайдера и
// не прерывая конференцию или запись.
type Service struct {
	repo     Repository
	adapters d.Providers
	cipher   TokenCipher
	options  Options
	html     *template.Template
	text     *texttemplate.Template
}

// NewService создаёт сервис с зависимостями и ограничениями, необходимыми для интеграции
// учётных записей и настроек доставки.
//
// @Parameters:
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - adapters: Адаптеры внешних провайдеров, настраиваемые оператором.
//   - cipher: Зависимость от аутентифицированного шифрования для учетных данных частного
//     провайдера.
//   - options: Проверенные параметры создания или выполнения компонентов.
//
// @returns:
//   - result1: Созданный или загруженный сервис.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewService(repo Repository, adapters d.Providers, cipher TokenCipher, options Options) (*Service, error) {
	allNoop := adapters.Capabilities.Email == "noop" && adapters.Capabilities.Push == "noop" && adapters.Capabilities.Calendar == "noop"
	u, err := url.Parse(options.PublicURL)
	if !(allNoop && options.PublicURL == "") && (err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http")) {
		return nil, errors.New("PUBLIC_FRONTEND_URL must be an absolute HTTP(S) URL")
	}
	if options.ProviderTimeout <= 0 {
		options.ProviderTimeout = 30 * time.Second
	}
	if options.ProviderTimeout > 5*time.Minute {
		return nil, errors.New("integration timeout exceeds five minutes")
	}
	if len(options.ReminderOffsets) == 0 {
		options.ReminderOffsets = []time.Duration{24 * time.Hour, 15 * time.Minute}
	}
	if len(options.ReminderOffsets) > 8 {
		return nil, errors.New("too many reminder offsets")
	}
	seen := map[time.Duration]bool{}
	for _, v := range options.ReminderOffsets {
		if v < time.Second || v%time.Second != 0 || v > 30*24*time.Hour || seen[v] {
			return nil, errors.New("invalid reminder offsets")
		}
		seen[v] = true
	}
	h := template.Must(template.New("notification-v1").Parse(`<h1>{{.Subject}}</h1><p>{{.Title}}</p><p>{{.Body}}</p><p><a href="{{.URL}}">Открыть встречу</a></p>`))
	t := texttemplate.Must(texttemplate.New("notification-v1").Parse("{{.Subject}}\n{{.Title}}\n{{.Body}}\n{{.URL}}\n"))
	return &Service{repo: repo, adapters: adapters, cipher: cipher, options: options, html: h, text: t}, nil
}

// Capabilities возвращает возможности продукта или интеграции, фактически доступные оператору.
//
// @returns:
//   - result1: Результирующие возможности.
func (s *Service) Capabilities() d.Capabilities { return s.adapters.Capabilities }

// Preferences считывает сохраненные настройки доставки или переписки действующего аккаунта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Результирующие настройки.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Preferences(ctx context.Context, userID string) (d.Preferences, error) {
	return s.repo.Preferences(ctx, userID)
}

// SavePreferences проверяет и сохраняет настройки доставки аккаунта, не инициируя доставку.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) SavePreferences(ctx context.Context, userID string, value d.Preferences) error {
	value.UserID = userID
	return s.repo.SavePreferences(ctx, value)
}

// Devices возвращает зарегистрированные устройства доставки аккаунта, не раскрывая их
// конфиденциальные учетные данные.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Собранные значения устройств.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Devices(ctx context.Context, userID string) ([]d.Device, error) {
	return s.repo.Devices(ctx, userID, false)
}

// RegisterDevice регистрирует проверенное устройство доставки действующей учетной записи и
// защищает ее приватный токен.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Результирующее устройство.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) RegisterDevice(ctx context.Context, userID string, request d.DeviceRequest) (d.Device, error) {
	if s.adapters.Capabilities.Push == "noop" || s.cipher == nil {
		return d.Device{}, apperrors.ErrUnavailable
	}
	if (request.Platform != "web" && request.Platform != "ios" && request.Platform != "android") || len(request.Token) < 8 || len(request.Token) > 8192 {
		return d.Device{}, apperrors.ErrInvalidInput
	}
	sum := sha256.Sum256([]byte(request.Token))
	fp := hex.EncodeToString(sum[:])
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(userID+":"+s.adapters.Capabilities.Push+":"+fp)).String()
	binding := userID + ":device:" + id
	encrypted, err := s.cipher.Encrypt(request.Token, binding)
	if err != nil {
		return d.Device{}, apperrors.ErrUnavailable
	}
	device := d.Device{ID: id, UserID: userID, Platform: request.Platform, Provider: s.adapters.Capabilities.Push, TokenCiphertext: encrypted, TokenFingerprint: fp}
	err = s.repo.SaveDevice(ctx, device)
	return device, err
}

// RevokeDevice отзывает только выбранное устройство доставки действующего аккаунта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) RevokeDevice(ctx context.Context, userID, id string) error {
	return s.repo.RevokeDevice(ctx, userID, id)
}

// Connections возвращает подключения провайдера действующего аккаунта с исключенными из общего
// доступа конфиденциальными учетными данными.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Собранные значения подключений календаря.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Connections(ctx context.Context, userID string) ([]d.CalendarConnection, error) {
	return s.repo.Connections(ctx, userID)
}

// MockConnect создает фиктивное подключение к календарю только в том случае, если это явно
// разрешено в настройках среды.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: В результате создается подключение к календарю.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) MockConnect(ctx context.Context, userID string) (d.CalendarConnection, error) {
	if !s.options.MockConnectAllowed || !s.adapters.Capabilities.MockConnectAllowed {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	c := d.CalendarConnection{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(userID+":calendar:mock")).String(), UserID: userID, Provider: "mock", CalendarID: "primary", Status: "connected"}
	if err := s.repo.SaveConnection(ctx, c); err != nil {
		return c, err
	}
	return c, s.repo.ScheduleCalendarSync(ctx, userID)
}

// OAuthStart перед перенаправлением к провайдеру создается одноразовое состояние OAuth и
// привязанный к учетной записи верификатор PKCE.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - provider: Идентификатор провайдера, выбранный оператором, или фиксированная техническая
//     метка провайдера.
//
// @returns:
//   - result1: Авторизация провайдера URL с одноразовым состоянием и запросом PKCE.
//   - result2: Одноразовое состояние, привязанное к учетной записи и провайдеру.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) OAuthStart(ctx context.Context, userID, provider string) (string, string, error) {
	if provider != "generic" || s.adapters.OAuth == nil || s.cipher == nil {
		return "", "", apperrors.ErrUnavailable
	}
	state, err := randomToken()
	if err != nil {
		return "", "", err
	}
	verifier, err := randomToken()
	if err != nil {
		return "", "", err
	}
	id := uuid.NewString()
	encrypted, err := s.cipher.Encrypt(verifier, userID+":oauth:"+id)
	if err != nil {
		return "", "", err
	}
	record := d.OAuthState{ID: id, UserID: userID, Provider: provider, StateHash: digest(state), VerifierCiphertext: encrypted, ExpiresAt: time.Now().UTC().Add(10 * time.Minute)}
	if err = s.repo.SaveOAuthState(ctx, record); err != nil {
		return "", "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	authURL, err := s.adapters.OAuth.AuthorizeURL(state, base64.RawURLEncoding.EncodeToString(challenge[:]))
	return authURL, state, err
}

// OAuthCallback один раз использует состояние, привязанное к учетной записи и провайдеру, перед
// обменом кодом авторизации.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - provider: Идентификатор провайдера, выбранный оператором, или фиксированная техническая
//     метка провайдера.
//   - code: Ограниченный публичный статус или код ошибки.
//   - state: Текущее состояние или обновлённое валидированное состояние.
//
// @returns:
//   - result1: В результате создается подключение к календарю.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) OAuthCallback(ctx context.Context, userID, provider, code, state string) (d.CalendarConnection, error) {
	if provider != "generic" || s.adapters.OAuth == nil || s.cipher == nil {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	if len(code) < 1 || len(code) > 4096 || len(state) != 43 {
		return d.CalendarConnection{}, apperrors.ErrInvalidInput
	}
	record, err := s.repo.TakeOAuthState(ctx, userID, provider, digest(state))
	if err != nil {
		return d.CalendarConnection{}, err
	}
	verifier, err := s.cipher.Decrypt(record.VerifierCiphertext, userID+":oauth:"+record.ID)
	if err != nil {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
	defer cancel()
	credentials, err := s.adapters.OAuth.Exchange(op, code, verifier)
	if err != nil {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(userID+":calendar:generic")).String()
	connection := d.CalendarConnection{ID: id, UserID: userID, Provider: provider, CalendarID: "primary", Status: "connected"}
	if err = s.encryptCredentials(&connection, credentials); err != nil {
		return connection, err
	}
	if err = s.repo.SaveConnection(ctx, connection); err != nil {
		return connection, err
	}
	return connection, s.repo.ScheduleCalendarSync(ctx, userID)
}

// Disconnect — при отключении отменяется интеграционное подключение действующей учетной записи и
// прекращается ее дальнейшая синхронизация.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Disconnect(ctx context.Context, userID, id string) error {
	items, err := s.repo.Connections(ctx, userID)
	if err != nil {
		return err
	}
	var found *d.CalendarConnection
	for _, c := range items {
		if c.ID == id {
			v := c
			found = &v
			break
		}
	}
	if found == nil {
		return apperrors.ErrNotFound
	}
	token := ""
	if found.RefreshCiphertext != "" && s.cipher != nil {
		token, _ = s.cipher.Decrypt(found.RefreshCiphertext, userID+":calendar:"+id+":refresh")
	}
	if err = s.repo.RevokeConnection(ctx, userID, id); err != nil {
		return err
	}
	if s.adapters.OAuth != nil && token != "" {
		op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
		defer cancel()
		_ = s.adapters.OAuth.Revoke(op, token)
	}
	return nil
}

// CalendarMappings возвращает сохраненные сопоставления между конференциями и календарями для
// действующей учетной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Собранные значения сопоставлений между календарями.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) CalendarMappings(ctx context.Context, userID, conferenceID string) ([]d.CalendarMapping, error) {
	return s.repo.CalendarMappings(ctx, userID, conferenceID)
}

// Tick — тик выполняет одну ограниченную итерацию обслуживания или доставки после получения доступного
// сохраненного состояния.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Tick(ctx context.Context) error {
	return s.repo.ScheduleReminders(ctx, s.options.ReminderOffsets)
}

// eventPayload содержит только ссылки, необходимые для фонового события.
type eventPayload struct {
	Event          string `json:"event"`
	NotificationID string `json:"notificationId"`
	Channel        string `json:"channel"`
}

// Handle — обработчик передает проверенную операцию интеграции учетной записи в обработчик приложения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Handle(ctx context.Context, job jobs.Job) error {
	var payload eventPayload
	if json.Unmarshal(job.Payload, &payload) != nil {
		return jobs.Error{Code: "integration_payload"}
	}
	switch job.Kind {
	case "integrations.invitation":
		return s.deliverInvitation(ctx, job)
	case "integrations.delivery":
		return s.deliver(ctx, job, payload)
	case "integrations.event":
		return s.repo.Fanout(ctx, job, payload.Event)
	case "integrations.conference":
		if err := s.repo.Fanout(ctx, job, payload.Event); err != nil {
			return err
		}
		return s.syncCalendar(ctx, job)
	case "integrations.calendar":
		return s.syncCalendar(ctx, job)
	}
	return jobs.Error{Code: "integration_kind"}
}

// FailJob регистрирует сбой задания с ограничением, не раскрывая ответы провайдера и не заменяя
// более новую версию.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - code: Ограниченный публичный статус или код ошибки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) FailJob(ctx context.Context, job jobs.Job, code string) error {
	if job.Kind == "integrations.invitation" {
		repo, ok := s.repo.(invitationDeliveryRepository)
		if !ok {
			return jobs.Error{Code: "invitation_repository"}
		}
		return repo.CompleteInvitation(ctx, job, "failed", code)
	}
	if job.Kind == "integrations.delivery" {
		return s.repo.CompleteDelivery(ctx, job, "failed", code)
	}
	if job.Kind == "integrations.calendar" || job.Kind == "integrations.conference" {
		return s.repo.FailCalendars(ctx, job, code)
	}
	return nil
}

// templateData содержит только минимальное безопасное представление событий.
type templateData struct{ Subject, Title, Body, URL string }

// RenderEmail создает экранированный HTML и текстовое уведомление по электронной почте на
// основе версионного шаблона и доверенной ссылки UI.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//   - to: Конечное состояние или получатель операции.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - title: Подтвержденное название конференции или уведомления.
//   - link: Доверенная ссылка на внешний интерфейс, созданная приложением.
//
// @returns:
//   - result1: Результирующее электронное письмо.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) RenderEmail(key, to, kind, title, link string) (d.EmailMessage, error) {
	body := eventText(kind)
	data := templateData{Subject: body, Title: title, Body: body, URL: link}
	var h, t bytes.Buffer
	if err := s.html.Execute(&h, data); err != nil {
		return d.EmailMessage{}, err
	}
	if err := s.text.Execute(&t, data); err != nil {
		return d.EmailMessage{}, err
	}
	return d.EmailMessage{IdempotencyKey: key, To: to, Subject: body, HTML: h.String(), Text: t.String()}, nil
}

// eventText создает текст события с привязкой к календарю на основе сохраненных данных о
// конференции.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//
// @returns:
//   - result1: Текст события с привязкой к внешнему календарю.
func eventText(kind string) string {
	switch kind {
	case "conference.invited":
		return "Приглашение на встречу"
	case "conference.rescheduled":
		return "Время встречи изменилось"
	case "conference.cancelled":
		return "Встреча отменена"
	case "conference.soon":
		return "Встреча скоро начнётся"
	case "recording.ready":
		return "Запись встречи готова"
	case "transcript.ready":
		return "Расшифровка встречи готова"
	case "summary.ready":
		return "Итоги встречи готовы"
	case "processing.failed":
		return "Обработка встречи не завершилась"
	default:
		return "Обновление встречи"
	}
}

// deliver проверяет текущие права доступа получателя непосредственно перед доставкой
// сохраненного события или команды.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - payload: Структурированная полезная информация о событии или команде.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) deliver(ctx context.Context, job jobs.Job, payload eventPayload) error {
	delivery, err := s.repo.Delivery(ctx, job)
	if err != nil {
		return err
	}
	skip := !delivery.Allowed || !delivery.Preferences.Allows(delivery.Notification.Type)
	if payload.Channel == "email" {
		skip = skip || !delivery.Preferences.Email || s.adapters.Capabilities.Email == "noop"
	} else if payload.Channel == "push" {
		skip = skip || !delivery.Preferences.Push || s.adapters.Capabilities.Push == "noop"
	} else {
		return jobs.Error{Code: "delivery_channel"}
	}
	if skip {
		if err = s.repo.CompleteDelivery(ctx, job, "skipped", ""); err != nil {
			return err
		}
		return jobs.ErrSkip
	}
	conference, err := s.repo.Conference(ctx, job.ConferenceID)
	if err != nil {
		return err
	}
	link := strings.TrimRight(s.options.PublicURL, "/") + "/conferences/" + url.PathEscape(conference.ID)
	if delivery.Notification.Payload.RecordingID != "" && (delivery.Notification.Type == "transcript.ready" || delivery.Notification.Type == "summary.ready") {
		query := url.Values{"recording": {delivery.Notification.Payload.RecordingID}, "tab": {"transcript"}}
		if delivery.Notification.Type == "summary.ready" {
			query.Set("tab", "summary")
		}
		link += "?" + query.Encode()
	}
	op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
	defer cancel()
	if payload.Channel == "email" {
		message, err := s.RenderEmail(job.DedupKey, delivery.Email, delivery.Notification.Type, conference.Title, link)
		if err != nil {
			return err
		}
		if err = s.adapters.Email.Send(op, message); err != nil {
			return err
		}
	} else {
		devices, err := s.repo.Devices(ctx, delivery.Notification.UserID, true)
		if err != nil {
			return err
		}
		if len(devices) == 0 {
			if err = s.repo.CompleteDelivery(ctx, job, "skipped", ""); err != nil {
				return err
			}
			return jobs.ErrSkip
		}
		for _, device := range devices {
			if s.cipher == nil {
				return jobs.Error{Code: "provider_encryption_disabled"}
			}
			token, err := s.cipher.Decrypt(device.TokenCiphertext, device.UserID+":device:"+device.ID)
			if err != nil {
				return jobs.Error{Code: "device_token_invalid"}
			}
			if err = s.adapters.Push.Send(op, d.PushMessage{IdempotencyKey: job.DedupKey + ":" + device.ID, Token: token, Title: conference.Title, Body: eventText(delivery.Notification.Type), URL: link}); err != nil {
				return err
			}
		}
	}
	return s.repo.CompleteDelivery(ctx, job, "delivered", "")
}

// syncCalendar обновляет сопоставление внешнего календаря только до тех пор, пока активна
// аренда задания и генерация расписания конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) syncCalendar(ctx context.Context, job jobs.Job) error {
	if s.adapters.Capabilities.Calendar == "noop" {
		return jobs.ErrSkip
	}
	release, err := s.repo.AcquireCalendar(ctx, job.ConferenceID)
	if err != nil {
		return err
	}
	defer release()
	conference, err := s.repo.Conference(ctx, job.ConferenceID)
	if err != nil {
		return err
	}
	if job.Version != conference.IntegrationVersion {
		return jobs.ErrSkip
	}
	connections, err := s.repo.Connections(ctx, conference.OwnerID)
	if err != nil {
		return err
	}
	mappings, err := s.repo.MappingsForSync(ctx, conference.ID)
	if err != nil {
		return err
	}
	for _, connection := range connections {
		if connection.Status != "connected" {
			continue
		}
		if (connection.Provider == "mock" && s.adapters.Capabilities.Calendar != "mock") || (connection.Provider != "mock" && s.adapters.Capabilities.Calendar != "http") {
			continue
		}
		mapping := d.CalendarMapping{ConferenceID: conference.ID, ConnectionID: connection.ID, Provider: connection.Provider, ExternalCalendarID: connection.CalendarID, SourceVersion: job.Version, SyncStatus: "pending"}
		for _, m := range mappings {
			if m.ConnectionID == connection.ID {
				mapping.ID = m.ID
				mapping.ExternalEventID = m.ExternalEventID
				break
			}
		}
		if conference.Status != "cancelled" && conference.ScheduledAt == nil {
			continue
		}
		credentials, err := s.connectionCredentials(ctx, &connection)
		if err != nil {
			return err
		}
		duration := 60
		if conference.PlannedDurationMin != nil {
			duration = *conference.PlannedDurationMin
		}
		event := d.CalendarEvent{IdempotencyKey: "calendar:" + conference.ID + ":" + connection.ID, ID: mapping.ExternalEventID, CalendarID: connection.CalendarID, Title: conference.Title, SourceVersion: job.Version, JoinURL: strings.TrimRight(s.options.PublicURL, "/") + "/i/" + url.PathEscape(conference.InviteCode)}
		if conference.ScheduledAt != nil {
			event.StartsAt = conference.ScheduledAt.UTC()
			event.EndsAt = event.StartsAt.Add(time.Duration(duration) * time.Minute)
		}
		op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
		if conference.Status == "cancelled" {
			if event.ID != "" {
				err = s.adapters.Calendar.CancelEvent(op, credentials, event)
			}
			mapping.SyncStatus = "cancelled"
		} else if event.ID == "" {
			var result d.CalendarEvent
			result, err = s.adapters.Calendar.CreateEvent(op, credentials, event)
			mapping.ExternalEventID = result.ID
			// После потери ответа идемпотентная операция создания может вернуть более раннюю
			// версию события.  Более новое расписание должно обновлять тот же идентификатор, а
			// не помечать устаревшие данные как синхронизированные.
			if err == nil && (result.SourceVersion != job.Version || !result.StartsAt.Equal(event.StartsAt) || !result.EndsAt.Equal(event.EndsAt) || result.Title != event.Title || result.JoinURL != event.JoinURL) {
				event.ID = result.ID
				event.IdempotencyKey += ":" + strconv.FormatInt(job.Version, 10)
				_, err = s.adapters.Calendar.UpdateEvent(op, credentials, event)
			}
			mapping.SyncStatus = "synced"
		} else {
			event.IdempotencyKey += ":" + strconv.FormatInt(job.Version, 10)
			_, err = s.adapters.Calendar.UpdateEvent(op, credentials, event)
			mapping.SyncStatus = "synced"
		}
		cancel()
		if err != nil {
			mapping.SyncStatus = "failed"
			_ = s.repo.SaveMapping(ctx, job, mapping)
			return err
		}
		now := time.Now().UTC()
		mapping.LastSyncedAt = &now
		if err = s.repo.SaveMapping(ctx, job, mapping); err != nil {
			return err
		}
	}
	return nil
}

// encryptCredentials шифрует учетные данные провайдера перед сохранением.
//
// @Parameters:
//   - connection: Текущая интеграция учетной записи или физическое транспортное соединение.
//   - credentials: Учетные данные провайдера только для сервера; никогда не раскрывайте их в
//     логах или публичных ответах.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) encryptCredentials(connection *d.CalendarConnection, credentials d.CalendarCredentials) error {
	if s.cipher == nil {
		return apperrors.ErrUnavailable
	}
	binding := connection.UserID + ":calendar:" + connection.ID
	access, err := s.cipher.Encrypt(credentials.AccessToken, binding+":access")
	if err != nil {
		return err
	}
	refresh := ""
	if credentials.RefreshToken != "" {
		refresh, err = s.cipher.Encrypt(credentials.RefreshToken, binding+":refresh")
		if err != nil {
			return err
		}
	}
	connection.AccessCiphertext = access
	connection.RefreshCiphertext = refresh
	connection.ExpiresAt = credentials.ExpiresAt
	connection.Scopes = credentials.Scopes
	return nil
}

// connectionCredentials загружает и расшифровывает учетные данные провайдера, обновляя их при
// необходимости.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - connection: Текущая интеграция учетной записи или физическое транспортное соединение.
//
// @returns:
//   - result1: Результирующие учетные данные календаря.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) connectionCredentials(ctx context.Context, connection *d.CalendarConnection) (d.CalendarCredentials, error) {
	if connection.Provider == "mock" {
		return d.CalendarCredentials{}, nil
	}
	if s.cipher == nil {
		return d.CalendarCredentials{}, jobs.Error{Code: "provider_encryption_disabled"}
	}
	binding := connection.UserID + ":calendar:" + connection.ID
	access, err := s.cipher.Decrypt(connection.AccessCiphertext, binding+":access")
	if err != nil {
		return d.CalendarCredentials{}, jobs.Error{Code: "calendar_token_invalid"}
	}
	refresh := ""
	if connection.RefreshCiphertext != "" {
		refresh, err = s.cipher.Decrypt(connection.RefreshCiphertext, binding+":refresh")
		if err != nil {
			return d.CalendarCredentials{}, jobs.Error{Code: "calendar_token_invalid"}
		}
	}
	credentials := d.CalendarCredentials{AccessToken: access, RefreshToken: refresh, ExpiresAt: connection.ExpiresAt, Scopes: connection.Scopes}
	if credentials.ExpiresAt != nil && credentials.ExpiresAt.Before(time.Now().Add(time.Minute)) {
		if s.adapters.OAuth == nil || refresh == "" {
			return credentials, jobs.Error{Code: "calendar_reconnect_required"}
		}
		credentials, err = s.adapters.OAuth.Refresh(ctx, refresh)
		if err != nil {
			return credentials, err
		}
		oldRefresh := connection.RefreshCiphertext
		if err = s.encryptCredentials(connection, credentials); err != nil {
			return credentials, err
		}
		if err = s.repo.RefreshConnection(ctx, *connection, oldRefresh); err != nil {
			return credentials, err
		}
	}
	return credentials, nil
}

// randomToken генерирует криптографически стойкий одноразовый токен.
//
// @returns:
//   - result1: Криптографически стойкий URL-безопасный секрет.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// digest — дайджест хеширует одноразовый секретный ключ для сравнения, не сохраняя исходное значение.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Дайджест предоставленного секретного ключа в шестнадцатеричном формате.
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

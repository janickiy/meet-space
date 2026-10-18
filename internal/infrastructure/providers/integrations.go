package providers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	d "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
)

// AdapterConfig выбирает режим доставки и доверенную конечную точку серверного шлюза; введенные
// пользователем URLs отклоняются.
type AdapterConfig struct {
	Mode      string
	Endpoint  string
	Secret    string
	Timeout   time.Duration
	AllowHTTP bool
}

// IntegrationConfig группирует независимые настройки каналов доставки и календаря OAuth.
type IntegrationConfig struct {
	Email, Push, Calendar AdapterConfig
	SMTP                  SMTPConfig
	OAuth                 OAuthConfig
	MockConnectAllowed    bool
}

// NewIntegrations создает интеграции с зависимостями и ограничениями, необходимыми для
// ограниченных внешних интеграций.
//
// @Parameters:
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//
// @returns:
//   - result1: Результирующие провайдеры.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewIntegrations(cfg IntegrationConfig) (d.Providers, error) {
	var email d.EmailProvider
	emailMode := cfg.Email.Mode
	if emailMode == "smtp" {
		var err error
		email, err = NewSMTPEmail(cfg.SMTP)
		if err != nil {
			return d.Providers{}, err
		}
	} else {
		e, err := newGateway(cfg.Email)
		if err != nil {
			return d.Providers{}, err
		}
		email, emailMode = &EmailAdapter{e}, e.mode
	}
	p, err := newGateway(cfg.Push)
	if err != nil {
		return d.Providers{}, err
	}
	c, err := newGateway(cfg.Calendar)
	if err != nil {
		return d.Providers{}, err
	}
	result := d.Providers{Email: email, Push: &PushAdapter{p}, Calendar: &CalendarAdapter{gateway: c}, Capabilities: d.Capabilities{Email: emailMode, Push: p.mode, Calendar: c.mode, MockConnectAllowed: cfg.MockConnectAllowed && c.mode == "mock"}}
	if cfg.OAuth.ClientID != "" {
		o, err := NewOAuth(cfg.OAuth)
		if err != nil {
			return result, err
		}
		result.OAuth = o
		result.Capabilities.CalendarOAuthConfigured = true
	}
	return result, nil
}

// gateway — шлюз выполняет ограниченные вызовы JSON HTTP или имитирует детерминированное поведение без
// доступа к сети.
type gateway struct {
	mode     string
	endpoint string
	secret   string
	client   *http.Client
}

// newGateway создает ограниченный шлюз HTTP для конечной точки интеграции, настраиваемой
// оператором.
//
// @Parameters:
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//
// @returns:
//   - result1: Созданный или загруженный шлюз.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func newGateway(cfg AdapterConfig) (*gateway, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = "noop"
	}
	if mode != "noop" && mode != "mock" && mode != "http" {
		return nil, errors.New("invalid integration provider mode")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Timeout > 5*time.Minute {
		return nil, errors.New("integration provider timeout exceeds five minutes")
	}
	g := &gateway{mode: mode, secret: cfg.Secret, endpoint: strings.TrimRight(cfg.Endpoint, "/"), client: &http.Client{Timeout: cfg.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if mode == "http" {
		u, err := url.Parse(cfg.Endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || (u.Scheme != "https" && !(cfg.AllowHTTP && u.Scheme == "http")) {
			return nil, errors.New("integration endpoint must be a trusted HTTPS URL")
		}
		if cfg.Secret == "" {
			return nil, errors.New("HTTP integration provider secret is required")
		}
	}
	return g, nil
}

// call — вызов выполняет ограниченный запрос к настроенному провайдеру и возвращает только безопасные
// классификации ошибок.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - operation: Фиксированный провайдер из списка разрешенных или операция протокола.
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//   - input: Входные данные, необходимые для этой операции.
//   - output: Целевой или сгенерированный результат, необходимый для выполнения этой операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (g *gateway) call(ctx context.Context, operation, key string, input, output any) error {
	if g.mode == "noop" {
		return jobs.ErrSkip
	}
	if g.mode == "mock" {
		return nil
	}
	body, err := json.Marshal(input)
	if err != nil {
		return jobs.Error{Code: "provider_input", Retryable: false}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint+"/"+operation, bytes.NewReader(body))
	if err != nil {
		return jobs.Error{Code: "provider_request"}
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	res, err := g.client.Do(req)
	if err != nil {
		return jobs.Error{Code: "provider_unavailable", Retryable: true}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
	if err != nil {
		return jobs.Error{Code: "provider_response", Retryable: true}
	}
	if len(raw) > 1<<20 {
		return jobs.Error{Code: "provider_response_too_large"}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		retry := res.StatusCode == 429 || res.StatusCode == 408 || res.StatusCode >= 500
		after := time.Duration(0)
		if seconds, err := time.ParseDuration(res.Header.Get("Retry-After") + "s"); err == nil && seconds > 0 && seconds <= time.Hour {
			after = seconds
		}
		code := "provider_http"
		if res.StatusCode == 429 {
			code = "provider_rate_limited"
		}
		return jobs.Error{Code: code, Retryable: retry, RetryAfter: after}
	}
	if output != nil && json.Unmarshal(raw, output) != nil {
		return jobs.Error{Code: "provider_invalid_response"}
	}
	return nil
}

// EmailAdapter реализует EmailProvider через серверный шлюз или в явно обозначенном режиме
// имитации или отключения.
type EmailAdapter struct{ gateway *gateway }

// Send доставляет ограниченное электронное письмо со стабильным идентификатором сообщения;
// настроенное транспортное средство определяет возможность дедупликации при повторных попытках
// доставки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - message: Ограниченное содержимое электронного письма, получатель и стабильная
//     идентификация доставки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (a *EmailAdapter) Send(ctx context.Context, message d.EmailMessage) error {
	return a.gateway.call(ctx, "email/send", message.IdempotencyKey, message, nil)
}

// PushAdapter обеспечивает безопасную push-доставку через заменяемый шлюз.
type PushAdapter struct{ gateway *gateway }

// Send доставляет минимальное уведомление на выбранное зарегистрированное устройство без
// расшифровки текста и без использования секретов провайдера.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - message: Минимальное push-уведомление и зарегистрированное устройство-получатель.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (a *PushAdapter) Send(ctx context.Context, message d.PushMessage) error {
	return a.gateway.call(ctx, "push/send", message.IdempotencyKey, message, nil)
}

// CalendarAdapter сохраняет стабильный внешний идентификатор события и выполняет операции
// жизненного цикла через шлюз.
type CalendarAdapter struct{ gateway *gateway }

// calendarInput сериализует учетные данные только в защищенный запрос к доверенному шлюзу, а не
// в общедоступные ответы или логи API.
type calendarInput struct {
	Event       d.CalendarEvent `json:"event"`
	AccessToken string          `json:"accessToken"`
}

// invoke выполняет настроенную календарную операцию, используя детерминированную
// идемпотентность в явно разрешенном режиме имитации.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - operation: Фиксированный провайдер из списка разрешенных или операция протокола.
//   - credentials: Учетные данные провайдера только для сервера; никогда не раскрывайте их в
//     логах или публичных ответах.
//   - event: Доверенное событие для публикации или обработки.
//
// @returns:
//   - result1: Результирующее календарное событие.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (a *CalendarAdapter) invoke(ctx context.Context, operation string, credentials d.CalendarCredentials, event d.CalendarEvent) (d.CalendarEvent, error) {
	if a.gateway.mode == "mock" {
		if event.ID == "" {
			sum := sha256.Sum256([]byte(event.IdempotencyKey))
			event.ID = "mock-" + hex.EncodeToString(sum[:12])
		}
		return event, nil
	}
	var out d.CalendarEvent
	err := a.gateway.call(ctx, "calendar/"+operation, event.IdempotencyKey+":"+operation, calendarInput{event, credentials.AccessToken}, &out)
	if err != nil {
		return out, err
	}
	if out.ID == "" || len(out.ID) > 512 || len(out.CalendarID) > 512 {
		return out, jobs.Error{Code: "calendar_missing_event_id"}
	}
	return out, nil
}

// CreateEvent создает идемпотентное календарное событие с помощью настроенного провайдера.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - c: Значение учетных данных календаря, обрабатываемое этой операцией.
//   - e: Состояние события календаря, обрабатываемое этой операцией.
//
// @returns:
//   - result1: Результирующее календарное событие.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (a *CalendarAdapter) CreateEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) (d.CalendarEvent, error) {
	return a.invoke(ctx, "create", c, e)
}

// UpdateEvent обновляет сопоставленное внешнее календарное событие для текущего расписания
// конференции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - c: Значение учетных данных календаря, обрабатываемое этой операцией.
//   - e: Состояние события календаря, обрабатываемое этой операцией.
//
// @returns:
//   - result1: Результирующее календарное событие.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (a *CalendarAdapter) UpdateEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) (d.CalendarEvent, error) {
	return a.invoke(ctx, "update", c, e)
}

// CancelEvent отменяет внешнее календарное событие, связанное с сохранённым сопоставлением.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - c: Значение учетных данных календаря, обрабатываемое этой операцией.
//   - e: Состояние события календаря, обрабатываемое этой операцией.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (a *CalendarAdapter) CancelEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) error {
	if a.gateway.mode == "mock" {
		return nil
	}
	return a.gateway.call(ctx, "calendar/cancel", e.IdempotencyKey+":cancel", calendarInput{e, c.AccessToken}, nil)
}

// GetEvent считывает сопоставленное внешнее календарное событие для сверки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - c: Значение учетных данных календаря, обрабатываемое этой операцией.
//   - e: Состояние события календаря, обрабатываемое этой операцией.
//
// @returns:
//   - result1: Результирующее календарное событие.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (a *CalendarAdapter) GetEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) (d.CalendarEvent, error) {
	return a.invoke(ctx, "get", c, e)
}

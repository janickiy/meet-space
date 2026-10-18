package providers

import (
	"context"
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

// OAuthConfig содержит только доверенные конечные точки сервера и минимальные области действия
// адаптера календаря.
type OAuthConfig struct {
	AuthorizationURL, TokenURL, RevokeURL, ClientID, ClientSecret, RedirectURL string
	Scopes                                                                     []string
	Timeout                                                                    time.Duration
	AllowHTTP                                                                  bool
}

// OAuth реализует обмен, обновление и отзыв кодов S256 и PKCE на стороне сервера без
// использования провайдера SDK.
type OAuth struct {
	cfg    OAuthConfig
	client *http.Client
}

// NewOAuth создает объект o auth с зависимостями и ограничениями, необходимыми для интеграции с
// внешними системами.
//
// @Parameters:
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//
// @returns:
//   - result1: Созданный или загруженный объект o auth.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewOAuth(cfg OAuthConfig) (*OAuth, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || len(cfg.Scopes) == 0 {
		return nil, errors.New("calendar OAuth client credentials and minimal scopes are required")
	}
	for _, value := range []string{cfg.AuthorizationURL, cfg.TokenURL, cfg.RevokeURL, cfg.RedirectURL} {
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && !(cfg.AllowHTTP && u.Scheme == "http")) {
			return nil, errors.New("calendar OAuth endpoints must be trusted HTTPS URLs")
		}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Timeout > 5*time.Minute {
		return nil, errors.New("OAuth timeout exceeds five minutes")
	}
	return &OAuth{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// AuthorizeURL формирует перенаправление авторизации провайдера с одноразовым состоянием и
// запросом S256 PKCE, не раскрывая клиентский секрет.
//
// @Parameters:
//   - state: Текущее состояние или обновлённое валидированное состояние.
//   - challenge: Запрос S256 PKCE отправляется провайдеру авторизации.
//
// @returns:
//   - result1: Авторизация провайдера URL без клиентского секрета.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (o *OAuth) AuthorizeURL(state, challenge string) (string, error) {
	u, err := url.Parse(o.cfg.AuthorizationURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", o.cfg.ClientID)
	q.Set("redirect_uri", o.cfg.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(o.cfg.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// tokenResponse ограничивает анализируемые OAuth и JSON поля учетными данными, необходимыми для
// работы календаря.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

// request — запрос вызывает фиксированную конечную точку OAuth, не раскрывая содержимое ответов
// провайдера в случае ошибок.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - endpoint: Конечная точка сервиса, настраиваемая оператором.
//   - form: Поля формы OAuth, созданные на сервере.
//   - response: Ответ приложения, в котором обрабатываются метаданные сеанса или общедоступные
//     метаданные.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (o *OAuth) request(ctx context.Context, endpoint string, form url.Values, response *tokenResponse) error {
	form.Set("client_id", o.cfg.ClientID)
	form.Set("client_secret", o.cfg.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return jobs.Error{Code: "oauth_request"}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := o.client.Do(req)
	if err != nil {
		return jobs.Error{Code: "oauth_unavailable", Retryable: true}
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 64*1024+1))
	if err != nil {
		return jobs.Error{Code: "oauth_response", Retryable: true}
	}
	if len(b) > 64*1024 {
		return jobs.Error{Code: "oauth_response_size"}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		code := "oauth_http"
		if res.StatusCode == 429 {
			code = "oauth_rate_limited"
		}
		return jobs.Error{Code: code, Retryable: res.StatusCode == 429 || res.StatusCode >= 500}
	}
	if response != nil && (json.Unmarshal(b, response) != nil || response.AccessToken == "" || !strings.EqualFold(response.TokenType, "bearer") || response.ExpiresIn < 0 || response.ExpiresIn > 366*24*3600) {
		return jobs.Error{Code: "oauth_invalid_token"}
	}
	return nil
}

// credentials декодирует и проверяет учетные данные провайдера, не возвращая их в общедоступных
// ответах.
//
// @Parameters:
//   - r: Недоверенный ответ токена провайдера для проверки и преобразования в закрытые учетные
//     данные.
//
// @returns:
//   - result1: Результирующие учетные данные календаря.
func credentials(r tokenResponse) d.CalendarCredentials {
	var at *time.Time
	if r.ExpiresIn > 0 {
		v := time.Now().UTC().Add(time.Duration(r.ExpiresIn) * time.Second)
		at = &v
	}
	return d.CalendarCredentials{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, ExpiresAt: at, Scopes: r.Scope}
}

// Exchange обменивает одноразовый код авторизации и верификатор PKCE на учетные данные
// провайдера на сервере.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - code: Ограниченный публичный статус или код ошибки.
//   - verifier: Верификатор токена или приватный верификатор PKCE в зависимости от его типа.
//
// @returns:
//   - result1: Результирующие учетные данные календаря.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (o *OAuth) Exchange(ctx context.Context, code, verifier string) (d.CalendarCredentials, error) {
	var r tokenResponse
	err := o.request(ctx, o.cfg.TokenURL, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {o.cfg.RedirectURL}}, &r)
	return credentials(r), err
}

// Refresh обновляет краткосрочные учетные данные для доступа, используя текущий провайдер или
// сеанс обновления учетной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//
// @returns:
//   - result1: Результирующие учетные данные календаря.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (o *OAuth) Refresh(ctx context.Context, token string) (d.CalendarCredentials, error) {
	var r tokenResponse
	err := o.request(ctx, o.cfg.TokenURL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}}, &r)
	if r.RefreshToken == "" {
		r.RefreshToken = token
	}
	return credentials(r), err
}

// Revoke отзывает сохраненные права доступа к провайдеру без изменения локальных настроек
// доставки учетной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (o *OAuth) Revoke(ctx context.Context, token string) error {
	return o.request(ctx, o.cfg.RevokeURL, url.Values{"token": {token}}, nil)
}

package media

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/media"
)

// HTTPClient выполняет внутренние медиавызовы, никогда не передает учетные данные при
// редиректах и никогда не предоставляет тела ответов с SDP в случае ошибок.
type HTTPClient struct {
	secret string
	client *http.Client
}

// NewHTTPClient создает клиент HTTP с зависимостями и ограничениями, необходимыми для
// аутентифицированного управления медиакомнатами.
//
// @Parameters:
//   - secret: Материал для создания закрытого ключа или необработанный секретный ключ; его
//     нельзя регистрировать или раскрывать.
//   - timeout: Максимальное время, отведенное на выполнение этой операции.
//
// @returns:
//   - result1: Созданный или загруженный клиент HTTP.
func NewHTTPClient(secret string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{secret: secret, client: &http.Client{Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, MaxIdleConns: 64, MaxIdleConnsPerHost: 32, IdleConnTimeout: 60 * time.Second, ResponseHeaderTimeout: timeout},
	}}
}

// Call — вызов выполняет аутентифицированную внутреннюю операцию HTTP с выбранным медиа-воркером.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - action: Запрошенное действие по управлению жизненным циклом или модерации.
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//
// @returns:
//   - result1: Результирующий результат.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *HTTPClient) Call(ctx context.Context, action string, command domain.Command) (domain.Result, error) {
	if config.ValidateMediaEndpoint(command.Route.Endpoint) != nil {
		return domain.Result{}, domain.ErrInvalid
	}
	switch action {
	case "join", "offer", "ready", "ice", "leave", "unpublish", "policy", "close":
	default:
		return domain.Result{}, domain.ErrInvalid
	}
	raw, err := json.Marshal(command)
	if err != nil || len(raw) > domain.MaxCommandBytes {
		return domain.Result{}, domain.ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(command.Route.Endpoint, "/")+"/internal/media/"+action, bytes.NewReader(raw))
	if err != nil {
		return domain.Result{}, domain.ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("X-Request-ID", command.RequestID)
	resp, err := c.client.Do(req)
	if err != nil {
		return domain.Result{}, apperrors.Wrap(domain.ErrUnavailable, err, domain.ErrorCode(domain.ErrUnavailable))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, domain.MaxCommandBytes+1))
	if err != nil {
		return domain.Result{}, apperrors.Wrap(domain.ErrUnavailable, err, domain.ErrorCode(domain.ErrUnavailable))
	}
	if len(body) > domain.MaxCommandBytes {
		return domain.Result{}, domain.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &failure)
		code := failure.Code
		// В более старых воркерах для того же машинного кода использовалось поле error.
		if code == "" {
			code = failure.Error
		}
		return domain.Result{}, domain.ErrorFromCode(code)
	}
	var result domain.Result
	if len(body) > 0 && json.Unmarshal(body, &result) != nil {
		return domain.Result{}, domain.ErrUnavailable
	}
	return result, nil
}

// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
func (c *HTTPClient) Close() { c.client.CloseIdleConnections() }

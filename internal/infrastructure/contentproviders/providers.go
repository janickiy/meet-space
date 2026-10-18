// Пакет contentproviders содержит заменяемые отключенные, фиктивные и HTTP-адаптеры для
// транскрипции и AI-адаптеры.  Общий контракт HTTP описан отдельно; провайдер SDKs не входит в
// доменный уровень.
package contentproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	domain "github.com/janickiy/meet-space/internal/domain/content"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// provider — провайдер содержит настройки только для сервера; токен не возвращается и не регистрируется.
type provider struct {
	mode, endpoint, token, model string
	client                       *http.Client
}

// newProvider создает ограниченного внешнего клиента-провайдера на основе настроек, управляемых
// оператором.
//
// @Parameters:
//   - mode: Проверенная операция или режим записи.
//   - endpoint: Конечная точка сервиса, настраиваемая оператором.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//   - model: Настроенный идентификатор модели провайдера.
//   - timeout: Максимальное время, отведенное на выполнение этой операции.
//
// @returns:
//   - result1: Созданный или загруженный провайдер.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func newProvider(mode, endpoint, token, model string, timeout time.Duration) (*provider, error) {
	if mode != "noop" && mode != "mock" && mode != "http" {
		return nil, fmt.Errorf("invalid content provider mode")
	}
	if timeout <= 0 || timeout > time.Hour {
		return nil, fmt.Errorf("invalid content provider timeout")
	}
	if mode == "http" {
		u, e := url.Parse(endpoint)
		if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || token == "" {
			return nil, fmt.Errorf("HTTP provider requires trusted URL and credential")
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = min(timeout, 30*time.Second)
	return &provider{mode: mode, endpoint: endpoint, token: token, model: model, client: &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// NewTranscriptionProvider создает провайдера транскрипции с зависимостями и ограничениями,
// необходимыми для провайдеров транскрипции, реферирования и встраивания.
//
// @Parameters:
//   - mode: Проверенная операция или режим записи.
//   - endpoint: Конечная точка сервиса, настраиваемая оператором.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//   - timeout: Максимальное время, отведенное на выполнение этой операции.
//
// @returns:
//   - result1: Результирующий провайдер транскрипции.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewTranscriptionProvider(mode, endpoint, token string, timeout time.Duration) (domain.TranscriptionProvider, error) {
	p, e := newProvider(mode, endpoint, token, "", timeout)
	if e != nil {
		return nil, e
	}
	return &transcription{p}, e
}

// NewAIProvider создает провайдера искусственного интеллекта с зависимостями и ограничениями,
// необходимыми для провайдеров транскрипции, реферирования и встраивания.
//
// @Parameters:
//   - mode: Проверенная операция или режим записи.
//   - endpoint: Конечная точка сервиса, настраиваемая оператором.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//   - model: Настроенный идентификатор модели провайдера.
//   - timeout: Максимальное время, отведенное на выполнение этой операции.
//
// @returns:
//   - result1: Результирующий провайдер искусственного интеллекта.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewAIProvider(mode, endpoint, token, model string, timeout time.Duration) (domain.AIProvider, error) {
	p, e := newProvider(mode, endpoint, token, model, timeout)
	if e != nil {
		return nil, e
	}
	return &intelligence{p}, e
}

// transcription — транскрипция реализует общий контракт распознавания речи; в тестовом режиме генерируется
// текст, предназначенный исключительно для тестирования.
type transcription struct{ *provider }

// Name возвращает настроенный идентификатор провайдера, используемый для маркировки и проверки
// сгенерированного контента.
//
// @returns:
//   - result1: Настроенный идентификатор провайдера.
func (p *transcription) Name() string { return p.mode }

// Transcribe отправляет ограниченный объём аудиозаписи настроенный провайдер транскрипции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Результат транскрипции.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *transcription) Transcribe(ctx context.Context, request domain.TranscriptionRequest) (domain.TranscriptionResult, error) {
	if p.mode == "noop" {
		return domain.TranscriptionResult{}, jobs.ErrSkip
	}
	if p.mode == "mock" {
		return domain.TranscriptionResult{Language: "ru", Segments: []domain.Segment{{StartMS: 0, EndMS: min(int64(request.DurationSec)*1000, 5000), Text: "ТЕСТОВАЯ РАСШИФРОВКА: обсуждение проекта и следующих шагов. Это демонстрационный текст, не результат распознавания аудио."}}}, nil
	}
	if request.Audio == nil || request.Size <= 0 || request.Size > 2<<30 {
		return domain.TranscriptionResult{}, &jobs.Error{Code: "invalid_audio", Retryable: false}
	}
	var header bytes.Buffer
	writer := multipart.NewWriter(&header)
	if e := writer.WriteField("language", request.Language); e != nil {
		return domain.TranscriptionResult{}, e
	}
	if _, e := writer.CreateFormFile("audio", "meeting.wav"); e != nil {
		return domain.TranscriptionResult{}, e
	}
	prefix := append([]byte(nil), header.Bytes()...)
	header.Reset()
	if e := writer.Close(); e != nil {
		return domain.TranscriptionResult{}, e
	}
	suffix := append([]byte(nil), header.Bytes()...)
	body := io.MultiReader(bytes.NewReader(prefix), io.LimitReader(request.Audio, request.Size), bytes.NewReader(suffix))
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, body)
	if e != nil {
		return domain.TranscriptionResult{}, &jobs.Error{Code: "provider_request", Retryable: false}
	}
	req.ContentLength = int64(len(prefix)) + request.Size + int64(len(suffix))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	data, e := p.call(req, request.IdempotencyKey, 10<<20)
	if e != nil {
		return domain.TranscriptionResult{}, e
	}
	var out domain.TranscriptionResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil {
		return out, &jobs.Error{Code: "invalid_transcript", Retryable: false}
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return out, &jobs.Error{Code: "invalid_transcript", Retryable: false}
	}
	return out, nil
}

// intelligence — интеллектуальные запросы JSON — только вывод AI без использования инструментов, вызовов
// функций или сетевых действий.
type intelligence struct{ *provider }

// Name возвращает настроенный идентификатор провайдера, используемый для маркировки и проверки
// сгенерированного контента.
//
// @returns:
//   - result1: Настроенный идентификатор провайдера.
func (p *intelligence) Name() string { return p.mode }

// Model возвращает настроенный идентификатор модели, используемый для маркировки и проверки
// сгенерированного контента.
//
// @returns:
//   - result1: Настроенный идентификатор модели провайдера.
func (p *intelligence) Model() string {
	if p.mode == "mock" {
		return "deterministic-demo"
	}
	return p.model
}

// Summarize создает сводную информацию с ограничениями по схеме на основе переданного
// транскрипта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Результирующее необработанное сообщение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *intelligence) Summarize(ctx context.Context, request domain.AIRequest) (json.RawMessage, error) {
	if p.mode == "noop" {
		return nil, jobs.ErrSkip
	}
	if p.mode == "mock" {
		return json.Marshal(domain.SummaryOutput{Summary: "ТЕСТОВАЯ СВОДКА: обсуждение проекта и следующих шагов. Демонстрация, не реальный AI-анализ.", KeyPoints: []string{"Тестовые данные; провайдер mock"}, ActionItems: []domain.ActionItem{}, Topics: []string{"Демонстрация"}})
	}
	if len(request.InputJSON) > 1<<20 || len(request.Instructions) > 20000 {
		return nil, &jobs.Error{Code: "ai_input_limit", Retryable: false}
	}
	body, e := json.Marshal(map[string]any{"model": p.model, "promptVersion": request.PromptVersion, "schemaVersion": request.SchemaVersion, "instructions": request.Instructions, "input": json.RawMessage(request.InputJSON), "merge": request.Merge, "responseFormat": "meeting-summary-v1", "tools": []any{}})
	if e != nil {
		return nil, &jobs.Error{Code: "invalid_ai_input", Retryable: false}
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if e != nil {
		return nil, &jobs.Error{Code: "provider_request", Retryable: false}
	}
	req.Header.Set("Content-Type", "application/json")
	return p.call(req, request.IdempotencyKey, 256<<10)
}

// call — вызов выполняет ограниченный запрос к настроенному провайдеру и возвращает только безопасные
// классификации ошибок.
//
// @Parameters:
//   - request: Входящий запрос HTTP.
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//   - maximum: Максимально допустимый размер или количество символов.
//
// @returns:
//   - result1: Собранные байтовые значения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (p *provider) call(request *http.Request, key string, maximum int64) ([]byte, error) {
	request.Header.Set("Authorization", "Bearer "+p.token)
	request.Header.Set("Idempotency-Key", key)
	response, e := p.client.Do(request)
	if e != nil {
		return nil, &jobs.Error{Code: "provider_unavailable", Retryable: true}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		retry := response.StatusCode == 429 || response.StatusCode == 408 || response.StatusCode >= 500
		code := "provider_rejected"
		if response.StatusCode == 429 {
			code = "provider_rate_limited"
		}
		return nil, &jobs.Error{Code: code, Retryable: retry, RetryAfter: retryAfter(response.Header.Get("Retry-After"))}
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if e != nil {
		return nil, &jobs.Error{Code: "provider_unavailable", Retryable: true}
	}
	if int64(len(data)) > maximum {
		return nil, &jobs.Error{Code: "provider_response_limit", Retryable: false}
	}
	return data, nil
}

// retryAfter интерпретирует задержку повторных попыток провайдера, не допуская неограниченного
// ожидания.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Продолжительность или ограниченный интервал ожидания, вычисленный в ходе
//     операции.
func retryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, e := strconv.Atoi(value); e == nil {
		return max(0, min(time.Duration(seconds)*time.Second, time.Hour))
	}
	if at, e := http.ParseTime(value); e == nil {
		return max(0, min(time.Until(at), time.Hour))
	}
	return 0
}

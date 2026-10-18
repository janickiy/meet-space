package liveproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/media"
	"io"
	"net/http"
	"strings"
	"time"
)

// Registry — реестр извлекает адрес текущего владельца комнаты только из доверенного внутреннего реестра.
type Registry interface {
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
}

// Tap открывает отдельный поток HTTP без проксирования медиа через API или RabbitMQ.
type Tap struct {
	Registry Registry
	Secret   string
	Client   *http.Client
}

// NewTap создает ответвление с зависимостями и ограничениями, необходимыми для провайдеров
// прямой речи.
//
// @Parameters:
//   - registry: Реестр зависимостей или метрик, используемый для поиска или регистрации
//     компонента.
//   - secret: Материал для создания закрытого ключа или необработанный секретный ключ; его
//     нельзя регистрировать или раскрывать.
//
// @returns:
//   - result1: Созданное или загруженное ответвление.
func NewTap(registry Registry, secret string) *Tap {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 5 * time.Second
	transport.MaxConnsPerHost = 32
	return &Tap{registry, secret, &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Open открывает доступ к медиафайлам, записям или сеансам, необходимым для работы этого
// компонента.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Результирующий считывающий модуль.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (t *Tap) Open(ctx context.Context, cid, id string) (io.ReadCloser, error) {
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	route, err := t.Registry.GetOwner(op, cid)
	cancel()
	if err != nil || config.ValidateMediaEndpoint(route.Endpoint) != nil {
		return nil, media.ErrUnavailable
	}
	command := media.EgressRequest{Purpose: "captions", RequestID: uuid.NewString(), ConferenceID: cid, RecordingID: id, Route: route, SegmentDurationSec: 5}
	raw, _ := json.Marshal(command)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(route.Endpoint, "/")+"/internal/media/egress", bytes.NewReader(raw))
	if err != nil {
		return nil, media.ErrUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+t.Secret)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", command.RequestID)
	response, err := t.Client.Do(request)
	if err != nil {
		return nil, media.ErrUnavailable
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, media.ErrUnavailable
	}
	return response.Body, nil
}

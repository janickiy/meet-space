package engagementapp

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/ratelimit"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	usecase "github.com/janickiy/meet-space/internal/usecase/realtime"
)

// Лимитер определяет заменяемый контракт зависимости для временных реакций на конференции.
type Limiter interface {
	// Allow оценивает ограничение скорости и возвращает оставшийся бюджет и границу сброса.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: хранилище, кэш или криптографический ключ, используемый операцией.
	//   - аргумент 3: максимальное количество результатов или операций в этом пакете.
	//   - аргумент 4: продолжительность ограничения скорости или окна наблюдения.
	//
	// Возвращаемые значения:
	//   - результат 1: полученный результат.
	//   - результат 2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения.
	Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error)
}

// Обработчик адаптирует HTTP-запросы к временным реакциям конференции, проверяет входные данные и формирует ответ.
type Handler struct {
	service   *usecase.Engagement
	limiter   Limiter
	namespace string
}

// NewHandler создает обработчик с зависимостями и ограничениями, необходимыми для временных
// реакций на конференциях.
//
// Параметры:
//   - service: служба приложения, вызываемая этим адаптером.
//   - limiter: общий ограничитель частоты запросов.
//   - namespace: пространство имен, изолирующее распределенные ключи и каналы этого компонента.
//
// Возвращает:
//   - result 1: созданный или загруженный обработчик.
func NewHandler(service *usecase.Engagement, limiter Limiter, namespace string) *Handler {
	return &Handler{service: service, limiter: limiter, namespace: namespace}
}

// id проверяет UUID ресурса из HTTP-маршрута перед выполнением операции в приложении или базе данных.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//
// Возвращаемые значения:
//   - result 1: проверенный UUID ресурса.
//   - result 2: выполняется ли задокументированное условие операции или произошло ли успешное изменение условного состояния.
func id(c *gin.Context) (string, bool) {
	v, err := uuid.Parse(c.Param("id"))
	if err != nil || v == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", false
	}
	return v.String(), true
}

// limit проверяет независимые бюджеты на реакцию для каждого пользователя и для каждой конференции.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - conferenceID: идентификатор конференции, ограничивающий область действия операции.
//   - kind: валидированное событие, задание, медиафайл или категория ресурса.
//   - n: количество запрошенных элементов или операций.
//   - window: продолжительность ограничения частоты запросов или окна наблюдения.
//
// Возвращаемые значения:
//   - result 1: выполняется ли задокументированное условие операции или ее условное состояние
//     изменилось успешно.
func (h *Handler) limit(c *gin.Context, conferenceID, kind string, n int, window time.Duration) bool {
	if h.limiter == nil {
		return true
	}
	keys := []string{h.namespace + ":engagement:" + kind + ":user:" + httpmiddleware.UserID(c), h.namespace + ":engagement:" + kind + ":conference:" + conferenceID}
	for i, key := range keys {
		count := n
		if i == 1 {
			// An unauthorized account must not consume another conference's shared reaction budget.
			if err := h.service.Authorize(c.Request.Context(), conferenceID, httpmiddleware.UserID(c)); err != nil {
				httpresponse.Fail(c, err)
				return false
			}
			count = 30
			window = time.Second
		}
		result, err := h.limiter.Allow(c.Request.Context(), key, count, window)
		if err != nil {
			httpresponse.Fail(c, err)
			return false
		}
		if !result.Allowed {
			c.Header("Retry-After", "10")
			c.AbortWithStatusJSON(429, gin.H{"error": "too many " + kind})
			return false
		}
	}
	return true
}

// Reaction публикует одобренную временную рецензию от имени участника конференции.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Reaction(c *gin.Context) {
	conf, ok := id(c)
	if !ok {
		return
	}
	var request struct {
		Emoji string `json:"emoji"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if !h.limit(c, conf, "reactions", 5, 10*time.Second) {
		return
	}
	if err := h.service.Reaction(c.Request.Context(), conf, httpmiddleware.UserID(c), request.Emoji); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success"})
}

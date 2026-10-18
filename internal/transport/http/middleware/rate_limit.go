package httpmiddleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/ratelimit"
)

// Limiter определяет заменяемый контракт зависимости для HTTP.
type Limiter interface {
	// Функция Allow оценивает ограничение скорости и возвращает оставшийся бюджет и границу
	// сброса.
	//
	// @Parameters:
	//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
	//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
	//   - limit: Максимальное количество результатов или операций в этом пакете.
	//   - window: Продолжительность действия ограничения скорости или окна наблюдения.
	//
	// @returns:
	//   - result1: Результирующий результат.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (ratelimit.Result, error)
}

// KeyFunc формирует идентификационные данные клиента для одного правила ограничения скорости,
// контролируемого сервером.
type KeyFunc func(c *gin.Context) string

// Rule — правило определяет отдельный бюджет скорости, область действия, идентификационный ключ и
// временное окно.
type Rule struct {
	Method string
	Path   string
	Scope  string
	Limit  int
	Window time.Duration
	Key    KeyFunc
}

// RateLimitConfig содержит правила ограничения скорости и политику обработки сбоев
// ограничителя.
type RateLimitConfig struct {
	Enabled bool
	Rules   []Rule
}

// RateLimit создает промежуточное ПО Gin, которое обеспечивает соблюдение заданного общего
// бюджета запросов.
//
// @Parameters:
//   - limiter: Общий ограничитель частоты запросов.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//
// @returns:
//   - result1: Обработчик HTTP для регистрации этой операции.
func RateLimit(limiter Limiter, cfg RateLimitConfig) gin.HandlerFunc {
	rulesByRoute := make(map[string][]Rule)
	for _, rule := range cfg.Rules {
		if rule.Limit <= 0 || rule.Key == nil {
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(rule.Method))
		path := strings.TrimSpace(rule.Path)
		if method == "" || path == "" {
			continue
		}
		rule.Method = method
		rule.Path = path
		rule.Scope = strings.TrimSpace(rule.Scope)
		if rule.Scope == "" {
			rule.Scope = "default"
		}
		if rule.Window <= 0 {
			rule.Window = time.Minute
		}
		routeKey := method + " " + path
		rulesByRoute[routeKey] = append(rulesByRoute[routeKey], rule)
	}

	return func(c *gin.Context) {
		if !cfg.Enabled || limiter == nil {
			c.Next()
			return
		}
		routeRules := rulesByRoute[c.Request.Method+" "+c.FullPath()]
		if len(routeRules) == 0 {
			c.Next()
			return
		}

		for _, rule := range routeRules {
			identity := strings.TrimSpace(rule.Key(c))
			if identity == "" {
				continue
			}
			result, err := limiter.Allow(c.Request.Context(), redisRateLimitKey(rule, identity), rule.Limit, rule.Window)
			if err != nil {
				abortRateLimitError(c, http.StatusInternalServerError, "rate limit unavailable")
				return
			}
			if !result.Allowed {
				abortRateLimitExceeded(c, result)
				return
			}
			setRateLimitHeaders(c, result)
		}

		c.Next()
	}
}

// ClientIPKey создает объект ограничения скорости на основе доверенного разрешения клиентского
// адреса.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
//
// @returns:
//   - result1: Идентификатор ограничения скорости, полученный на основе доверенного
//     клиентского адреса.
func ClientIPKey(c *gin.Context) string {
	return c.ClientIP()
}

// redisRateLimitKey создает ключ Redis с пространством имен без указания неограниченного
// значения запроса.
//
// @Parameters:
//   - rule: Политика ограничения скорости запросов, контролируемая сервером.
//   - identity: Подтвержденный аккаунт и идентификатор физического сеанса.
//
// @returns:
//   - result1: Ключ сегмента ограничения скорости с пространством имен.
func redisRateLimitKey(rule Rule, identity string) string {
	return strings.Join([]string{
		"rate",
		safeKeyPart(rule.Scope),
		safeKeyPart(identity),
		safeKeyPart(rule.Method),
		safeKeyPart(rule.Path),
	}, ":")
}

// safeKeyPart ограничивает и нормализует значение перед использованием его в качестве
// компонента ключа ограничения скорости.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Компонент безопасного ограниченного ключа.
func safeKeyPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", ".")

	return value
}

// abortRateLimitExceeded записывает ответ с ограничением скорости и останавливает цепочку
// обработчиков Gin.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
//   - result: Подтвержденный результат обработки или операции.
func abortRateLimitExceeded(c *gin.Context, result ratelimit.Result) {
	setRateLimitHeaders(c, result)
	if result.RetryAfter > 0 {
		c.Header("Retry-After", strconv.FormatInt(int64(result.RetryAfter.Seconds()), 10))
	}
	httpresponse.Fail(c, apperrors.ErrRateLimited)
}

// abortRateLimitError записывает ответ об ошибке безопасного ограничения и останавливает
// обработку запроса.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
//   - status: Состояние жизненного цикла или статус ответа на запись.
//   - message: Ограниченный текст сообщения, используемый в этой операции; не указывайте личные
//     учетные данные.
func abortRateLimitError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"status":  "failed",
		"message": message,
	})
}

// setRateLimitHeaders добавляет в ответ принятые данные о бюджете ограничения скорости и
// метаданные сброса.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
//   - result: Подтвержденный результат обработки или операции.
func setRateLimitHeaders(c *gin.Context, result ratelimit.Result) {
	if result.Limit > 0 {
		c.Header("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	}
	if result.Remaining >= 0 {
		c.Header("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	}
	if !result.ResetAt.IsZero() {
		c.Header("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
	}
}

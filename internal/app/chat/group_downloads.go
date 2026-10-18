package chatapp

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// GroupDownloads заменяет URL-адреса с поддержкой bearer-capability только для групп.
// Возвращаемый // URL-адрес требует повторной аутентификации при запросе его байтов.
type GroupDownloads interface {
	// GroupDownload авторизует групповое вложение и возвращает целевое значение для частной загрузки с ограниченным доступом.
	//
	// Параметры:
	// - аргумент 1: Контекст, управляющий отменой, крайними сроками и сроком действия операции.
	// - аргумент 2: Действующий идентификатор учетной записи, используемый для расширения доступа.
	// - аргумент 3: Идентификатор диалога, ограничивающий область действия операции.
	// - аргумент 4: Идентификатор частного вложения или метаданные, определяемые его типом.
	//
	// Возвращает:
	// - результат 1: Выполняется ли задокументированное условие операции или ее условное состояние
	// успешно изменено.
	// - результат 2: URL-адрес для загрузки временного разрешенного вложения.
	// - результат 3: Временная метка или граница действия, связанная с результатом.
	// - результат 4: вероятность успеха равна нулю; в противном случае проверка, доступ, транспортировка или сохранение не будут выполнены
	// ошибка.
	GroupDownload(context.Context, string, string, string) (bool, string, time.Time, error)
}

// WithGroupDownloads подключает групповые загрузки к компоненту до того, как будут выполнены его операции.
//
// Параметры:
//   - provider: Настроенный адаптер внешнего поставщика.
//
// Возвращаемое значение:
//   - result 1: Созданный или загруженный обработчик.
func (h *Handler) WithGroupDownloads(provider GroupDownloads) *Handler {
	h.groupDownloads = provider
	return h
}

// downloadGroup указывает путь для загрузки групповых вложений, если данный репозиторий принадлежит
// группе.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - scope: авторизованная область диалога или ресурса.
//   - attachment: личные данные о вложениях или метаданные, в зависимости от их типа.
//
// Возвращает:
//   - result 1: выполняется ли задокументированное условие операции или ее условное состояние
//     изменилось успешно.
func (h *Handler) downloadGroup(c *gin.Context, scope, attachment string) bool {
	if h.namespace != "conversations" || h.groupDownloads == nil {
		return false
	}
	handled, url, expires, err := h.groupDownloads.GroupDownload(c.Request.Context(), middleware.UserID(c), scope, attachment)
	if !handled {
		return false
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return true
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"status": "success", "url": url, "expiresAt": expires, "authenticated": true})
	return true
}

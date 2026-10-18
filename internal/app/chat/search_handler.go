package chatapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/chat"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type SearchService interface {
	// Поиск постоянных сообщений в чате и личных вложений с использованием ограниченного ввода и фильтров видимости вызывающей стороны.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - аргумент 2: действующий идентификатор аккаунта, используемый для ограничения доступа.
	//   - аргумент 3: идентификатор конференции или беседы, ограничивающий поиск.
	//   - аргумент 4: текст для поиска или проверенные фильтры запросов.
	//   - аргумент 5: непрозрачная граница продолжения с предыдущей страницы.
	//   - аргумент 6: максимальное количество результатов или операций в этом пакете.
	//
	// Возвращает:
	//   - результат 1: результирующая страница.
	//   - результат 2: nil в случае успеха; в противном случае ошибка валидации, доступа, передачи данных или сохранения
	//     данных.
	Search(context.Context, string, string, string, string, int) (chat.Page, error)
	// Контекст возвращает видимые сообщения, окружающие выбранное сообщение, чтобы клиенты могли перейти к
	// результату поиска.
	//
	// Параметры:
	//   - аргумент 1: Контекст, управляющий отменой, сроками и временем жизни операции.
	//   - аргумент 2: Действующий идентификатор аккаунта, используемый для ограничения доступа.
	//   - аргумент 3: Идентификатор конференции или беседы, в которой находится выбранное сообщение.
	//   - аргумент 4: Идентификатор сообщения в авторизованной беседе.
	//
	// Возвращает:
	//   - результат 1: Полученная страница.
	//   - результат 2: nil в случае успеха; в противном случае — ошибка валидации, доступа, транспорта или сохранения
	//     данных.
	Context(context.Context, string, string, string) (chat.Page, error)
}

type SearchHandler struct{ Service SearchService }

// Поиск постоянных сообщений чата и личных вложений с использованием ограниченного ввода и фильтров видимости вызывающей стороны.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *SearchHandler) Search(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit, ok := actionPage(c)
	if !ok {
		return
	}
	page, err := h.Service.Search(c.Request.Context(), httpmiddleware.UserID(c), id, c.Query("q"), c.Query("before"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	searchPageResponse(c, page)
}

// Context возвращает видимые сообщения, окружающие выбранное сообщение, чтобы клиенты могли перейти к результату поиска.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *SearchHandler) Context(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	messageID, ok := parameter(c, "messageId")
	if !ok {
		return
	}
	page, err := h.Service.Context(c.Request.Context(), httpmiddleware.UserID(c), id, messageID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	searchPageResponse(c, page)
}

// searchPageResponse записывает страницу с результатами поиска сообщений с метаданными ответа, ожидаемыми клиентом.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - page: страница с результатами для форматирования или обработки.
func searchPageResponse(c *gin.Context, page chat.Page) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor, "unreadCount": page.UnreadCount, "lastReadMessageId": page.LastReadMessageID})
}

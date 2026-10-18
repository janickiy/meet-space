package chatapp

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	chatusecase "github.com/janickiy/meet-space/internal/usecase/chat"
)

type ActionHandler struct{ Service *chatusecase.ActionService }

// actionPage анализирует и проверяет размер страницы, используемый в конечных точках действий чата.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//
// @returns:
//   - result1: итоговое количество, размер или числовое значение.
//   - result2: выполняется ли задокументированное условие операции или ее условное состояние
//     изменено успешно.
func actionPage(c *gin.Context) (int, bool) {
	limit := 20
	if value, present := c.GetQuery("limit"); present {
		var err error
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 50 {
			httpresponse.Fail(c, apperrors.New(apperrors.ErrInvalidInput, "limit must be between 1 and 50"))
			return 0, false
		}
	}
	return limit, true
}

// Info возвращает информацию о чате конкретного участника, доступную для текущего аккаунта.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ActionHandler) Info(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	item, err := h.Service.Info(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Members возвращает текущие идентификаторы участников или их представления для авторизованного диалога.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ActionHandler) Members(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit := 50
	if value, present := c.GetQuery("limit"); present {
		var err error
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			httpresponse.Fail(c, apperrors.New(apperrors.ErrInvalidInput, "limit must be between 1 and 100"))
			return
		}
	}
	items, next, err := h.Service.Members(c.Request.Context(), httpmiddleware.UserID(c), id, c.Query("after"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items, "nextCursor": next})
}

// UpdateInfo обновляет валидированный заголовок или описание чата после проверки разрешения на управление
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ActionHandler) UpdateInfo(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.UpdateInfoRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.Service.UpdateInfo(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Preferences считывает сохраненные настройки доставки или переписки действующего аккаунта.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ActionHandler) Preferences(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	item, err := h.Service.Preferences(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// SetPreferences сохраняет подтвержденные настройки диалога действующей учетной записи.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ActionHandler) SetPreferences(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request struct {
		NotificationsEnabled *bool `json:"notificationsEnabled"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if request.NotificationsEnabled == nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	item, err := h.Service.SetPreferences(c.Request.Context(), httpmiddleware.UserID(c), id, *request.NotificationsEnabled)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// При выходе из системы удаляется активное членство или подключение к медиафайлам с сохранением сохраненной истории
// диалога или конференции.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ActionHandler) Leave(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	if err := h.Service.Leave(c.Request.Context(), httpmiddleware.UserID(c), id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// Поиск постоянных сообщений чата и личных вложений с использованием ограниченного ввода и фильтров видимости вызывающей стороны.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ActionHandler) Search(c *gin.Context) {
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
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor})
}

// Context возвращает видимые сообщения, окружающие выбранное сообщение, чтобы клиенты могли перейти к результату поиска.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ActionHandler) Context(c *gin.Context) {
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
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor, "unreadCount": page.UnreadCount, "lastReadMessageId": page.LastReadMessageID})
}

// Materials возвращают ограниченную страницу с видимыми вложениями из диалогов и общими ссылками.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ActionHandler) Materials(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit, ok := actionPage(c)
	if !ok {
		return
	}
	page, err := h.Service.Materials(c.Request.Context(), httpmiddleware.UserID(c), id, c.Query("kind"), c.Query("before"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor})
}

// Pins возвращает ограниченную страницу закрепленных сообщений в беседе.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ActionHandler) Pins(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit, ok := actionPage(c)
	if !ok {
		return
	}
	page, err := h.Service.Pins(c.Request.Context(), httpmiddleware.UserID(c), id, c.Query("before"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor})
}

// SetPin обновляет закрепленные сообщения в чате и личные вложения.
//
// @Parameters:
//   - important: нужно ли закрепить сообщение или пометить его как важное.
//
// @returns:
//   - result1: HTTP-обработчик для регистрации этой операции.
func (h *ActionHandler) SetPin(important bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parameter(c, "id")
		if !ok {
			return
		}
		messageID, ok := parameter(c, "messageId")
		if !ok {
			return
		}
		if err := h.Service.SetPin(c.Request.Context(), httpmiddleware.UserID(c), id, messageID, important); err != nil {
			httpresponse.Fail(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	}
}

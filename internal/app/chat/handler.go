package chatapp

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	chatusecase "github.com/janickiy/meet-space/internal/usecase/chat"
)

// Обработчик адаптирует HTTP-запросы к постоянным сообщениям в чате и личным вложениям, проверяет входные данные и формирует ответ.
type Handler struct {
	service        *chatusecase.Service
	namespace      string
	groupDownloads GroupDownloads
}

// NewHandler создает обработчик с зависимостями и ограничениями, необходимыми для постоянных сообщений в чате и приватных вложений.
//
// Параметры:
//   - service: служба приложения, вызываемая этим адаптером.
//
// Возвращает:
//   - result 1: созданный или загруженный обработчик.
func NewHandler(service *chatusecase.Service) *Handler {
	return &Handler{service: service, namespace: "conferences"}
}

// Параметр проверяет наличие обязательного параметра маршрута перед вызовом операции приложения.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - key: хранилище, кэш или криптографический ключ, используемый операцией.
//
// Возвращаемые значения:
//   - result 1: проверенный обязательный параметр маршрута.
//   - result 2: выполняется ли задокументированное условие операции или ее условное состояние
//     изменено успешно.
func parameter(c *gin.Context, key string) (string, bool) {
	id, err := chat.UUID(c.Param(key))
	if err != nil {
		httpresponse.Fail(c, err)
		return "", false
	}
	return id, true
}

// List возвращает ограниченную страницу записей о сообщениях в чате с использованием заданных параметров разбивки на страницы и фильтров.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) List(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit := 50
	if raw, exists := c.GetQuery("limit"); exists {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			httpresponse.Fail(c, apperrors.New(apperrors.ErrInvalidInput, "limit must be between 1 and 100"))
			return
		}
	}
	page, err := h.service.List(c.Request.Context(), httpmiddleware.UserID(c), id, c.Query("before"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor, "unreadCount": page.UnreadCount, "lastReadMessageId": page.LastReadMessageID})
}

// Send проверяет ссылки на доступ, ответ и вложение и сохраняет сообщение в чате с
// защитой от идемпотентности при повторных попытках.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Send(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.SendRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, created, err := h.service.Send(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"status": "success", "item": item})
}

// При редактировании существующий текст сообщения автора изменяется после проверки видимости и возможности редактирования.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Edit(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	messageID, ok := parameter(c, "messageId")
	if !ok {
		return
	}
	var request chat.EditRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.Edit(c.Request.Context(), httpmiddleware.UserID(c), id, messageID, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Delete удаляет запрошенное сообщение в чате, используя область видимости вызывающей стороны и жизненный цикл ресурса
// rules.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	messageID, ok := parameter(c, "messageId")
	if !ok {
		return
	}
	item, err := h.service.Delete(c.Request.Context(), httpmiddleware.UserID(c), id, messageID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// ReadState возвращает сохраненную границу прочитанных сообщений участника и количество видимых непрочитанных сообщений.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) ReadState(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	item, err := h.service.ReadState(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// MarkRead расширяет сохраненную границу чтения, не допуская перемещения запоздалых или повторяющихся запросов
// все задом наперед.
//
// Параметры:
// - c: Контекст запроса Gin и ответа, включая аутентифицированный идентификатор.
func (h *Handler) MarkRead(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.ReadRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.MarkRead(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// InitAttachment создает или повторно использует ожидающие обработки метаданные вложения, чтобы можно было повторить попытку загрузки
// безопасно.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) InitAttachment(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.InitRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, created, err := h.service.InitAttachment(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"status": "success", "item": item, "uploadUrl": "/api/v1/" + h.namespace + "/" + id + "/attachments/" + item.ID + "/content"})
}

// Upload checks the limited data flow and saves it for the current upload
// attempt.
//
// Parameters:
//   - c: the Gin request and response context, including authenticated data.
func (h *Handler) Upload(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	attachmentID, ok := parameter(c, "attachmentId")
	if !ok {
		return
	}
	// Ограничение на количество медленных клиентов и размер входных данных. Загрузка осуществляется по отдельному пути, не связанному с сигнализацией и модерацией;
	// сервис также ограничивает количество одновременных загрузок из памяти.
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(60 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, chat.MaxAttachmentBytes+1)
	item, err := h.service.Upload(c.Request.Context(), httpmiddleware.UserID(c), id, attachmentID, c.Request.Body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// FinalizeAttachment помечает успешно загруженное вложение как готовое к использованию в сообщении.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) FinalizeAttachment(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	attachmentID, ok := parameter(c, "attachmentId")
	if !ok {
		return
	}
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.FinalizeAttachment(c.Request.Context(), httpmiddleware.UserID(c), id, attachmentID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Download проверяет видимость вложения и возвращает временный приватный URL для скачивания.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Download(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	attachmentID, ok := parameter(c, "attachmentId")
	if !ok {
		return
	}
	if h.downloadGroup(c, id, attachmentID) {
		return
	}
	url, expires, err := h.service.Download(c.Request.Context(), httpmiddleware.UserID(c), id, attachmentID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"status": "success", "url": url, "expiresAt": expires})
}

// ForConversations preserves the shared HTTP/file flow with a distinct public scope.
//
// Returns:
//   - result 1: The constructed or loaded handler.
func (h *Handler) ForConversations() *Handler { h.namespace = "conversations"; return h }

package personalapp

import (
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/chat"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	personalusecase "github.com/janickiy/meet-space/internal/usecase/personal"
)

type AssetHandler struct{ service *personalusecase.AssetService }

// NewAssetHandler создает обработчик ресурсов с зависимостями и ограничениями, необходимыми для // личных и групповых чатов.
//
// Параметры:
//   - service: служба приложения, вызываемая этим адаптером.
//
// Возвращает:
//   - result 1: созданный или загруженный обработчик ресурсов.
func NewAssetHandler(service *personalusecase.AssetService) *AssetHandler {
	return &AssetHandler{service: service}
}

// assetParameter проверяет идентификатор частного актива из маршрута перед обращением к хранилищу.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - name: имя настраиваемого параметра, файла cookie, ресурса или провайдера.
//
// Возвращаемые значения:
//   - result 1: проверенный идентификатор частного актива.
//   - result 2: выполняется ли задокументированное условие операции или ее условное состояние
//     изменено успешно.
func assetParameter(c *gin.Context, name string) (string, bool) {
	id, err := chat.UUID(c.Param(name))
	if err != nil {
		httpresponse.Fail(c, err)
		return "", false
	}
	return id, true
}

// PutAvatar проверяет и сохраняет нормализованный групповой аватар, а затем публикует зафиксированное изменение в диалоге.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *AssetHandler) PutAvatar(c *gin.Context) {
	id, ok := assetParameter(c, "id")
	if !ok {
		return
	}
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(personalusecase.AssetStreamTimeout))
	defer controller.SetReadDeadline(time.Time{})
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, personalusecase.MaxAvatarBytes+1)
	item, err := h.service.PutAvatar(c.Request.Context(), middleware.UserID(c), id, c.GetHeader("Content-Type"), c.Request.Body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// DeleteAvatar очищает аватар авторизованной группы и планирует удаление устаревших приватных объектов для
// очистки.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *AssetHandler) DeleteAvatar(c *gin.Context) {
	id, ok := assetParameter(c, "id")
	if !ok {
		return
	}
	item, err := h.service.DeleteAvatar(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// AvatarContent предоставляет аватар для закрытой группы только после проверки членства запрашивающей учетной записи.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *AssetHandler) AvatarContent(c *gin.Context) {
	id, ok := assetParameter(c, "id")
	if !ok {
		return
	}
	h.content(c, id, "")
}

// AttachmentContent serves a visible group-message attachment through the authorized content
// endpoint.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *AssetHandler) AttachmentContent(c *gin.Context) {
	id, ok := assetParameter(c, "id")
	if !ok {
		return
	}
	attachment, ok := assetParameter(c, "attachmentId")
	if !ok {
		return
	}
	h.content(c, id, attachment)
}

// content открывает авторизованный приватный аватар или вложение и передает его через HTTP
// ответ.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
//   - id: идентификатор ресурса, на который указывает эта операция.
//   - attachment: идентификатор приватного вложения или метаданные в соответствии с его типом.
func (h *AssetHandler) content(c *gin.Context, id, attachment string) {
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetWriteDeadline(time.Now().Add(personalusecase.AssetStreamTimeout))
	defer controller.SetWriteDeadline(time.Time{})
	err := h.service.Stream(c.Request.Context(), middleware.UserID(c), id, attachment, func(asset personalusecase.Asset, reader io.Reader) error {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Length", strconv.FormatInt(asset.Size, 10))
		if attachment == "" {
			c.Header("Content-Type", asset.ContentType)
		} else {
			c.Header("Content-Type", "application/octet-stream")
			c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": asset.Filename}))
		}
		c.Status(http.StatusOK)
		_, err := io.CopyN(c.Writer, reader, asset.Size)
		return err
	})
	if err != nil {
		if !c.Writer.Written() {
			httpresponse.Fail(c, err)
		} else {
			c.Abort()
		}
	}
}

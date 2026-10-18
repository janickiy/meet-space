// Пакет platformapp предоставляет безопасные возможности продукта и операционные возможности только для администраторов.
package platformapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	domain "github.com/janickiy/meet-space/internal/domain/platform"
)

type Service interface {
	// Capabilities возвращает возможности продукта или интеграции, фактически доступные оператору.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//
	// @returns:
	//   - result1: полученные возможности.
	Capabilities(context.Context) domain.Capabilities
	// Summary возвращает ограниченные операционные агрегаты, доступные только администратору, и безопасные коды ошибок
	// без пользовательского контента.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//
	// @returns:
	//   - result1: итоговый сводный отчет.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения
	//     данных.
	Summary(context.Context) (domain.Summary, error)
}

type Handler struct {
	Service      Service
	BuildVersion string
}

// Capabilities возвращает возможности продукта или интеграции, фактически доступные оператору.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Capabilities(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "capabilities": h.Service.Capabilities(c.Request.Context()), "buildVersion": h.BuildVersion})
}

// Summary возвращает ограниченные служебные данные только для администраторов и безопасные коды ошибок
// без пользовательского контента.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Summary(c *gin.Context) {
	value, err := h.Service.Summary(c.Request.Context())
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": value})
}

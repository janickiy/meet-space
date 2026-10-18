package conferencesapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	d "github.com/janickiy/meet-space/internal/domain/conferences"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type InvitationService interface {
	// Search Поиск по жизненному циклу конференции и членству участников с использованием ограниченных входных данных и фильтров видимости вызывающей стороны.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - аргумент 2: идентификатор аккаунта, выполняющего операцию.
	//   - аргумент 3: идентификатор конференции, ограничивающий область действия операции.
	//   - аргумент 4: текст поиска или проверенные фильтры запроса.
	//
	// Возвращаемые значения:
	//   - результат 1: собранные значения пользователей приглашения.
	//   - результат 2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения.  //
	Search(context.Context, string, string, string) ([]d.InvitationUser, error)
	// Invite создает или обновляет приглашения на конференцию для проверенных получателей.
	//
	// Параметры:
	//   - аргумент 1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - аргумент 2: идентификатор учетной записи, выполняющей операцию.
	//   - аргумент 3: идентификатор конференции, ограничивающий область действия операции.
	//   - аргумент 4: проверенный запрос приложения или входящие параметры операции.
	//
	// Возвращаемые значения:
	//   - результат 1: собранные значения результатов приглашения.
	//   - результат 2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения данных.
	//
	Invite(context.Context, string, string, d.InvitationRequest) ([]d.InvitationResult, error)
}
type InvitationHandler struct{ Service InvitationService }

// Search по жизненному циклу конференции и членству участников с использованием ограниченных входных данных и фильтров видимости вызывающей стороны.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *InvitationHandler) Search(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	items, err := h.Service.Search(c.Request.Context(), middleware.UserID(c), id, c.Query("query"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

// Invite создает или обновляет приглашения на конференцию для проверенных получателей.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *InvitationHandler) Invite(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	var request d.InvitationRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	items, err := h.Service.Invite(c.Request.Context(), middleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "items": items})
}

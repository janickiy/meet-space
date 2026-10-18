package conferencesapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// ControlService определяет заменяемый контракт зависимости для управления жизненным циклом конференции и
// членством участников.
type ControlService interface {
	// Moderate applies a participant moderation action after checking the actor's role and the
	// target restrictions.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Conference participant identifier.
	//   - argument 5: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The resulting participant view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Moderate(context.Context, string, string, string, conferences.ModerationRequest) (conferences.ParticipantView, error)
	// UpdateMediaState saves the participant's declared media-source state.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Conference identifier limiting the operation scope.
	//   - argument 4: Current state or validated state update.
	//
	// Returns:
	//   - result 1: The resulting participant view.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	UpdateMediaState(context.Context, string, string, conferences.MediaState) (conferences.ParticipantView, error)
}

// ControlHandler адаптирует модерацию участников и объявленные изменения состояния мультимедиа для HTTP-запросов с аутентификацией.
type ControlHandler struct{ service ControlService }

// NewControlHandler создает обработчик управления с зависимостями и ограничениями, необходимыми для
// жизненного цикла конференции и управления членством участников.
//
// Параметры:
//   - service: служба приложения, вызываемая этим адаптером.
//
// Возвращает:
//   - result 1: созданный или загруженный обработчик управления.
func NewControlHandler(service ControlService) *ControlHandler {
	return &ControlHandler{service: service}
}

// Moderate применяет действие по модерации участников после проверки роли пользователя и целевых ограничений.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ControlHandler) Moderate(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	participant, err := uuid.Parse(c.Param("participantId"))
	if err != nil || participant == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	var request conferences.ModerationRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.Moderate(c.Request.Context(), httpmiddleware.UserID(c), id, participant.String(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Media принимает данные о состоянии микрофона, камеры и экрана текущего участника.
//
// Параметры:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *ControlHandler) Media(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	var request conferences.MediaState
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.UpdateMediaState(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

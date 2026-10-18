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
	// Moderate применяет действие по модерации участников после проверки роли субъекта и
	// целевых ограничений.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор участника конференции.
	//   - arg5: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Полученное представление участника.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Moderate(context.Context, string, string, string, conferences.ModerationRequest) (conferences.ParticipantView, error)
	// UpdateMediaState сохраняет заявленное участником состояние источника мультимедиа.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Текущее состояние или обновлённое валидированное состояние.
	//
	// @returns:
	//   - result1: Полученное представление участника.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	UpdateMediaState(context.Context, string, string, conferences.MediaState) (conferences.ParticipantView, error)
}

// ControlHandler адаптирует модерацию участников и объявленные изменения состояния мультимедиа для HTTP-запросов с аутентификацией.
type ControlHandler struct{ service ControlService }

// NewControlHandler создает обработчик управления с зависимостями и ограничениями, необходимыми для
// жизненного цикла конференции и управления членством участников.
//
// @Parameters:
//   - service: служба приложения, вызываемая этим адаптером.
//
// @returns:
//   - result1: созданный или загруженный обработчик управления.
func NewControlHandler(service ControlService) *ControlHandler {
	return &ControlHandler{service: service}
}

// Moderate применяет действие по модерации участников после проверки роли пользователя и целевых ограничений.
//
// @Parameters:
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
// @Parameters:
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

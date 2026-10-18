// Пакет captionsapp подключает настройки субтитров в реальном времени к аутентифицированным
// конечным точкам HTTP.
package captionsapp

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/config"
	analytics "github.com/janickiy/meet-space/internal/domain/analytics"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	live "github.com/janickiy/meet-space/internal/usecase/captions"
)

// Обработчик возвращает сохраненные финальные субтитры и настройки без открытия сеанса речевого провайдера в API.
type Handler struct {
	Repo      live.Repository
	Enabled   bool
	Analytics interface {
		// Read загружает сохраненные данные технических конференций после проверки истории аккаунта
		// доступ.
		//
		// @Parameters:
		//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
		//   - arg2: действующий идентификатор аккаунта, используемый для ограничения доступа.
		//   - arg3: идентификатор конференции, ограничивающий область действия операции.
		//
		// @returns:
		//   - result1: полученная конференция.
		//   - result2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, передачи или сохранения
		//     данных.
		Read(context.Context, string, string) (analytics.Conference, error)
	}
	Search interface {
		// Reindex переиндексация очередей с ограниченной переиндексацией транскрипта после проверки прав доступа субъекта.
		//
		// @Parameters:
		//   - arg1: контекст, управляющий отменой, сроками и временем выполнения операции.
		//   - arg2: идентификатор действующего аккаунта, используемый для ограничения доступа.
		//   - arg3: идентификатор конференции, ограничивающий область действия операции.
		//   - arg4: идентификатор записи, используемый текущей операцией.
		//   - arg5: конфигурация и ограничения обработки, контролируемые оператором.
		//   - arg6: минимальный интервал между дорогостоящими запросами на повторную обработку.
		//
		// @returns:
		//   - result1: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспорта или сохранения. 		//
		Reindex(context.Context, string, string, string, config.StageEightConfig, time.Duration) error
	}
	Config config.StageEightConfig
}

// ReadAnalytics возвращает сохраненные данные технической конференции только допущенным
// участникам.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
func (h *Handler) ReadAnalytics(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	value, err := h.Analytics.Read(c.Request.Context(), middleware.UserID(c), cid)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": value})
}

// Reindex переиндексирует очереди с помощью ограниченной транскрипт-реиндексации после проверки прав доступа актора.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Reindex(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	rid, err := uuid.Parse(c.Param("recordingId"))
	if err != nil || rid == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	err = h.Search.Reindex(c.Request.Context(), middleware.UserID(c), cid, rid.String(), h.Config, 5*time.Minute)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success"})
}

// id проверяет UUID ресурса из HTTP-маршрута перед выполнением операции в приложении или базе данных.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//
// @returns:
//   - result1: проверенный UUID ресурса.
//   - result2: выполняется ли задокументированное условие операции или произошло ли успешное изменение условного состояния.
func id(c *gin.Context) (string, bool) {
	v, e := uuid.Parse(c.Param("id"))
	if e != nil || v == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", false
	}
	return v.String(), true
}

// Read считывает состояние субтитров в реальном времени, необходимое вызывающей стороне.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Read(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	s, e := h.Repo.Read(c.Request.Context(), middleware.UserID(c), cid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	s.Available = h.Enabled
	s.CanManage = s.CanManage && h.Enabled
	if !s.Enabled {
		s.Status = "off"
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": s})
}

// Read считывает статус субтитров в реальном времени, необходимый вызывающей стороне.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Set(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	var request struct {
		Enabled  *bool  `json:"enabled"`
		Language string `json:"language"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if request.Enabled == nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	if !h.Enabled && *request.Enabled {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	if request.Language == "" {
		request.Language = "auto"
	}
	s, e := h.Repo.Set(c.Request.Context(), middleware.UserID(c), cid, *request.Enabled, request.Language)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	s.Available = h.Enabled
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": s})
}

// Finals возвращает страницу с сохраненными финальными подписями, ограниченную курсором, чтобы клиенты могли восстановить пропущенные
// события в реальном времени.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Finals(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	after, e := strconv.ParseInt(c.DefaultQuery("afterCursor", "0"), 10, 64)
	limit, e2 := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if e != nil || e2 != nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	items, e := h.Repo.Finals(c.Request.Context(), middleware.UserID(c), cid, after, limit)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	cursor := after
	for _, v := range items {
		cursor = max(cursor, v.Cursor)
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items, "nextCursor": cursor, "hasMore": len(items) == limit})
}

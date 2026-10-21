// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package integrationsapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	d "github.com/janickiy/meet-space/internal/domain/integrations"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	u "github.com/janickiy/meet-space/internal/usecase/integrations"
)

type Handler struct{ service *u.Service }

// NewHandler создает обработчик с зависимостями и ограничениями, необходимыми для интеграции учетных записей // и настроек доставки.
//
// @Parameters:
//   - service: служба приложения, вызываемая этим адаптером.
//
// @returns:
//   - result1: созданный или загруженный обработчик.
func NewHandler(service *u.Service) *Handler { return &Handler{service: service} }

// Capabilities возвращает возможности продукта или интеграции, фактически доступные оператору.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Capabilities(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, h.service.Capabilities())
}

// Preferences считывает сохраненные настройки доставки или переписки действующего аккаунта.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Preferences(c *gin.Context) {
	value, err := h.service.Preferences(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, value)
}

// SavePreferences проверяет и сохраняет настройки доставки аккаунта, не инициируя доставку.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) SavePreferences(c *gin.Context) {
	var request d.Preferences
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	if err := h.service.SavePreferences(c.Request.Context(), middleware.UserID(c), request); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, request)
}

// Devices возвращает зарегистрированные в аккаунте устройства доставки без раскрытия их личных данных.
// учетные данные.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Devices(c *gin.Context) {
	items, err := h.service.Devices(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// RegisterDevice регистрирует проверенное устройство доставки действующей учетной записи и защищает его
// приватный токен.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) RegisterDevice(c *gin.Context) {
	var request d.DeviceRequest
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.RegisterDevice(c.Request.Context(), middleware.UserID(c), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": item})
}

// RevokeDevice отзывает доступ только к выбранному устройству доставки действующей учетной записи.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) RevokeDevice(c *gin.Context) {
	id, ok := resourceID(c, "deviceId")
	if !ok {
		return
	}
	if err := h.service.RevokeDevice(c.Request.Context(), middleware.UserID(c), id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Calendars возвращает подключения к календарю действующей учетной записи с учетными данными частного провайдера
// опущено.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Calendars(c *gin.Context) {
	items, err := h.service.Connections(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// MockConnect создает фиктивное подключение к календарю только в том случае, если среда настроена явно
// разрешает это.
//
// @Parameters:
// - c: Контекст запроса и ответа Gin, включая аутентифицированный идентификатор.
func (h *Handler) MockConnect(c *gin.Context) {
	var request struct{}
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.MockConnect(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": item})
}

// Connect открывает настроенную зависимость или запускает процесс авторизованного подключения к ней.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Connect(c *gin.Context) {
	authURL, state, err := h.service.OAuthStart(c.Request.Context(), middleware.UserID(c), c.Param("provider"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"authUrl": authURL, "state": state})
}

// oauthCallbackRequest принимает только одноразовый код и состояние, но не токены доступа или обновления
// провайдера.
type oauthCallbackRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// Callback завершает авторизованное подключение к календарю с использованием одноразового состояния и кода OAuth.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Callback(c *gin.Context) {
	var request oauthCallbackRequest
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.OAuthCallback(c.Request.Context(), middleware.UserID(c), c.Param("provider"), request.Code, request.State)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"item": item})
}

// Disconnect разрывает интеграционное соединение с действующим аккаунтом и прекращает его дальнейшую синхронизацию.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Disconnect(c *gin.Context) {
	id, ok := resourceID(c, "provider")
	if !ok {
		return
	}
	if err := h.service.Disconnect(c.Request.Context(), middleware.UserID(c), id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// CalendarMappings возвращает сохраненные сопоставления конференций с календарями для действующей учетной записи.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) CalendarMappings(c *gin.Context) {
	id, ok := resourceID(c, "id")
	if !ok {
		return
	}
	items, err := h.service.CalendarMappings(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// resourceID проверяет идентификатор ресурса из интеграционного маршрута.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - name: имя настраиваемого параметра, файла cookie, ресурса или провайдера.
//
// @returns:
//   - result1: валидированный UUID интеграционного ресурса.
//   - result2: выполняется ли задокументированное условие операции или ее условное состояние
//     изменено успешно.
func resourceID(c *gin.Context, name string) (string, bool) {
	value, err := uuid.Parse(c.Param(name))
	if err != nil || value == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", false
	}
	return value.String(), true
}

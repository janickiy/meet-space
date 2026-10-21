// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package chatapp

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// GroupDownloads заменяет URL-адреса с поддержкой bearer-capability только для групп.
// Возвращаемый // URL-адрес требует повторной аутентификации при запросе его байтов.
type GroupDownloads interface {
	// GroupDownload авторизует групповое вложение и возвращает целевое значение для частной загрузки с ограниченным доступом.
	//
	// @Parameters:
	// - arg1: Контекст, управляющий отменой, крайними сроками и сроком действия операции.
	// - arg2: Действующий идентификатор учетной записи, используемый для расширения доступа.
	// - arg3: Идентификатор диалога, ограничивающий область действия операции.
	// - arg4: Идентификатор частного вложения или метаданные, определяемые его типом.
	//
	// @returns:
	// - result1: Выполняется ли задокументированное условие операции или ее условное состояние
	// успешно изменено.
	// - result2: URL-адрес для загрузки временного разрешенного вложения.
	// - result3: Временная метка или граница действия, связанная с результатом.
	// - result4: вероятность успеха равна нулю; в противном случае проверка, доступ, транспортировка или сохранение не будут выполнены
	// ошибка.
	GroupDownload(context.Context, string, string, string) (bool, string, time.Time, error)
}

// WithGroupDownloads подключает групповые загрузки к компоненту до того, как будут выполнены его операции.
//
// @Parameters:
//   - provider: Настроенный адаптер внешнего поставщика.
//
// Возвращаемое значение:
//   - result1: Созданный или загруженный обработчик.
func (h *Handler) WithGroupDownloads(provider GroupDownloads) *Handler {
	h.groupDownloads = provider
	return h
}

// downloadGroup указывает путь для загрузки групповых вложений, если данный репозиторий принадлежит
// группе.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - scope: авторизованная область диалога или ресурса.
//   - attachment: личные данные о вложениях или метаданные, в зависимости от их типа.
//
// @returns:
//   - result1: выполняется ли задокументированное условие операции или ее условное состояние
//     изменилось успешно.
func (h *Handler) downloadGroup(c *gin.Context, scope, attachment string) bool {
	if h.namespace != "conversations" || h.groupDownloads == nil {
		return false
	}
	handled, url, expires, err := h.groupDownloads.GroupDownload(c.Request.Context(), middleware.UserID(c), scope, attachment)
	if !handled {
		return false
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return true
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"status": "success", "url": url, "expiresAt": expires, "authenticated": true})
	return true
}

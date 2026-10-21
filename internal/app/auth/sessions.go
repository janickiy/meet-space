// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package authapp

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

const SessionCookieName = "meetrix_session"
const sessionCookiePath = "/api/v1/auth"
const sessionCookieMaxAge = 400 * 24 * 60 * 60

type persistentService interface {
	// Refresh обновляет краткосрочные данные доступа с помощью текущего провайдера
	// или сеанса обновления учётной записи.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: сериализованное значение, не требующее доверия, для декодирования или проверки.
	//
	// @returns:
	//   - result1: полученный ответ на запрос входа.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка при проверке, доступе, передаче или сохранении
	//     данных.
	Refresh(context.Context, string) (users.LoginResponse, error)
	// Bootstrap обновляет существующие действительные учетные данные для доступа до постоянного сеанса учетной записи.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем жизни операции.
	//   - arg2: действующий идентификатор учетной записи, используемый для ограничения доступа.
	//   - arg3: сериализованное значение, не требующее доверия, для декодирования или проверки.
	//   - arg4: текущий подписанный токен доступа, используемый только для проверки сеанса.
	//
	// @returns:
	//   - result1: полученный ответ на запрос входа.
	//   - result2: nil в случае успешного выполнения; в противном случае — ошибка при проверке, доступе, передаче или сохранении данных.
	//
	Bootstrap(context.Context, string, string, string) (users.LoginResponse, error)
	// Logout отзывает выбранный постоянный сеанс и привязку предоставленного токена доступа.
	//
	// @Parameters:
	//   - arg1: контекст, управляющий отменой, сроками и временем выполнения операции.
	//   - arg2: сериализованное значение, не требующее доверия, для декодирования или проверки.
	//   - arg3: текущий подписанный токен доступа, используемый только для проверки сеанса.
	//
	// @returns:
	//   - result1: nil в случае успеха; в противном случае — ошибка валидации, доступа, транспорта или сохранения.
	//
	Logout(context.Context, string, string) error
}

// WithSessionCookies настраивает публичный источник, а не доверяет перенаправленным хостам.
//
// @Parameters:
//   - publicURL: публичный источник внешнего интерфейса, настроенный оператором.
//   - secure: нужно ли ограничивать использование файлов cookie протоколом HTTPS.
//
// @returns:
//   - result1: созданный или загруженный обработчик.
func (h *Handler) WithSessionCookies(publicURL string, secure bool) *Handler {
	if parsed, err := url.Parse(publicURL); err == nil && parsed.Host != "" {
		h.publicOrigin = parsed.Scheme + "://" + parsed.Host
	}
	h.secureCookie = secure
	h.persistentSessions = true
	return h
}

// PersistentSessionsEnabled сообщает, настроены ли для этого обработчика устойчивые сеансы работы с аккаунтами.
//
// @returns:
//   - result1: выполняется ли описанное в документации условие операции или произошло ли успешное изменение ее условного состояния.
func (h *Handler) PersistentSessionsEnabled() bool { return h.persistentSessions }

// sameOrigin отклоняет мутации сессий из разных источников, используя настроенный публичный источник и
// метаданные запроса.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//
// @returns:
//   - result1: выполняется ли задокументированное условие операции или ее условное состояние
//     изменено успешно.
func (h *Handler) sameOrigin(c *gin.Context) bool {
	if strings.EqualFold(c.GetHeader("Sec-Fetch-Site"), "cross-site") {
		httpresponse.Fail(c, apperrors.ErrForbidden)
		return false
	}
	origin := c.GetHeader("Origin")
	if origin == "" {
		// Клиенты CLI, не являющиеся браузерами, не отправляют метаданные Origin или Fetch.
		if c.GetHeader("Sec-Fetch-Site") != "" {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return false
		}
	} else {
		expected := h.publicOrigin
		if expected == "" {
			scheme := "http"
			if h.secureCookie || c.Request.TLS != nil {
				scheme = "https"
			}
			expected = scheme + "://" + c.Request.Host
		}
		if origin != expected {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return false
		}
	}
	if c.Request.ContentLength != 0 {
		mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || mediaType != "application/json" {
			c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{"status": "failed", "message": "application/json is required"})
			return false
		}
	}
	return true
}

// sessionCookie считывает файл cookie обновления для выбранной учетной записи при выполнении операции с сеансом.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//
// Возвращаемое значение:
//   - result1: необработанный секретный ключ обновления для выбранной учетной записи или пустая строка при его отсутствии.
func (h *Handler) sessionCookie(c *gin.Context) string {
	raw, _ := c.Cookie(SessionCookieName)
	return raw
}

// setSessionCookie устанавливает или удаляет выбранный постоянный файл cookie для обновления с настроенными атрибутами безопасности.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - raw: сериализованный токен доступа или учетные данные для обновления для проверки; никогда не записывайте и не раскрывайте их.
func (h *Handler) setSessionCookie(c *gin.Context, raw string) {
	h.setNamedSessionCookie(c, SessionCookieName, raw)
}

// setNamedSessionCookie устанавливает или удаляет именованный файл cookie для обновления, не раскрывая его секретные данные.
// JavaScript.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - name: имя изменяемой переменной, файла cookie, ресурса или провайдера.
//   - raw: сериализованный токен доступа или учетные данные для обновления для проверки; никогда не записывайте и не раскрывайте их.
func (h *Handler) setNamedSessionCookie(c *gin.Context, name, raw string) {
	cookie := &http.Cookie{Name: name, Value: raw, Path: sessionCookiePath,
		MaxAge: sessionCookieMaxAge, Expires: time.Now().UTC().Add(time.Duration(sessionCookieMaxAge) * time.Second),
		HttpOnly: true, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode}
	if raw == "" {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0).UTC()
	}
	http.SetCookie(c.Writer, cookie)
}

// bearerToken извлекает токен доступа из правильно сформированного заголовка авторизации Bearer.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//
// @returns:
//   - result1: необработанный токен доступа Bearer или пустая строка в случае некорректного заголовка.
func bearerToken(c *gin.Context) string {
	fields := strings.Fields(c.GetHeader("Authorization"))
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		return fields[1]
	}
	return ""
}

// Refresh принимает данные сеанса браузера, даже если срок действия предыдущего JWT истёк.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Refresh(c *gin.Context) { h.sessionOperation(c, false) }

// Bootstrap переносит существующие учетные записи без необходимости повторного входа в систему.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
func (h *Handler) Bootstrap(c *gin.Context) { h.sessionOperation(c, true) }

// sessionOperation обрабатывает запросы на обновление или инициализацию с проверкой строгого источника, типа содержимого и
// учетной записи.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - bootstrap: нужно ли обновить существующий токен доступа вместо обновления постоянного
//     сеанса.
func (h *Handler) sessionOperation(c *gin.Context, bootstrap bool) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var request struct {
		UserID string `json:"userId"`
	}
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	sessions, ok := h.service.(persistentService)
	if !ok || !h.persistentSessions {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	var response users.LoginResponse
	var err error
	if bootstrap {
		id := httpmiddleware.UserID(c)
		raw, _ := c.Cookie(accountCookieName(id))
		if raw == "" {
			raw = h.sessionCookie(c)
		}
		response, err = sessions.Bootstrap(c.Request.Context(), id, raw, bearerToken(c))
	} else {
		raw := h.sessionCookie(c)
		if request.UserID != "" {
			raw, _ = c.Cookie(accountCookieName(request.UserID))
			// В существующих браузерах изначально есть только исходный файл cookie. Перед переносом проверьте его
			// владельца, указанного ниже; никогда не восстанавливайте другую идентификацию.
			if raw == "" {
				raw = h.sessionCookie(c)
			}
		}
		response, err = sessions.Refresh(c.Request.Context(), raw)
		if err == nil && request.UserID != "" && response.User.ID != request.UserID {
			err = apperrors.ErrForbidden
		}
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	// Обновление фона не должно влиять на активную учетную запись в других вкладках.
	if bootstrap || request.UserID == "" {
		h.setSessionCookie(c, response.SessionToken)
	}
	h.setNamedSessionCookie(c, accountCookieName(response.User.ID), response.SessionToken)
	c.JSON(http.StatusOK, response)
}

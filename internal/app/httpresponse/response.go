// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package httpresponse

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// InternalErrorMessage безопасное публичное сообщение о непредвиденных ошибках сервера.
const InternalErrorMessage = "internal server error"

// Fail преобразует ошибку приложения в безопасный HTTP-статус и тело ответа, не раскрывая
// внутреннюю диагностику.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - err: ошибка, которую нужно классифицировать, обернуть или безопасно сообщить о ней.
func Fail(c *gin.Context, err error) {
	status, message := http.StatusInternalServerError, InternalErrorMessage
	switch apperrors.Classify(err) {
	case apperrors.Validation:
		status, message = http.StatusUnprocessableEntity, "invalid input"
	case apperrors.Unauthenticated:
		status, message = http.StatusUnauthorized, "authentication required or token invalid"
	case apperrors.Forbidden:
		status, message = http.StatusForbidden, "access denied"
	case apperrors.NotFound:
		status, message = http.StatusNotFound, "not found"
	case apperrors.Conflict:
		status, message = http.StatusConflict, "conflict"
	case apperrors.RateLimited:
		status, message = http.StatusTooManyRequests, "rate limit exceeded"
	case apperrors.Timeout:
		status, message = http.StatusGatewayTimeout, "operation timed out"
	case apperrors.Unavailable:
		status, message = http.StatusServiceUnavailable, "service temporarily unavailable"
	}
	var applicationError *apperrors.Error
	if status != http.StatusInternalServerError && errors.As(err, &applicationError) {
		message = applicationError.Message
	}
	c.AbortWithStatusJSON(status, gin.H{"status": "failed", "message": message})
}

// BindJSON декодирует одно ограниченное значение JSON, отбрасывая неизвестные поля и завершающие данные; опционально
// входные данные могут быть пустыми.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//   - target: целевое состояние, учетная запись или декодированное значение в соответствии с его типом.
//   - optional: разрешено ли использование пустого тела запроса.
//
// @returns:
//   - result1: выполняется ли задокументированное условие операции или ее условное состояние
//     изменено успешно.
func BindJSON(c *gin.Context, target any, optional bool) bool {
	if c.Request.Body == nil {
		c.Request.Body = http.NoBody
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if optional && errors.Is(err, io.EOF) {
		return true
	}
	if err == nil {
		var trailing any
		if errors.Is(decoder.Decode(&trailing), io.EOF) {
			return true
		}
	}
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"status": "failed", "message": "invalid json body or unknown fields"})
	return false
}

// При разбивке на страницы размер страницы и смещение в запросе анализируются, а недопустимые значения или значения вне допустимого диапазона отклоняются.
//
// @Parameters:
//   - c: контекст запроса и ответа Gin, включая аутентифицированные данные.
//
// @returns:
//   - result1: итоговое количество, размер или числовое значение.
//   - result2: итоговое количество, размер или числовое значение.
//   - result3: выполняется ли задокументированное условие операции или достигнуто ли условное состояние
//     изменения.
func Pagination(c *gin.Context) (int, int, bool) {
	limit, offset := 20, 0
	var err error
	if raw, exists := c.GetQuery("limit"); exists {
		limit, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 100 {
		Fail(c, apperrors.New(apperrors.ErrInvalidInput, "limit must be between 1 and 100"))
		return 0, 0, false
	}
	if raw, exists := c.GetQuery("offset"); exists {
		offset, err = strconv.Atoi(raw)
	}
	if err != nil || offset < 0 {
		Fail(c, apperrors.New(apperrors.ErrInvalidInput, "offset must be non-negative"))
		return 0, 0, false
	}
	return limit, offset, true
}

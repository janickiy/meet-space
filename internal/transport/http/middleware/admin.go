// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package httpmiddleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// AdminChecker считывает сохранённые глобальные права администратора для аутентифицированной
// учётной записи.
type AdminChecker interface {
	// IsAdmin проверяет настроенные административные идентификационные данные, не раскрывая
	// конфигурацию администратора.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	IsAdmin(context.Context, string) (bool, error)
}

// RequireAdmin проверяет сохранённый доступ администратора при каждом запросе, не полагаясь на
// роли конференции или устаревшие утверждения в токене.
//
// @Parameters:
//   - checker: Проверка сохранённого доступа администратора.
//
// @returns:
//   - result1: Обработчик HTTP для регистрации этой операции.
func RequireAdmin(checker AdminChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker == nil || UserID(c) == "" {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		allowed, err := checker.IsAdmin(c.Request.Context(), UserID(c))
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if !allowed {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return
		}
		c.Next()
	}
}

// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"context"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
)

// SessionAccount возвращает общедоступное представление учетной записи для действительного
// постоянного сеанса.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - raw: Сериализованный токен доступа или обновленные учетные данные для проверки; никогда
//     не сохраняйте и не раскрывайте их.
//
// @returns:
//   - result1: Результирующее представление.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) SessionAccount(ctx context.Context, raw string) (users.View, error) {
	session, err := s.sessionForCookie(ctx, raw)
	if err != nil {
		return users.View{}, err
	}
	user, err := s.account(ctx, session.UserID)
	return user.View(), err
}

// DeleteAccount проверяет право владения учетной записью и удаляет учетную запись вместе с ее
// состоянием сеанса.
//
// При удалении всегда выполняется повторная аутентификация текущего идентификатора;
// отправленный пользовательский ID никогда не будет принят.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - password: Пароль в виде обычного текста принимается только для проверки или хеширования.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) DeleteAccount(ctx context.Context, id, password string) error {
	user, err := s.account(ctx, id)
	if err != nil {
		return err
	}
	if password == "" || !utf8.ValidString(password) || utf8.RuneCountInString(password) > users.MaxPasswordCharacters {
		return apperrors.New(apperrors.ErrInvalidInput, "incorrect account password")
	}
	valid, err := s.verifyPassword(ctx, password, user.PasswordHash)
	if err != nil {
		return err
	}
	if !valid {
		return apperrors.New(apperrors.ErrInvalidInput, "incorrect account password")
	}
	repo, ok := s.repository.(interface {
		// DeleteAccount проверяет право владения учетной записью и удаляет учетную запись
		// вместе с ее состоянием сеанса.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
		//   - arg3: Пароль в виде обычного текста принимается только для проверки или
		//   хеширования.
		//
		// @returns:
		//   - result1: nil — в случае успеха; в противном случае — ошибка валидации,
		//   доступа, передачи данных или сохранения.
		DeleteAccount(context.Context, string, string) error
	})
	if !ok {
		return apperrors.ErrUnavailable
	}
	return repo.DeleteAccount(ctx, id, user.PasswordHash)
}

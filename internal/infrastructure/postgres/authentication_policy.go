// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"gorm.io/gorm"
)

// Сеанс допустим только для источника, выбранного администратором. Отсутствие
// настроек не должно открывать доступ по умолчанию.
const accountAuthenticationModeSQL = `EXISTS(SELECT 1 FROM platform_settings ps WHERE ps.singleton
 AND (u.auth_source='ldap')=COALESCE((ps.values->>'ldapEnabled')::boolean,false))`

const localAuthenticationModeSQL = `EXISTS(SELECT 1 FROM platform_settings ps WHERE ps.singleton
 AND NOT COALESCE((ps.values->>'ldapEnabled')::boolean,false))`

// Блокируем настройки раньше пользователя: смена режима не может завершиться
// между проверкой политики и созданием аккаунта, сессии или сменой пароля.
func lockAuthenticationMode(tx *gorm.DB) (bool, error) {
	var enabled bool
	result := tx.Raw("SELECT COALESCE((values->>'ldapEnabled')::boolean,false) FROM platform_settings WHERE singleton FOR SHARE").Scan(&enabled)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected != 1 {
		return false, apperrors.ErrUnavailable
	}
	return enabled, nil
}

func requireLocalAuthentication(tx *gorm.DB) error {
	ldapOnly, err := lockAuthenticationMode(tx)
	if err != nil {
		return err
	}
	if ldapOnly {
		return apperrors.New(apperrors.ErrForbidden, "Локальные аккаунты отключены. Используйте корпоративный email и пароль.")
	}
	return nil
}

// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// AuthorizeLegacy сохраняет действительные старые учетные данные, если пользователь не вышел из
// системы в этом браузере.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - hash: Ранее вычисленный дайджест, используемый для проверки подлинности или целостности.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *AuthSessionRepository) AuthorizeLegacy(ctx context.Context, hash, userID string) error {
	var enabled bool
	if err := r.db.WithContext(ctx).Raw("SELECT EXISTS(SELECT 1 FROM users WHERE id=? AND password_hash<>'')", userID).Scan(&enabled).Error; err != nil {
		return err
	}
	if !enabled {
		return apperrors.ErrUnauthorized
	}
	var legacy authLegacyToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).Take(&legacy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if legacy.UserID != userID || legacy.RevokedAt != nil {
		return apperrors.ErrUnauthorized
	}
	if legacy.SessionID != nil {
		session, err := r.GetByID(ctx, *legacy.SessionID)
		if errors.Is(err, apperrors.ErrNotFound) || err == nil && session.RevokedAt != nil {
			return apperrors.ErrUnauthorized
		}
		return err
	}
	return nil
}

// AuthSessionRepository намеренно не предусматривает тайм-аут простоя или очистку по истечении
// срока действия.
type AuthSessionRepository struct{ db *gorm.DB }

type authSessionToken struct {
	TokenHash string `gorm:"primaryKey"`
	SessionID string
}

type authLegacyToken struct {
	TokenHash string `gorm:"primaryKey"`
	UserID    string
	SessionID *string
	RevokedAt *time.Time
}

// NewAuthSessionRepository создает репозиторий сессий аутентификации с зависимостями и
// ограничениями, необходимыми для работы сессий аутентификации.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий сессий аутентификации.
func NewAuthSessionRepository(db *gorm.DB) *AuthSessionRepository {
	return &AuthSessionRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

// Create сохраняет новую серверную сессию и дайджест секрета обновления, не сохраняя исходный
// секрет обновления.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *AuthSessionRepository) Create(ctx context.Context, session users.AuthSession) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		return tx.Create(&authSessionToken{TokenHash: session.TokenHash, SessionID: session.ID}).Error
	})
}

// GetByHash загружает постоянную серверную сессию по дайджесту секрета обновления.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - hash: Ранее вычисленный дайджест, используемый для проверки подлинности или целостности.
//
// @returns:
//   - result1: Результирующая сессия аутентификации.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *AuthSessionRepository) GetByHash(ctx context.Context, hash string) (users.AuthSession, error) {
	var session users.AuthSession
	err := r.db.WithContext(ctx).Model(&users.AuthSession{}).Select("auth_sessions.*").
		Joins("JOIN users u ON u.id=auth_sessions.user_id AND u.password_hash<>''").
		Joins("JOIN auth_session_tokens ON auth_session_tokens.session_id = auth_sessions.id").
		Where("auth_session_tokens.token_hash = ?", hash).Take(&session).Error
	return session, mapNotFound(err)
}

// GetByID загружает постоянный сеанс сервера по его идентификатору.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Результирующая сессия аутентификации.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *AuthSessionRepository) GetByID(ctx context.Context, id string) (users.AuthSession, error) {
	var session users.AuthSession
	err := r.db.WithContext(ctx).Model(&users.AuthSession{}).Select("auth_sessions.*").Joins("JOIN users u ON u.id=auth_sessions.user_id AND u.password_hash<>''").Where("auth_sessions.id = ?", id).Take(&session).Error
	return session, mapNotFound(err)
}

// RevokeByHash отзывает постоянный сеанс, соответствующий дайджесту refresh-secret.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - hash: Ранее вычисленный дайджест, используемый для проверки подлинности или целостности.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *AuthSessionRepository) RevokeByHash(ctx context.Context, hash string) error {
	return r.db.WithContext(ctx).Exec(`UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now())
  WHERE id IN (SELECT session_id FROM auth_session_tokens WHERE token_hash = ?)`, hash).Error
}

// RevokeByID отзывает только постоянный сеанс, принадлежащий указанной учетной записи и имеющий
// указанный идентификатор.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *AuthSessionRepository) RevokeByID(ctx context.Context, id, userID string) error {
	return r.db.WithContext(ctx).Exec(`UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now())
  WHERE id = ? AND user_id = ?`, id, userID).Error
}

// Bootstrap — при начальной загрузке сериализуются все параллельные обновления JWT и маркер выхода из
// системы.  Непрозрачные секреты восстановлению не подлежат; для каждого SID используются
// отдельные хэши.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - candidate: Сессия кандидата или запись ICE для проверки перед использованием.
//   - legacyHash: Дайджест устаревшего токена доступа, используемый для проверки на возможность
//     отзыва.
//   - preferredID: При повторном входе в систему через браузер предпочтительнее использовать
//     существующий идентификатор сессии аккаунта.
//
// @returns:
//   - result1: Результирующая сессия аутентификации.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *AuthSessionRepository) Bootstrap(ctx context.Context, candidate users.AuthSession, legacyHash, preferredID string) (users.AuthSession, error) {
	var session users.AuthSession
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var legacy authLegacyToken
		if legacyHash != "" {
			legacy = authLegacyToken{TokenHash: legacyHash, UserID: candidate.UserID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&legacy).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", legacyHash).Take(&legacy).Error; err != nil {
				return err
			}
			if legacy.UserID != candidate.UserID || legacy.RevokedAt != nil {
				return apperrors.ErrUnauthorized
			}
			if legacy.SessionID != nil {
				preferredID = *legacy.SessionID
			}
		}
		if preferredID != "" {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", preferredID).Take(&session).Error; err != nil {
				return mapNotFound(err)
			}
			if session.UserID != candidate.UserID || session.RevokedAt != nil {
				return apperrors.ErrUnauthorized
			}
		} else {
			session = candidate
			if err := tx.Create(&session).Error; err != nil {
				return err
			}
		}
		if legacyHash != "" && legacy.SessionID == nil {
			if err := tx.Model(&authLegacyToken{}).Where("token_hash = ?", legacyHash).Update("session_id", session.ID).Error; err != nil {
				return err
			}
		}
		return tx.Create(&authSessionToken{TokenHash: candidate.TokenHash, SessionID: session.ID}).Error
	})
	return session, err
}

// RevokeLegacy создает неизменяемый маркер, даже если параллельное обновление еще не запущено.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - hash: Ранее вычисленный дайджест, используемый для проверки подлинности или целостности.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *AuthSessionRepository) RevokeLegacy(ctx context.Context, hash, userID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		legacy := authLegacyToken{TokenHash: hash, UserID: userID, RevokedAt: &now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&legacy).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", hash).Take(&legacy).Error; err != nil {
			return err
		}
		if legacy.UserID != userID {
			return apperrors.ErrUnauthorized
		}
		if err := tx.Model(&authLegacyToken{}).Where("token_hash = ?", hash).Update("revoked_at", now).Error; err != nil {
			return err
		}
		if legacy.SessionID != nil {
			return tx.Exec("UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ? AND user_id = ?", now, *legacy.SessionID, userID).Error
		}
		return nil
	})
}

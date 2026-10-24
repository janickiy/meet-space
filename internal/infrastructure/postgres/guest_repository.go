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

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type GuestRepository struct{ db *gorm.DB }

// NewGuestRepository создает гостевой репозиторий с зависимостями и ограничениями, необходимыми
// для работы с гостевыми учетными записями.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный гостевой репозиторий.
func NewGuestRepository(db *gorm.DB) *GuestRepository {
	return &GuestRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

// JoinGuest фиксирует личность и членство в группе одновременно.  Гости, получившие отказ, при
// повторной попытке с использованием того же сеанса сохраняют свои текущие учетные данные.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - code: Ограниченный публичный статус или код ошибки.
//   - name: Имя параметра, файла cookie, ресурса или провайдера, к которому осуществляется
//     обращение.
//   - resumeUserID: Идентификатор существующей гостевой учетной записи, с помощью которого
//     можно возобновить действие приглашения.
//
// @returns:
//   - result1: Полученный пользователь.
//   - result2: Полученный участник.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *GuestRepository) JoinGuest(ctx context.Context, code, name, resumeUserID string) (users.User, conferences.Participant, error) {
	var user users.User
	var participant conferences.Participant
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conference conferences.Conference
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("invite_code = ? AND kind = 'meeting'", code).Take(&conference).Error; err != nil {
			return mapNotFound(err)
		}
		if conference.Status != conferences.Created && conference.Status != conferences.Active {
			return apperrors.New(apperrors.ErrConflict, "conference is not open for joining")
		}
		if resumeUserID != "" {
			if err := tx.Where("id = ?", resumeUserID).Take(&user).Error; err != nil {
				return mapNotFound(err)
			}
			if user.GuestConferenceID == nil {
				return apperrors.ErrForbidden
			}
			if *user.GuestConferenceID != conference.ID {
				user = users.User{}
			}
		}
		if user.ID == "" {
			user = users.User{ID: uuid.NewString(), DisplayName: &name, GuestConferenceID: &conference.ID, PasswordHash: "!guest"}
			user.Email = "guest-" + user.ID + "@guest.invalid"
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		}
		var err error
		participant, err = NewConferenceRepository(tx).Join(ctx, conference.ID, user, code)
		if err != nil {
			return err
		}
		if err := tx.Model(&user).Update("display_name", name).Error; err != nil {
			return err
		}
		if err := tx.Model(&participant).Update("display_name", name).Error; err != nil {
			return err
		}
		user.DisplayName, participant.DisplayName = &name, name
		return nil
	})
	return user, participant, err
}

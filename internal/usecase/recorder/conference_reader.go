// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package recorder

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
)

// ReadComposite загружает авторизованную карту составной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученная карта записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) ReadComposite(ctx context.Context, id string) (records.RecordCard, error) {
	details, err := s.repository.FindDetailsByUUID(ctx, id)
	if err != nil {
		return records.RecordCard{}, err
	}
	if !records.IsComposite(details.Record) {
		return records.RecordCard{}, apperrors.ErrNotFound
	}
	return s.recordCard(ctx, details)
}

// ReadComposites формирует уже авторизованную страницу, сохраняя при этом проверку доступности
// и режима для одной записи. Старые адаптеры репозиториев продолжают работать.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - ids: Идентификаторы ресурсов для обработки в рамках ограниченного пакета.
//
// @returns:
//   - result1: Собранные значения карты записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) ReadComposites(ctx context.Context, ids []string) ([]records.RecordCard, error) {
	batch, ok := s.repository.(interface {
		// FindDetailsByUUIDs загружает данные по uui ds из репозитория записей.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Идентификаторы ресурсов для обработки в рамках ограниченного пакета.
		//
		// @returns:
		//   - result1: Собранные сведения о записях.
		//   - result2: nil — в случае успеха; в противном случае — ошибка валидации,
		//   доступа, передачи данных или сохранения.
		FindDetailsByUUIDs(context.Context, []string) ([]records.RecordDetails, error)
	})
	if !ok {
		cards := make([]records.RecordCard, 0, len(ids))
		for _, id := range ids {
			card, err := s.ReadComposite(ctx, id)
			if err != nil {
				return nil, err
			}
			cards = append(cards, card)
		}
		return cards, nil
	}
	rows, err := batch.FindDetailsByUUIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	cards := make([]records.RecordCard, 0, len(rows))
	for _, row := range rows {
		if !records.IsComposite(row.Record) {
			return nil, apperrors.ErrNotFound
		}
		card, err := s.recordCard(ctx, row)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, nil
}

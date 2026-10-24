// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package conferences

import (
	"context"
	"errors"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
)

// productRepository определяет заменяемый контракт зависимостей для жизненного цикла
// конференции и членства участников.
type productRepository interface {
	// DecideAdmission запись о подтверждении или отклонении заявки на участие в конференц-зале.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg4: Идентификатор участника конференции.
	//   - arg5: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Полученный участник.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	DecideAdmission(context.Context, string, string, string, domain.AdmissionRequest) (domain.Participant, error)
	// UpdateSchedule сохраняет утвержденное расписание конференции и его обновленную версию для
	// синхронизации.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg4: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Результирующая конференция.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	UpdateSchedule(context.Context, string, string, domain.ScheduleRequest) (domain.Conference, error)
	// Timeline возвращает разбитую на страницы хронологию предстоящих, текущих и прошедших
	// конференций для аккаунта.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Текст для поиска или валидированные фильтры запросов.
	//
	// @returns:
	//   - result1: Собранные данные о конференции.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Timeline(context.Context, string, domain.TimelineQuery) ([]domain.Conference, error)
	// History формирует авторизованное представление о завершенной конференции с историей
	// участников и краткими описаниями записей.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Результирующее представление об истории.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	History(context.Context, string, string) (domain.HistoryView, error)
}

// Self возвращает информацию о членстве действующего аккаунта, включая статус ожидания и
// допуск.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученное представление участника.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Self(ctx context.Context, userID, id string) (domain.ParticipantView, error) {
	p, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	return p.View(), err
}

// Admission — допуск принимает или отклоняет запрос на допуск из комнаты ожидания после проверки полномочий
// действующего участника.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - participantID: Идентификатор участника конференции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Полученное представление участника.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Admission(ctx context.Context, userID, id, participantID string, request domain.AdmissionRequest) (domain.ParticipantView, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.ParticipantView{}, apperrors.ErrUnavailable
	}
	p, err := repo.DecideAdmission(ctx, id, userID, participantID, request)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	kind := "participant.admitted"
	if request.Decision == "reject" {
		kind = "participant.rejected"
	}
	s.event(ctx, kind, id, p)
	return p.View(), nil
}

// Schedule обновляет запланированное расписание конференции после проверки прав собственности и
// временных ограничений.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Результирующее представление.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Schedule(ctx context.Context, userID, id string, request domain.ScheduleRequest) (domain.View, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.View{}, apperrors.ErrUnavailable
	}
	c, err := repo.UpdateSchedule(ctx, id, userID, request)
	if err != nil {
		return domain.View{}, err
	}
	s.changed(ctx, id)
	return c.View(), nil
}

// Timeline возвращает разбитую на страницы хронологию предстоящих, текущих и прошедших
// конференций для аккаунта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - query: Текст для поиска или валидированные фильтры запросов.
//
// @returns:
//   - result1: Результирующая страница с хронологией.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Timeline(ctx context.Context, userID string, query domain.TimelineQuery) (domain.TimelinePage, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.TimelinePage{}, apperrors.ErrUnavailable
	}
	rows, err := repo.Timeline(ctx, userID, query)
	if err != nil {
		return domain.TimelinePage{}, err
	}
	page := domain.TimelinePage{Items: []domain.View{}}
	if len(rows) > query.Limit {
		rows = rows[:query.Limit]
		cursor := domain.EncodeTimelineCursor(rows[len(rows)-1])
		page.NextCursor = &cursor
	}
	for _, c := range rows {
		// Для листинга не требуются данные о приглашениях.  Сохраняется конфиденциальность
		// данных о членстве в ожидании подтверждения без необходимости отдельно запрашивать
		// данные о членстве для каждой конференции.
		view := c.View()
		view.InviteCode, view.InviteURL = "", ""
		page.Items = append(page.Items, view)
	}
	return page, nil
}

// History формирует авторизованное представление о завершенной конференции с историей
// участников и краткими описаниями записей.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Результирующее представление об истории.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) History(ctx context.Context, userID, id string) (domain.HistoryView, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.HistoryView{}, apperrors.ErrUnavailable
	}
	return repo.History(ctx, id, userID)
}

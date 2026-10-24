// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package recordings

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/janickiy/meet-space/internal/domain/records"
)

// Repository является границей атомарной записи конференции API.  Доставка устойчивых команд
// осуществляется CommandRepository, а не службой обработки запросов.
type Repository interface {
	// Start запускает управление жизненным циклом записи конференции и подготавливает ресурсы,
	// используемые в рамках этой операции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Запрошенная продолжительность сегмента записи.
	//
	// @returns:
	//   - result1: Результирующая запись.
	//   - result2: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Start(context.Context, string, string, int) (records.Record, bool, error)
	// Stop останавливает активную операцию записи конференции и освобождает принадлежащие ей
	// ресурсы.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Внешняя запись UUID.
	//
	// @returns:
	//   - result1: Результирующая запись.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Stop(context.Context, string, string, string) (records.Record, error)
	// Accessible загружает запись только в рамках полномочий действующего аккаунта в рамках
	// конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Внешняя запись UUID.
	//
	// @returns:
	//   - result1: Результирующая запись.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Accessible(context.Context, string, string, string) (records.Record, error)
	// List возвращает ограниченную страницу записей конференции с использованием заданных
	// параметров разбивки на страницы и фильтров.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Максимальное количество результатов или операций в этом пакете.
	//   - arg5: Количество результатов, пропущенных до перехода на запрошенную страницу.
	//
	// @returns:
	//   - result1: Собранные значения записей.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	List(context.Context, string, string, int, int) ([]records.Record, error)
	// Delete удаляет запрошенную запись конференции, используя область видимости вызывающей
	// стороны и правила управления жизненным циклом ресурсов.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Внешняя запись UUID.
	//   - arg5: Обратный вызов, удаляющий объекты хранения, принадлежащие авторизованной
	//   записи.
	//
	// @returns:
	//   - result1: Результирующая запись.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Delete(context.Context, string, string, string, func(context.Context, records.Record) error) (records.Record, error)
}

type Storage interface {
	// RemoveRecording удаляет только проверенные префиксы приватных объектов, относящиеся к
	// выбранной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Запись и сохраненные метаданные ее жизненного цикла.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RemoveRecording(context.Context, records.Record) error
}

// Reader загружает карточки с записями после того, как репозиторий проверит доступ.
type Reader interface {
	// ReadComposite загружает одну запись и связанные с ней артефакты.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Полученная карта записи.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ReadComposite(context.Context, string) (records.RecordCard, error)
}

// Events публикует изменения в записях для участников конференции.
type Events interface {
	// Broadcast публикует доверенное событие конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Broadcast(context.Context, realtime.Envelope) error
}

// Service обрабатывает авторизованные запросы на запись и чтение. Не использует горутины
// опроса, очереди подключений или распределённые блокировки.
type Service struct {
	repo    Repository
	reader  Reader
	events  Events
	storage Storage
}

// NewConferenceService создаёт сервис конференции с зависимостями и ограничениями, необходимыми
// для управления жизненным циклом записей конференции.
//
// @Parameters:
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - reader: Поток исходного контента или устройство чтения карт памяти, в зависимости от его
//     типа.
//   - events: Место назначения для зафиксированных событий жизненного цикла или чатов.
//   - storage: Зависимость от приватного объекта или хранилища файлов.
//
// @returns:
//   - result1: Созданный или загруженный сервис.
func NewConferenceService(repo Repository, reader Reader, events Events, storage Storage) *Service {
	return &Service{repo: repo, reader: reader, events: events, storage: storage}
}

// Delete удаляет артефакты только после авторизации и публикует их после фиксации.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Delete(ctx context.Context, userID, conferenceID, recordID string) error {
	operation, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	record, err := s.repo.Delete(operation, userID, conferenceID, recordID, s.storage.RemoveRecording)
	if err != nil {
		if operation.Err() == context.DeadlineExceeded {
			return apperrors.New(apperrors.ErrTimeout, "recording deletion timed out")
		}
		return err
	}
	publish(s.events, ctx, record, "recording.deleted")
	return nil
}

// Start фиксирует запрос на запись; диспетчер отправляет команду на сохранение.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Полученная карта записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Start(ctx context.Context, userID, conferenceID string, request records.ConferenceStartRequest) (records.RecordCard, error) {
	if request.Mode == "" {
		request.Mode = records.ModeComposite
	}
	if !records.ValidConferenceMode(request.Mode) {
		return records.RecordCard{}, apperrors.ErrInvalidInput
	}
	segmentDurationSec := request.SegmentDurationSec
	if segmentDurationSec == 0 {
		segmentDurationSec = 5
	}
	if segmentDurationSec < 2 || segmentDurationSec > 30 {
		return records.RecordCard{}, apperrors.New(apperrors.ErrInvalidInput, "segmentDurationSec must be between 2 and 30")
	}
	var record records.Record
	var created bool
	var err error
	if modes, ok := s.repo.(interface {
		// StartMode запускает запись в выбранном режиме валидированной записи конференции.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
		//   доступа.
		//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
		//   - arg4: Запрошенная продолжительность сегмента записи.
		//   - arg5: Проверенная операция или режим записи.
		//
		// @returns:
		//   - result1: Результирующая запись.
		//   - result2: Указывает, выполняется ли задокументированное условие операции и
		//   произошло ли успешное изменение ее условного состояния.
		//   - result3: nil — в случае успеха; в противном случае — ошибка валидации,
		//   доступа, передачи данных или сохранения.
		StartMode(context.Context, string, string, int, string) (records.Record, bool, error)
	}); ok {
		record, created, err = modes.StartMode(ctx, userID, conferenceID, segmentDurationSec, request.Mode)
	} else if request.Mode == records.ModeComposite {
		record, created, err = s.repo.Start(ctx, userID, conferenceID, segmentDurationSec)
	} else {
		return records.RecordCard{}, apperrors.ErrInvalidInput
	}
	if err != nil {
		return records.RecordCard{}, err
	}
	if created {
		publish(s.events, ctx, record, "recording.starting")
	}
	return s.card(ctx, record.UUID)
}

// Stop запрашивает остановку; обработчик захвата освобождает ресурсы для записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: Полученная карта записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Stop(ctx context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	record, err := s.repo.Stop(ctx, userID, conferenceID, recordID)
	if err != nil {
		return records.RecordCard{}, err
	}
	if record.Status == records.StatusStopping {
		publish(s.events, ctx, record, "recording.stopping")
	}
	return s.card(ctx, record.UUID)
}

// Read проверяет доступ перед загрузкой карточки с записью.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: Полученная карта записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Read(ctx context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	record, err := s.repo.Accessible(ctx, userID, conferenceID, recordID)
	if err != nil {
		return records.RecordCard{}, err
	}
	return s.card(ctx, record.UUID)
}

// List проверяет доступ и использует пакетное чтение, если считывающее устройство его
// поддерживает.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//   - offset: Количество результатов, пропущенных до перехода на запрошенную страницу.
//
// @returns:
//   - result1: Собранные значения карты записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) List(ctx context.Context, userID, conferenceID string, limit, offset int) ([]records.RecordCard, error) {
	rows, err := s.repo.List(ctx, userID, conferenceID, limit, offset)
	if err != nil {
		return nil, err
	}
	if batch, ok := s.reader.(interface {
		// ReadComposites загружает авторизованные карточки с составными записями в ограниченном
		// пакете.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Идентификаторы ресурсов для обработки в рамках ограниченного пакета.
		//
		// @returns:
		//   - result1: Собранные значения карты записи.
		//   - result2: nil — в случае успеха; в противном случае — ошибка валидации,
		//   доступа, передачи данных или сохранения.
		ReadComposites(context.Context, []string) ([]records.RecordCard, error)
	}); ok {
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.UUID)
		}
		items, err := batch.ReadComposites(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i := range items {
			items[i].Status = records.PublicStatus(items[i].Status)
		}
		return items, nil
	}
	items := make([]records.RecordCard, 0, len(rows))
	for _, record := range rows {
		card, err := s.card(ctx, record.UUID)
		if err != nil {
			return nil, err
		}
		items = append(items, card)
	}
	return items, nil
}

// card — карта сопоставляет внутренние состояния финализации с общедоступным статусом записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - recordID: Внешняя запись UUID.
//
// @returns:
//   - result1: Полученная карта записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) card(ctx context.Context, recordID string) (records.RecordCard, error) {
	card, err := s.reader.ReadComposite(ctx, recordID)
	card.Status = records.PublicStatus(card.Status)
	return card, err
}

// publish — при публикации отправляется уведомление о том, что сделано все возможное; в случае сбоя
// доставки запрос не отменяется.
//
// @Parameters:
//   - events: Место назначения для зафиксированных событий жизненного цикла или чатов.
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//   - eventType: Ограниченная категория событий жизненного цикла записи.
func publish(events Events, ctx context.Context, record records.Record, eventType string) {
	if events != nil {
		_ = events.Broadcast(ctx, realtime.Event(eventType, record.ConferenceID, map[string]any{"recordingId": record.UUID, "conferenceId": record.ConferenceID, "status": records.PublicStatus(record.Status), "mode": record.Mode, "requestedBy": record.RequestedBy}))
	}
}

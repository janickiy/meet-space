// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package chat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

// Repository — репозиторий определяет заменяемый контракт зависимостей для постоянных сообщений чата и
// личных вложений.
type Repository interface {
	// List возвращает страницу с ограниченным количеством записей о сообщениях чата с
	// использованием заданных параметров разбивки на страницы и фильтров.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Непрозрачная граница продолжения с предыдущей страницы.
	//   - arg5: Максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: Полученная страница.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	List(context.Context, string, string, string, int) (domain.Page, error)
	// Send проверяет ссылки на доступ, ответы и вложения и сохраняет сообщение чата с защитой
	// от повторяющихся идентичных запросов.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Валидированный запрос приложения или входящие параметры операции.
	//   - arg5: Стабильный отпечаток запроса, используемый для обнаружения повторяющихся
	//   идентичных запросов.
	//
	// @returns:
	//   - result1: Полученное сообщение.
	//   - result2: Указывает, создал ли этот запрос новое состояние или повторно
	//   использовал идемпотентный результат.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Send(context.Context, string, string, domain.SendRequest, string) (domain.Message, bool, error)
	// Edit — при редактировании существующий текст сообщения автора изменяется после проверки
	// видимости и возможности редактирования.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg5: Текст UTF-8 для проверки, нормализации или обработки.
	//
	// @returns:
	//   - result1: Полученное сообщение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Edit(context.Context, string, string, string, string) (domain.Message, error)
	// Delete удаляет запрошенное сообщение чата с учётом области видимости вызывающей стороны и
	// правил жизненного цикла ресурса.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Полученное сообщение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Delete(context.Context, string, string, string) (domain.Message, error)
	// ReadState возвращает сохраненную границу прочитанных сообщений участника и количество
	// видимых непрочитанных сообщений.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Полученное состояние чтения.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ReadState(context.Context, string, string) (domain.ReadState, error)
	// MarkRead перемещает сохраненную границу прочитанных сообщений, не позволяя поздним или
	// повторным запросам сдвигать ее назад.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор сообщения в рамках авторизованного диалога.
	//
	// @returns:
	//   - result1: Полученное состояние чтения.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	MarkRead(context.Context, string, string, string) (domain.ReadState, error)
	// InitAttachment создаёт или повторно использует ожидающие обработки метаданные вложений,
	// чтобы можно было безопасно повторить загрузку.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Результирующее вложение.
	//   - result2: Указывает, создал ли этот запрос новое состояние или повторно
	//   использовал идемпотентный результат.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	InitAttachment(context.Context, string, string, domain.InitRequest) (domain.Attachment, bool, error)
	// ClaimUpload атомарно запрашивает попытку загрузки вложения, используя токен владения с
	// ограниченным сроком действия.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg5: Подписанные учетные данные или токен владения операцией для проверки.
	//
	// @returns:
	//   - result1: Результирующее вложение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ClaimUpload(context.Context, string, string, string, string) (domain.Attachment, error)
	// CompleteUpload сохраняет результат загрузки объекта только до тех пор, пока токен попытки
	// владения ещё действует.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg5: Подписанные учетные данные или токен владения операцией для проверки.
	//   - arg6: Ожидаемый дайджест содержимого, используемый для проверки целостности
	//   передачи.
	//
	// @returns:
	//   - result1: Результирующее вложение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CompleteUpload(context.Context, string, string, string, string, string) (domain.Attachment, error)
	// AbortUpload публикует только попытку загрузки, идентифицированную по текущему токену
	// владельца.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Подписанные учетные данные или токен владения операцией для проверки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	AbortUpload(context.Context, string, string) error
	// AttachmentForFinalize загружает видимые владельцу метаданные вложения, необходимые для
	// подтверждения его загрузки.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Результирующее вложение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	AttachmentForFinalize(context.Context, string, string, string) (domain.Attachment, error)
	// FinalizeAttachment помечает успешно загруженное вложение как готовое к использованию в
	// сообщении.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg5: Сгенерированный сервером закрытый ключ для хранения объектов.
	//
	// @returns:
	//   - result1: Результирующее вложение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	FinalizeAttachment(context.Context, string, string, string, string) (domain.Attachment, error)
	// DownloadAttachment загружает завершённое вложение только в том случае, если оно
	// принадлежит сообщению, доступному для вызывающей стороны.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Результирующее вложение.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	DownloadAttachment(context.Context, string, string, string) (domain.Attachment, error)
	// CleanupCandidates возвращает ограниченную выборку попыток загрузки вложений с истекшим
	// сроком действия, которые можно удалить.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: Собранные значения вложений.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CleanupCandidates(context.Context, int) ([]domain.Attachment, error)
	// CompleteCleanup фиксирует завершение очистки запрошенного вложения.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CompleteCleanup(context.Context, string, string) error
}

// Storage — хранилище определяет заменяемый контракт зависимостей для постоянных сообщений чата и личных
// вложений.
type Storage interface {
	// CheckAttachmentPrivacy отклоняет настройки объектного хранилища, которые раскрывают
	// приватные вложения чатов через публичную политику bucket.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CheckAttachmentPrivacy(context.Context) error
	// PutAttachment сохраняет байты вложений под сгенерированным сервером приватным ключом
	// объекта.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
	//   - arg3: Поток исходного контента или устройство чтения карт памяти, в зависимости
	//   от его типа.
	//   - arg4: Размер контента в байтах или запрошенная ограниченная емкость.
	//   - arg5: Объявленный или обнаруженный тип контента MIME.
	//   - arg6: Ожидаемый дайджест содержимого, используемый для проверки целостности
	//   передачи.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	PutAttachment(context.Context, string, io.Reader, int64, string, string) error
	// StatAttachment считывает фактический размер объекта и метаданные перед завершением
	// загрузки.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
	//
	// @returns:
	//   - result1: Возвращает количество, размер или числовое значение.
	//   - result2: Фактический тип хранимого содержимого объекта.
	//   - result3: Сохраненный дайджест содержимого SHA-256, используемый для завершения
	//   загрузки.
	//   - result4: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	StatAttachment(context.Context, string) (int64, string, string, error)
	// AttachmentDownloadURL создает недолговечный подписанный URL, который запрашивает загрузку
	// вложения.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
	//   - arg3: Безопасное имя файла для загрузки, указанное в заголовке
	//   Content-Disposition.
	//   - arg4: Срок действия или интервал действительности, определяемый типом.
	//
	// @returns:
	//   - result1: Недолговечный подписанный URL для загрузки вложения.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	AttachmentDownloadURL(context.Context, string, string, time.Duration) (string, error)
	// CleanAttachmentObjects удаляет только те объекты с префиксом вложения, которые не
	// являются сохраненным финализированным объектом.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Объект или префикс ключа, управляемый сервером.
	//   - arg3: Точный ключ финализированного объекта, который должен быть сохранен при
	//   очистке.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CleanAttachmentObjects(context.Context, string, string) error
}

// Events — события определяют заменяемый контракт зависимостей для постоянных сообщений в чате и
// приватных вложений.
type Events interface {
	// Функция Broadcast публикует доверенное событие только для тех участников конференции или
	// беседы, которые в данный момент имеют к ней доступ.
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
	// SendToParticipant публикует доверенное событие для физических сеансов выбранного
	// авторизованного участника.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий видимость события.
	//   - arg3: Идентификатор учетной записи предполагаемого участника.
	//   - arg4: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SendToParticipant(context.Context, string, string, realtime.Envelope) error
}

// Service — сервис координирует постоянные сообщения в чате и приватные вложения через настроенные
// зависимости сохранения и передачи данных.
type Service struct {
	repo    Repository
	storage Storage
	events  Events
	uploads chan struct{}
}

// NewService создает сервис с зависимостями и ограничениями, необходимыми для работы с
// постоянными сообщениями в чате и приватными вложениями.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - storage: Зависимость от приватного объекта или хранилища файлов.
//   - events: Место назначения для зафиксированных событий жизненного цикла или чатов.
//
// @returns:
//   - result1: Созданный или загруженный сервис.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewService(ctx context.Context, repo Repository, storage Storage, events Events) (*Service, error) {
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := storage.CheckAttachmentPrivacy(check); err != nil {
		return nil, err
	}
	return &Service{repo: repo, storage: storage, events: events, uploads: make(chan struct{}, 4)}, nil
}

// List возвращает страницу с ограниченным количеством записей о сообщениях чата с
// использованием заданных параметров разбивки на страницы и фильтров.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) List(ctx context.Context, user, conference, cursor string, limit int) (domain.Page, error) {
	return s.repo.List(ctx, user, conference, cursor, limit)
}

// Send проверяет ссылки на доступ, ответы и вложения и сохраняет сообщение чата с защитой от
// повторяющихся идентичных запросов.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Полученное сообщение.
//   - result2: Указывает, создал ли этот запрос новое состояние или повторно использовал
//     идемпотентный результат.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Send(ctx context.Context, user, conference string, request domain.SendRequest) (domain.Message, bool, error) {
	request, fingerprint, err := domain.NormalizeSend(request)
	if err != nil {
		return domain.Message{}, false, err
	}
	message, created, err := s.repo.Send(ctx, user, conference, request, fingerprint)
	if err == nil && created {
		s.publish(ctx, "chat.message.created", message)
	}
	return message, created, err
}

// Edit — при редактировании существующий текст сообщения автора изменяется после проверки видимости и
// возможности редактирования.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Полученное сообщение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Edit(ctx context.Context, user, conference, id string, request domain.EditRequest) (domain.Message, error) {
	text, err := domain.NormalizeText(request.Text, true)
	if err != nil {
		return domain.Message{}, err
	}
	message, err := s.repo.Edit(ctx, user, conference, id, text)
	if err == nil {
		s.publish(ctx, "chat.message.updated", message)
	}
	return message, err
}

// Delete удаляет запрошенное сообщение чата с учётом области видимости вызывающей стороны и
// правил жизненного цикла ресурса.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученное сообщение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Delete(ctx context.Context, user, conference, id string) (domain.Message, error) {
	message, err := s.repo.Delete(ctx, user, conference, id)
	if err == nil {
		s.publish(ctx, "chat.message.deleted", message)
	}
	return message, err
}

// Функция publish публикует изменения в сохраненных сообщениях чата через настроенный транспорт
// событий или команд.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - message: Сохраненное или отправленное сообщение в чате.
func (s *Service) publish(ctx context.Context, kind string, message domain.Message) {
	if s.events != nil {
		if err := s.events.Broadcast(ctx, realtime.Event(kind, message.ConferenceID, message)); err != nil {
			slog.Warn("chat realtime delivery deferred to client refresh", "conference_id", message.ConferenceID, "event_type", kind)
		}
	}
}

// ReadState возвращает сохраненную границу прочитанных сообщений участника и количество видимых
// непрочитанных сообщений.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Полученное состояние чтения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) ReadState(ctx context.Context, user, conference string) (domain.ReadState, error) {
	return s.repo.ReadState(ctx, user, conference)
}

// MarkRead перемещает сохраненную границу прочитанных сообщений, не позволяя поздним или
// повторным запросам сдвигать ее назад.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Полученное состояние чтения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) MarkRead(ctx context.Context, user, conference string, request domain.ReadRequest) (domain.ReadState, error) {
	id, err := domain.UUID(request.MessageID)
	if err != nil {
		return domain.ReadState{}, err
	}
	state, err := s.repo.MarkRead(ctx, user, conference, id)
	if err == nil && s.events != nil {
		_ = s.events.SendToParticipant(ctx, conference, state.ParticipantID, realtime.Event("chat.read.updated", conference, state))
	}
	return state, err
}

// InitAttachment создаёт или повторно использует ожидающие обработки метаданные вложений, чтобы
// можно было безопасно повторить загрузку.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Результирующее вложение.
//   - result2: Указывает, создал ли этот запрос новое состояние или повторно использовал
//     идемпотентный результат.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) InitAttachment(ctx context.Context, user, conference string, request domain.InitRequest) (domain.Attachment, bool, error) {
	request, err := domain.NormalizeInit(request)
	if err != nil {
		return domain.Attachment{}, false, err
	}
	return s.repo.InitAttachment(ctx, user, conference, request)
}

// ValidateContent проверяет байты, размер, дайджест и фактический формат медиафайла во
// вложении, не полагаясь на заявленные метаданные.
//
// @Parameters:
//   - data: Входные байты или структурированная полезная информация о событии.
//   - attachment: Идентификатор личного вложения или метаданные, в зависимости от его типа.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func ValidateContent(data []byte, attachment domain.Attachment) error {
	if int64(len(data)) != attachment.Size || len(data) == 0 || int64(len(data)) > domain.MaxAttachmentBytes {
		return apperrors.New(apperrors.ErrInvalidInput, "uploaded size differs from the declared attachment size")
	}
	canonical, allowed := domain.AttachmentMIMEType(attachment.Filename)
	if !allowed || canonical != attachment.MimeType {
		return apperrors.New(apperrors.ErrInvalidInput, "filename and MIME type disagree")
	}
	detected := strings.Split(http.DetectContentType(data), ";")[0]
	expected := attachment.MimeType
	if expected == "text/csv" || expected == "text/tab-separated-values" || expected == "text/markdown" {
		expected = "text/plain"
	}
	standard := expected == "image/jpeg" || expected == "image/png" || expected == "image/webp" || expected == "application/pdf" || expected == "text/plain"
	if (standard && detected != expected) || (!standard && !documentOrArchiveContent(data, expected)) {
		return apperrors.New(apperrors.ErrInvalidInput, "file content does not match its allowed type")
	}
	if expected == "text/plain" && (!utf8.Valid(data) || bytes.ContainsRune(data, 0)) {
		return apperrors.New(apperrors.ErrInvalidInput, "text attachments must be valid UTF-8")
	}
	if expected == "image/jpeg" || expected == "image/png" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
			return apperrors.New(apperrors.ErrInvalidInput, "invalid image or image dimensions exceed the limit")
		}
	}
	return nil
}

// Upload проверяет поток ограниченного контента и сохраняет его для текущей попытки загрузки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - reader: Поток исходного контента или устройство чтения карт памяти, в зависимости от его
//     типа.
//
// @returns:
//   - result: Результирующее вложение.
//   - err: nil — в случае успеха; в противном случае — ошибка валидации, доступа, передачи
//     данных или сохранения.
func (s *Service) Upload(ctx context.Context, user, conference, id string, reader io.Reader) (result domain.Attachment, err error) {
	select {
	case s.uploads <- struct{}{}:
		defer func() { <-s.uploads }()
	default:
		return result, apperrors.New(apperrors.ErrUnavailable, "attachment upload capacity is busy")
	}
	op, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	token := uuid.NewString()
	a, err := s.repo.ClaimUpload(op, user, conference, id, token)
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil && a.UploadedAt == nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.repo.AbortUpload(cleanup, id, token)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(reader, domain.MaxAttachmentBytes+1))
	if err != nil {
		return result, apperrors.New(apperrors.ErrInvalidInput, "attachment upload was interrupted")
	}
	if err = ValidateContent(data, a); err != nil {
		return result, err
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	if a.UploadedAt != nil {
		if a.Checksum != checksum {
			return result, apperrors.New(apperrors.ErrConflict, "attachment already contains different bytes")
		}
		return a, nil
	}
	if err = s.storage.PutAttachment(op, a.ObjectKey, bytes.NewReader(data), int64(len(data)), a.MimeType, checksum); err != nil {
		return result, apperrors.ErrUnavailable
	}
	return s.repo.CompleteUpload(op, user, conference, id, token, checksum)
}

// FinalizeAttachment помечает успешно загруженное вложение как готовое к использованию в
// сообщении.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Результирующее вложение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) FinalizeAttachment(ctx context.Context, user, conference, id string) (domain.Attachment, error) {
	a, err := s.repo.AttachmentForFinalize(ctx, user, conference, id)
	if err != nil {
		return a, err
	}
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	size, mime, checksum, err := s.storage.StatAttachment(op, a.ObjectKey)
	if err != nil {
		return domain.Attachment{}, apperrors.ErrUnavailable
	}
	if size != a.Size || mime != a.MimeType || checksum != a.Checksum {
		return domain.Attachment{}, apperrors.New(apperrors.ErrConflict, "stored attachment validation failed")
	}
	return s.repo.FinalizeAttachment(ctx, user, conference, id, a.ObjectKey)
}

// Download проверяет видимость вложения и возвращает недолговечный приватный объект для
// скачивания URL.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conference: Идентификатор конференции, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Временный подписанный объект для скачивания URL для авторизованного
//     приватного вложения.
//   - result2: Временная метка или граница допустимого срока действия, связанная с
//     результатом.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Download(ctx context.Context, user, conference, id string) (string, time.Time, error) {
	a, err := s.repo.DownloadAttachment(ctx, user, conference, id)
	if err != nil {
		return "", time.Time{}, err
	}
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url, err := s.storage.AttachmentDownloadURL(op, a.ObjectKey, a.Filename, domain.DownloadTTL)
	if err != nil {
		return "", time.Time{}, apperrors.ErrUnavailable
	}
	return url, time.Now().UTC().Add(domain.DownloadTTL), nil
}

// Cleanup — устраняет незавершённые загрузки вложений с истекшим сроком действия и удаляет только
// принадлежащие им приватные объекты.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Cleanup(ctx context.Context) error {
	rows, err := s.repo.CleanupCandidates(ctx, 100)
	if err != nil {
		return err
	}
	for _, a := range rows {
		keep := ""
		if a.Status == "attached" {
			keep = a.ObjectKey
		}
		if err := s.storage.CleanAttachmentObjects(ctx, a.Prefix(), keep); err != nil {
			return err
		}
		if err := s.repo.CompleteCleanup(ctx, a.ID, a.ObjectKey); err != nil {
			return err
		}
	}
	return nil
}

// Run обрабатывает постоянные сообщения чата и приватные вложения до тех пор, пока контекст не
// будет отменён или пока не произойдёт сбой в обработке.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		op, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.Cleanup(op)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("attachment orphan cleanup will retry")
		}
	}
}

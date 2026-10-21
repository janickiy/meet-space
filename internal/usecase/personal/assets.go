// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package personal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	domain "github.com/janickiy/meet-space/internal/domain/personal"
)

const MaxAvatarBytes int64 = 2 << 20
const AssetStreamTimeout = 30 * time.Second

// Asset — ключи активов — это данные, доступные только на сервере, и они никогда не являются частью
// публичного DTO.
type Asset struct {
	Version, ConversationID, Key, ContentType, Filename, Checksum string
	Size                                                          int64
	CleanupToken                                                  string
}

type AssetRepository interface {
	// BeginAvatar создает ожидающую публикации публикацию с аватаром после блокировки
	// авторизованной группы.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg4: Личные метаданные аватара или вложения.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	BeginAvatar(context.Context, string, string, Asset) error
	// ActivateAvatar атомарно делает текущий загруженный аватар видимым в групповом чате.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg4: Версия или значение ограждения, используемое для предотвращения
	//   устаревания прав собственности.
	//
	// @returns:
	//   - result1: Полученная беседа.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ActivateAvatar(context.Context, string, string, string) (domain.Conversation, error)
	// ClearAvatar очищает активные метаданные аватара группы и определяет устаревшие объекты
	// для удаления.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Полученная беседа.
	//   - result2: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ClearAvatar(context.Context, string, string) (domain.Conversation, bool, error)
	// GroupAttachment загружает завершенное вложение только в том случае, если на него
	// ссылается видимое групповое сообщение.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор чата, ограничивающий область действия операции.
	//   - arg4: Идентификатор ресурса, к которому обращается данная операция.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result2: Результирующий актив.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GroupAttachment(context.Context, string, string, string) (bool, Asset, error)
	// VisitAsset выполняет операцию с активом в рамках области видимости, пока актив и его
	// идентификационные данные действительны.
	//
	// Обратный вызов выполняется под блокировкой общего доступа к диалогу.  Мутации членства
	// блокируют доступ к диалогу, поэтому удаление не может быть выполнено в процессе.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор аккаунта, запрашивающего приватный актив.
	//   - arg3: Идентификатор группового диалога, содержащего актив.
	//   - arg4: Идентификатор вложения или селектор аватара, ожидаемый репозиторием.
	//   - arg5: Обратный вызов вызывается до тех пор, пока сохраняется право владения и
	//   авторизации на использование ресурсов операции.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	VisitAsset(context.Context, string, string, string, func(Asset) error) error
	// AvatarCleanupCandidates возвращает пакет устаревших объектов групповых аватаров.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: Собранные строковые значения.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	AvatarCleanupCandidates(context.Context, int) ([]string, error)
	// ClaimAvatarCleanup запрашивает один устаревший объект-аватар, чтобы параллельные рабочие
	// процессы не могли удалить текущий аватар.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Версия или значение ограждения, используемое для предотвращения
	//   устаревания прав собственности.
	//
	// @returns:
	//   - result1: Созданный или загруженный ресурс.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ClaimAvatarCleanup(context.Context, string) (*Asset, error)
	// FinishAvatarCleanup записывает, был ли успешно удалён запрошенный объект-аватар.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Значение актива, требуемое контрактом.
	//   - arg3: Успешно ли завершена очистка запрошенного объекта.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	FinishAvatarCleanup(context.Context, Asset, bool) error
}
type AssetStorage interface {
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
	// PutPersonalAvatar сохраняет нормализованные байты аватара под ключом для приватных
	// объектов, принадлежащих серверу, для группового аватара.
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
	PutPersonalAvatar(context.Context, string, io.Reader, int64, string, string) error
	// OpenPersonalAsset открывает приватный групповой аватар или вложение с фактическими
	// метаданными объекта.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
	//
	// @returns:
	//   - result1: Результирующий считывающий модуль.
	//   - result2: Возвращает количество, размер или числовое значение.
	//   - result3: Обнаружен тип содержимого ресурса.
	//   - result4: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	OpenPersonalAsset(context.Context, string) (io.ReadCloser, int64, string, error)
	// DeletePersonalAvatar удаляет только приватный объект, идентифицируемый по устаревшему
	// ключу аватара.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	DeletePersonalAvatar(context.Context, string) error
}
type AssetEvents interface {
	// PublishConversation публикует зафиксированное изменение в диалоге для авторизованных
	// текущих участников.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Подтвержденное событие, задание, категория медиаданных или ресурсов.
	//   - arg4: Входные байты или структурированная полезная информация о событии.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	PublishConversation(context.Context, string, string, any) error
}
type AssetService struct {
	repo             AssetRepository
	storage          AssetStorage
	events           AssetEvents
	uploads, streams chan struct{}
}

// NewAssetService создает сервис управления активами с зависимостями и ограничениями,
// необходимыми для личных и групповых диалогов.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - storage: Зависимость от приватного объекта или хранилища файлов.
//   - events: Место назначения для зафиксированных событий жизненного цикла или чатов.
//
// @returns:
//   - result1: Созданный или загруженный сервис управления активами.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewAssetService(ctx context.Context, repo AssetRepository, storage AssetStorage, events AssetEvents) (*AssetService, error) {
	if repo == nil || storage == nil {
		return nil, fmt.Errorf("personal assets dependencies are required")
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := storage.CheckAttachmentPrivacy(check); err != nil {
		return nil, err
	}
	return &AssetService{repo: repo, storage: storage, events: events, uploads: make(chan struct{}, 2), streams: make(chan struct{}, 8)}, nil
}

// assetIDs проверяет идентификаторы учетной записи и диалога перед выполнением частных операций
// с активами.
//
// @Parameters:
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func assetIDs(user, conversation string) error {
	if _, err := chat.UUID(user); err != nil {
		return err
	}
	_, err := chat.UUID(conversation)
	return err
}

// acquireAssetSlot ожидает выделенного слота для обработки активов или завершает работу при
// отмене контекста.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - slots: Ограниченный семафор, контролирующий параллельную обработку активов.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func acquireAssetSlot(ctx context.Context, slots chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case slots <- struct{}{}:
		return nil
	default:
		return apperrors.ErrRateLimited
	}
}

// PutAvatar проверяет и сохраняет нормализованный групповой аватар, а затем публикует
// зафиксированное изменение в диалоге.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
//   - contentType: Объявленный или обнаруженный тип контента MIME.
//   - body: Поток входного контента или сериализованное тело запроса.
//
// @returns:
//   - result1: Полученная беседа.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *AssetService) PutAvatar(ctx context.Context, user, conversation, contentType string, body io.Reader) (domain.Conversation, error) {
	if err := assetIDs(user, conversation); err != nil {
		return domain.Conversation{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, AssetStreamTimeout)
	defer cancel()
	if err := acquireAssetSlot(ctx, s.uploads); err != nil {
		return domain.Conversation{}, err
	}
	defer func() { <-s.uploads }()
	data, typ, err := normalizeAvatar(body, contentType)
	if err != nil {
		return domain.Conversation{}, err
	}
	version := uuid.NewString()
	sum := sha256.Sum256(data)
	asset := Asset{Version: version, ConversationID: conversation, Key: "avatars/conversations/" + conversation + "/" + version, ContentType: typ, Size: int64(len(data)), Checksum: hex.EncodeToString(sum[:])}
	// Сохраняется перед загрузкой: в случае сбоя или неоднозначного ответа хранилища объектов
	// остается устойчивая ожидающая обработки строка, которую может восстановить процесс
	// очистки.
	if err = s.repo.BeginAvatar(ctx, user, conversation, asset); err != nil {
		return domain.Conversation{}, err
	}
	if err = s.storage.PutPersonalAvatar(ctx, asset.Key, bytes.NewReader(data), asset.Size, typ, asset.Checksum); err != nil {
		return domain.Conversation{}, apperrors.Wrap(apperrors.ErrUnavailable, err, "avatar upload failed")
	}
	item, err := s.repo.ActivateAvatar(ctx, user, conversation, version)
	if err != nil {
		return domain.Conversation{}, err
	}
	s.changed(ctx, conversation)
	return item, nil
}

// DeleteAvatar очищает авторизованный групповой аватар и планирует очистку устаревших частных
// объектов.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
//
// @returns:
//   - result1: Полученная беседа.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *AssetService) DeleteAvatar(ctx context.Context, user, conversation string) (domain.Conversation, error) {
	if err := assetIDs(user, conversation); err != nil {
		return domain.Conversation{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, AssetStreamTimeout)
	defer cancel()
	item, changed, err := s.repo.ClearAvatar(ctx, user, conversation)
	if err != nil {
		return domain.Conversation{}, err
	}
	if changed {
		s.changed(ctx, conversation)
	}
	return item, nil
}

// changed уведомляет наблюдателя компонента об изменениях в локальном или сохранённом
// состоянии.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
func (s *AssetService) changed(ctx context.Context, conversation string) {
	if s.events != nil {
		_ = s.events.PublishConversation(ctx, conversation, "conversation.updated", map[string]any{"conversationId": conversation, "type": "group"})
	}
}

// normalizeAvatar декодирует ограниченное изображение и перекодирует нормализованный аватар, не
// полагаясь на метаданные загрузки.
//
// @Parameters:
//   - body: Поток входного контента или сериализованное тело запроса.
//   - contentType: Объявленный или обнаруженный тип контента MIME.
//
// @returns:
//   - result1: Собранные байтовые значения.
//   - result2: Канонический тип содержимого изображения после перекодирования.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func normalizeAvatar(body io.Reader, contentType string) ([]byte, string, error) {
	typ := strings.TrimSpace(strings.Split(contentType, ";")[0])
	if typ != "image/jpeg" && typ != "image/png" {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "avatar must be JPEG or PNG")
	}
	raw, err := io.ReadAll(io.LimitReader(body, MaxAvatarBytes+1))
	if err != nil {
		return nil, "", apperrors.ErrInvalidInput
	}
	if len(raw) == 0 || int64(len(raw)) > MaxAvatarBytes {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "avatar exceeds 2 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") || "image/"+format != typ || config.Width < 1 || config.Height < 1 || config.Width > 2048 || config.Height > 2048 || int64(config.Width)*int64(config.Height) > 4<<20 {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "invalid avatar image or dimensions")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", apperrors.ErrInvalidInput
	}
	// При каноническом перекодировании удаляются встроенные метаданные и завершающие данные, а
	// также полиглоты.
	var out avatarBuffer
	if format == "jpeg" {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&out, img)
	}
	if err != nil || int64(out.Len()) > MaxAvatarBytes {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "encoded avatar exceeds 2 MiB")
	}
	return out.Bytes(), typ, nil
}

// При кодировании сжатый входной файл может увеличиться в размере.  Ограничьте объем выделяемой
// памяти, а также размеры входного и декодированного файлов.
type avatarBuffer struct{ bytes.Buffer }

// Функция Write принимает выходные байты через контракт io.Writer, соблюдая правила буферизации
// компонента.
//
// @Parameters:
//   - data: Входные байты или структурированная полезная информация о событии.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (b *avatarBuffer) Write(data []byte) (int, error) {
	if int64(b.Len()+len(data)) > MaxAvatarBytes {
		return 0, apperrors.ErrInvalidInput
	}
	return b.Buffer.Write(data)
}

// GroupDownload возвращает ошибку URL, не связанную с возможностями; при каждом запросе байта
// происходит повторная авторизация.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
//   - attachment: Идентификатор личного вложения или метаданные, в зависимости от его типа.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: Временная авторизованная загрузка прикрепленного файла URL.
//   - result3: Временная метка или граница допустимого срока действия, связанная с
//     результатом.
//   - result4: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *AssetService) GroupDownload(ctx context.Context, user, conversation, attachment string) (bool, string, time.Time, error) {
	if err := assetIDs(user, conversation); err != nil {
		return true, "", time.Time{}, err
	}
	if _, err := chat.UUID(attachment); err != nil {
		return true, "", time.Time{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	handled, _, err := s.repo.GroupAttachment(ctx, user, conversation, attachment)
	if !handled || err != nil {
		return handled, "", time.Time{}, err
	}
	return true, "/api/v1/conversations/" + conversation + "/attachments/" + attachment + "/content", time.Now().UTC().Add(5 * time.Minute), nil
}

// Stream сохраняет свежую авторизацию членства и блокировку общего доступа к репозиторию до тех
// пор, пока не будет закрыт объект чтения.  Пустое прикрепленное изображение выбирает текущий
// аватар.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
//   - attachment: Идентификатор личного вложения или метаданные, в зависимости от его типа.
//   - deliver: Обратный вызов доставки вызывается только до тех пор, пока действителен
//     получатель или авторизация ресурса.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *AssetService) Stream(ctx context.Context, user, conversation, attachment string, deliver func(Asset, io.Reader) error) error {
	if err := assetIDs(user, conversation); err != nil {
		return err
	}
	if attachment != "" {
		if _, err := chat.UUID(attachment); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, AssetStreamTimeout)
	defer cancel()
	if err := acquireAssetSlot(ctx, s.streams); err != nil {
		return err
	}
	defer func() { <-s.streams }()
	return s.repo.VisitAsset(ctx, user, conversation, attachment, func(asset Asset) error {
		body, size, typ, err := s.storage.OpenPersonalAsset(ctx, asset.Key)
		if err != nil {
			return apperrors.Wrap(apperrors.ErrUnavailable, err, "asset is unavailable")
		}
		defer body.Close()
		if size != asset.Size || size < 1 || (attachment == "" && typ != asset.ContentType) {
			return apperrors.ErrUnavailable
		}
		return deliver(asset, io.LimitReader(body, asset.Size))
	})
}

// CleanAvatars запрашивает устаревшие объекты аватаров и удаляет только те ресурсы, которые
// больше не используются группой.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *AssetService) CleanAvatars(ctx context.Context) error {
	ids, err := s.repo.AvatarCleanupCandidates(ctx, 25)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		asset, err := s.repo.ClaimAvatarCleanup(ctx, id)
		if err != nil {
			return err
		}
		if asset == nil {
			continue
		}
		operation, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = s.storage.DeletePersonalAvatar(operation, asset.Key)
		cancel()
		if finishErr := s.repo.FinishAvatarCleanup(ctx, *asset, err == nil); finishErr != nil {
			return finishErr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Run ведет личные и групповые беседы до тех пор, пока контекст не будет отменен или пока не
// произойдет сбой обработки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (s *AssetService) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		operation, cancel := context.WithTimeout(ctx, 20*time.Second)
		_ = s.CleanAvatars(operation)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

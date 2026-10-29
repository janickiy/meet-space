// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/infrastructure/composite"
	"github.com/janickiy/meet-space/internal/infrastructure/ffmpeg"
	s3storage "github.com/janickiy/meet-space/internal/infrastructure/storage/s3"
	"github.com/janickiy/meet-space/internal/operations"
)

// CompositeRepository определяет заменяемый контракт зависимостей для записи заданий и их
// артефактов.
type CompositeRepository interface {
	// FindByUUID загружает запись по ее публичному идентификатору UUID.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//
	// @returns:
	//   - result1: Результирующая запись.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	FindByUUID(context.Context, string) (records.Record, error)
	// ListActiveComposite возвращает активные составные записи, чтобы работник мог сверить их
	// или восстановить процесс обработки.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: Собранные значения записей.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ListActiveComposite(context.Context) ([]records.Record, error)
	// ClaimComposite запрашивает аренду составной записи с версионным доступом для одного
	// работника.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Подписанные учетные данные или токен владения операцией для проверки.
	//   - arg4: Идентификатор работника, которому принадлежит эта операция.
	//   - arg5: Срок действия, назначенный для временного значения или записи о
	//   владельце.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ClaimComposite(context.Context, string, string, string, time.Duration) (bool, error)
	// RenewComposite продлевает аренду составной записи только в том случае, если её работник и
	// версия аренды по-прежнему совпадают.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Подписанные учетные данные или токен владения операцией для проверки.
	//   - arg4: Срок действия, назначенный для временного значения или записи о
	//   владельце.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	RenewComposite(context.Context, string, string, time.Duration) (bool, error)
	// ReleaseComposite освобождает только ту аренду составной записи, которой он владеет в
	// данный момент.
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
	ReleaseComposite(context.Context, string, string) error
	// TransitionComposite изменяет состояние составной записи только для текущей аренды.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Подписанные учетные данные или токен владения операцией для проверки.
	//   - arg4: Состояние жизненного цикла или статус ответа на запись.
	//   - arg5: Основная ошибка, из-за которой операция завершилась неудачно.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	TransitionComposite(context.Context, string, string, string, error) error
	// SaveCompositeArtifacts коммитит составные артефакты только для той генерации записи,
	// которой по-прежнему владеет работник.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор ресурса, к которому обращается данная операция.
	//   - arg3: Подписанные учетные данные или токен владения операцией для проверки.
	//   - arg4: Определяет, является ли этот файл последним и нужно ли закрыть архив.
	//   - arg5: Необязательные метаданные файла предварительного просмотра, связанные с
	//   записью.
	//   - arg6: Упорядоченные сегменты медиаданных.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SaveCompositeArtifacts(context.Context, string, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error
	// MarkStopping условно переводит запись в состояние остановки, не перезаписывая более новый
	// переход жизненного цикла.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//   - arg3: Ограниченный жизненный цикл или причина сбоя.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	MarkStopping(context.Context, string, string) error
	// AddEvent добавляет событие диагностики жизненного цикла постоянной записи.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Внешняя запись UUID.
	//   - arg3: Ограниченная категория событий жизненного цикла записи.
	//   - arg4: Медиа или источник контента, связанный с текущей операцией.
	//   - arg5: Ограниченная степень серьезности диагностики.
	//   - arg6: Сохраненное или отправленное сообщение в чате.
	//   - arg7: Идентификатор работника, которому принадлежит эта операция.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	AddEvent(context.Context, string, string, string, string, string, string) error
}

// CompositeRegistry определяет заменяемый контракт зависимостей для записи заданий и их
// артефактов.
type CompositeRegistry interface {
	// GetOwner загружает текущую версию медиакомнаты с распределенным владельцем и защитой от
	// несанкционированного доступа.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Результирующий маршрут.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	GetOwner(context.Context, string) (media.Route, error)
}

// CompositeOptions содержит зависимости и настройки составной записи конференции.
type CompositeOptions struct {
	Repository     CompositeRepository
	Registry       CompositeRegistry
	S3             *s3storage.Client
	StoragePath    string
	FFmpegPath     string
	WorkerID       string
	InternalSecret string
	Config         config.CompositeConfig
	ConferenceLock conferenceReleaser
	// Вызовите метод Publish после завершения перехода жизненного цикла. Он должен ограничивать
	// ввод-вывод и не должен повторно обращаться к этому сервису.
	Publish func(context.Context, records.Record, string) error
	Logger  *log.Logger
}

// CompositeService управляет записью независимо от времени доставки AMQP. Аренда базы данных
// защищает процессы приема данных, обработки FFmpeg и публикации артефактов.
type CompositeService struct {
	o         CompositeOptions
	composer  *composite.Composer
	processor *ffmpeg.PostProcessor
	artifacts compositeArtifactPublisher
	client    *http.Client
	mu        sync.Mutex
	ctx       context.Context
	running   map[string]struct{}
	wg        sync.WaitGroup
	started   bool
	draining  bool
}

// NewCompositeService создает составной сервис с зависимостями и ограничениями, необходимыми
// для записи заданий и их артефактов.
//
// @Parameters:
//   - o: Зависимости компонентов и параметры создания.
//
// @returns:
//   - result1: Созданный или загруженный составной сервис.
func NewCompositeService(o CompositeOptions) *CompositeService {
	if o.Logger == nil {
		o.Logger = log.Default()
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 5 * time.Second
	artifacts := compositeArtifactPublisher{repository: o.Repository, maxBytes: o.Config.MaxBytes}
	if o.S3 != nil {
		artifacts.storage = o.S3
	}
	return &CompositeService{o: o, composer: composite.NewComposer(o.FFmpegPath, o.Config.Width, o.Config.Height, o.Config.FPS, o.Config.Concurrency), processor: ffmpeg.NewPostProcessor(o.FFmpegPath), artifacts: artifacts, client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, ctx: context.Background(), running: map[string]struct{}{}}
}

// Start запускает запись заданий и их артефактов, а также подготавливает ресурсы, необходимые
// для выполнения этой операции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (s *CompositeService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.ctx = ctx
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.o.Config.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.reconcile(ctx)
			}
		}
	}()
}

// Wait ожидает завершения фоновых задач, принадлежащих компоненту, и освобождает оставшиеся
// транспортные ресурсы.
func (s *CompositeService) Wait() { s.wg.Wait(); s.client.CloseIdleConnections() }

// Active возвращает количество операций, принадлежащих компоненту, которые необходимо завершить
// до завершения работы.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
func (s *CompositeService) Active() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.running) }

// BeginDrain прекращает принимать новые задачи, пока не завершатся текущие операции.
func (s *CompositeService) BeginDrain() { s.mu.Lock(); s.draining = true; s.mu.Unlock() }

// WaitContext ограничивает время ожидания ранее остановленных задач указанным предельным
// временем; владелец должен запросить отмену перед вызовом этого метода.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *CompositeService) WaitContext(ctx context.Context) error {
	done := make(chan struct{})
	go func() { s.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// HandleCommand идемпотентно обрабатывает внутренние команды на запуск или остановку записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - command: Подтвержденная команда внутренней записи или передачи медиаданных.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *CompositeService) HandleCommand(ctx context.Context, command records.Command, record records.Record) error {
	switch command.Type {
	case "record.start":
		if records.IsTerminalStatus(record.Status) {
			return nil
		}
		return s.launch(ctx, record)
	case "record.stop":
		if records.IsTerminalStatus(record.Status) {
			return nil
		}
		if record.Status == records.StatusStarting || record.Status == records.StatusRecording {
			if err := s.o.Repository.MarkStopping(ctx, record.UUID, command.Reason); err != nil && !errors.Is(err, records.ErrRecordStateChanged) {
				return err
			}
			record.Status = records.StatusStopping
			s.publish(ctx, record, "recording.stopping")
		}
		// Команда может быть доставлена на другую реплику. Владелец отслеживает остановку,
		// опрашивая базу данных; запуск разрешен только в том случае, если текущий владелец
		// отсутствует.
		return s.launch(ctx, record)
	default:
		return fmt.Errorf("unknown composite command")
	}
}

// reconcile — примирительная процедура сверяет сохраненное состояние записи с текущими ресурсами после
// сбоев или перезапуска.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (s *CompositeService) reconcile(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	records, err := s.o.Repository.ListActiveComposite(queryCtx)
	if err != nil {
		return
	}
	for _, record := range records {
		if err := s.launch(queryCtx, record); err != nil {
			s.o.Logger.Printf("composite reconciliation record=%s failed", record.UUID)
		}
	}
}

// launch — запуск обработки составной записи начинается только после получения защищенной аренды.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *CompositeService) launch(ctx context.Context, record records.Record) error {
	s.mu.Lock()
	if _, ok := s.running[record.UUID]; ok || s.draining || len(s.running) >= s.o.Config.MaxActive || s.ctx.Err() != nil {
		s.mu.Unlock()
		return nil
	}
	s.running[record.UUID] = struct{}{}
	workParent := s.ctx
	s.wg.Add(1)
	s.mu.Unlock()
	token := uuid.NewString()
	claimed, err := s.o.Repository.ClaimComposite(ctx, record.UUID, token, s.o.WorkerID, s.o.Config.LeaseTTL)
	if err != nil || !claimed {
		s.mu.Lock()
		delete(s.running, record.UUID)
		s.mu.Unlock()
		s.wg.Done()
		return err
	}
	go func() {
		defer s.wg.Done()
		defer func() { s.mu.Lock(); delete(s.running, record.UUID); s.mu.Unlock() }()
		workCtx, cancel := context.WithCancel(workParent)
		defer cancel()
		renewed := make(chan struct{})
		go func() {
			defer close(renewed)
			ticker := time.NewTicker(s.o.Config.LeaseTTL / 4)
			defer ticker.Stop()
			for {
				select {
				case <-workCtx.Done():
					return
				case <-ticker.C:
					checkCtx, done := context.WithTimeout(workCtx, s.o.Config.LeaseTTL/8)
					ok, err := s.o.Repository.RenewComposite(checkCtx, record.UUID, token, s.o.Config.LeaseTTL)
					done()
					if err != nil || !ok {
						cancel()
						return
					}
				}
			}
		}()
		err := s.run(workCtx, record, token)
		if err != nil {
			s.fail(record, token, err)
		}
		cancel()
		<-renewed
		releaseCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		_ = s.o.Repository.ReleaseComposite(releaseCtx, record.UUID, token)
	}()
	return nil
}

// run — запускайте задания по записи и работу с их артефактами до тех пор, пока контекст не будет
// отменен или пока не произойдет сбой в процессе.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *CompositeService) run(ctx context.Context, record records.Record, token string) error {
	started := time.Now()
	defer func() { operations.Observe("recording", time.Since(started).Seconds()) }()
	dir := filepath.Join(s.o.StoragePath, "records", record.UUID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if record.Status == records.StatusRecording || (record.Status == records.StatusStarting && record.RecorderToken != nil) {
		return fmt.Errorf("recorder interrupted; completed chunks retained for recovery")
	}
	if record.Status == records.StatusStarting {
		if err := s.capture(ctx, record, token, dir); err != nil {
			return err
		}
	}
	current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
	if err != nil {
		return err
	}
	if current.Status != records.StatusStopping && current.Status != records.StatusFinalizing && current.Status != records.StatusUploading {
		return fmt.Errorf("recording input ended without a stop transition")
	}
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusFinalizing, nil); err != nil {
		return err
	}
	current.Status = records.StatusFinalizing
	s.publish(ctx, current, "recording.processing")
	if err := s.composer.Recover(ctx, dir); err != nil {
		return err
	}
	finalizationStart := time.Now()
	var result ffmpeg.Result
	audioOnly := record.Mode == records.ModeAudioOnly || record.Mode == records.ModeIndividualTracks
	if audioOnly {
		result, err = s.processor.FinalizeAudio(ctx, dir)
	} else {
		result, err = s.processor.FinalizeComposite(ctx, dir)
	}
	operations.Observe("finalization", time.Since(finalizationStart).Seconds())
	if err != nil {
		return err
	}
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusUploading, nil); err != nil {
		return err
	}
	publication := s.artifacts.begin(compositeArtifactCommand{record: record, token: token, dir: dir, result: result})
	defer publication.Close()
	if err := publication.Publish(ctx); err != nil {
		return err
	}
	current, err = s.o.Repository.FindByUUID(ctx, record.UUID)
	operations.Event("recording_ready")
	operations.Event("recording_mode_" + record.Mode)
	if err == nil {
		s.publish(ctx, current, "recording.ready")
	}
	if s.o.ConferenceLock != nil {
		_ = s.o.ConferenceLock.Release(ctx, record.ConferenceID, record.UUID)
	}
	_ = s.o.Repository.AddEvent(ctx, record.UUID, "recording.ready", "recorder", "info", "Validated composite MP4 and preview uploaded", s.o.WorkerID)
	// Сохраняйте сегменты из неудачных или прерванных записей.  После защищенного аренды
	// коммита в состоянии готовности приватный MinIO становится полномочным, и временные файлы
	// могут быть удалены.  KEEP_LOCAL предназначен для диагностики и приемочных проверок.
	if !s.o.Config.KeepLocal {
		if err := os.RemoveAll(dir); err != nil {
			s.o.Logger.Printf("composite local cleanup failed record=%s", record.UUID)
		}
	}
	return nil
}

// capture — захват архивирует закодированный медиапоток для надежного восстановления и последующей
// компоновки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//   - dir: Проверенный локальный каталог.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *CompositeService) capture(ctx context.Context, record records.Record, token, dir string) error {
	captureCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	route, err := s.o.Registry.GetOwner(captureCtx, record.ConferenceID)
	if err != nil {
		return fmt.Errorf("conference media worker unavailable")
	}
	endpoint, err := url.Parse(route.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("invalid media worker endpoint")
	}
	seconds := record.SegmentDurationSec
	if seconds < 1 || seconds > 30 {
		seconds = 5
	}
	request := media.EgressRequest{RequestID: uuid.NewString(), ConferenceID: record.ConferenceID, RecordingID: record.UUID, Route: route, SegmentDurationSec: seconds}
	data, _ := json.Marshal(request)
	httpRequest, err := http.NewRequestWithContext(captureCtx, http.MethodPost, strings.TrimRight(route.Endpoint, "/")+"/internal/media/egress", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+s.o.InternalSecret)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Request-ID", request.RequestID)
	response, err := s.client.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("connect recording egress failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("recording egress rejected (%d)", response.StatusCode)
	}
	monitored := make(chan struct{})
	input := &activityReader{reader: response.Body}
	input.lastRead.Store(time.Now().UnixNano())
	go func() {
		defer close(monitored)
		ticker := time.NewTicker(s.o.Config.PollInterval)
		defer ticker.Stop()
		deadline := time.NewTimer(s.o.Config.MaxDuration)
		defer deadline.Stop()
		for {
			select {
			case <-captureCtx.Done():
				return
			case <-deadline.C:
				queryCtx, done := context.WithTimeout(captureCtx, 3*time.Second)
				_ = s.o.Repository.MarkStopping(queryCtx, record.UUID, "duration_limit")
				done()
				cancel()
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, input.lastRead.Load())) > 15*time.Second {
					cancel()
					return
				}
				queryCtx, done := context.WithTimeout(captureCtx, 3*time.Second)
				current, err := s.o.Repository.FindByUUID(queryCtx, record.UUID)
				done()
				if err != nil {
					cancel()
					return
				}
				if current.Status == records.StatusStopping || records.IsTerminalStatus(current.Status) {
					cancel()
					return
				}
			}
		}
	}()
	err = s.composer.Capture(captureCtx, ctx, input, dir, composite.CaptureOptions{Mode: record.Mode, SegmentDuration: time.Duration(seconds) * time.Second, MaxBytes: s.o.Config.MaxBytes, OnStarted: func() error {
		if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusRecording, nil); err != nil {
			if errors.Is(err, records.ErrRecordStateChanged) {
				current, readErr := s.o.Repository.FindByUUID(ctx, record.UUID)
				if readErr == nil && current.Status == records.StatusStopping {
					cancel()
					return nil
				}
			}
			return err
		}
		current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
		if err != nil {
			return err
		}
		s.publish(ctx, current, "recording.started")
		return nil
	}})
	cancel()
	<-monitored
	if composite.OnlyEgressEnded(err) {
		current, readErr := s.o.Repository.FindByUUID(ctx, record.UUID)
		if readErr == nil && current.Status == records.StatusStopping {
			return nil
		}
	}
	return err
}

// activityReader хранит данные о действиях, используемые заданиями по записи и их артефактами.
type activityReader struct {
	reader   io.Reader
	lastRead atomic.Int64
}

// Read — прочитанные копии имеют размер, не превышающий размер буфера назначения, и извлекаются из
// потока, при этом сохраняются ограничения на количество прочитанных данных и возможность
// отмены операции.
//
// @Parameters:
//   - p: Целевой байтовый буфер заполняется данными из потока.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *activityReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.lastRead.Store(time.Now().UnixNano())
	}
	return n, err
}

// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *activityReader) Close() error {
	if closer, ok := r.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// fail — при сбое в работе задания по записи запускается процесс очистки принадлежащих ему ресурсов.
//
// @Parameters:
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//   - token: Подписанные учетные данные или токен владения операцией для проверки.
//   - cause: Основная ошибка, из-за которой операция завершилась неудачно.
func (s *CompositeService) fail(record records.Record, token string, cause error) {
	operations.Event("recording_failed")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Сохраняйте полную диагностику FFmpeg локально; клиенты получают только ограниченное
	// безопасное сообщение.
	s.o.Logger.Printf("composite record=%s failed: %v", record.UUID, cause)
	//lint:ignore ST1005 Сообщение клиенту сохраняет существующий текст ошибки.
	safe := errors.New("Conference recording failed; completed segments were retained")
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusFailed, safe); err != nil {
		return
	}
	current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
	if err == nil {
		s.publish(ctx, current, "recording.failed")
	}
	if s.o.ConferenceLock != nil {
		_ = s.o.ConferenceLock.Release(ctx, record.ConferenceID, record.UUID)
	}
	_ = s.o.Repository.AddEvent(ctx, record.UUID, "recording.failed", "recorder", "error", safe.Error(), s.o.WorkerID)
}

// publish публикует сохраненное изменение задания записи через настроенный транспорт событий
// или команд.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//   - event: Доверенное событие для публикации или обработки.
func (s *CompositeService) publish(ctx context.Context, record records.Record, event string) {
	if s.o.Publish == nil {
		return
	}
	publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.o.Publish(publishCtx, record, event); err != nil {
		s.o.Logger.Printf("recording event publish failed record=%s event=%s", record.UUID, event)
	}
}

// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/janickiy/meet-space/internal/domain/records"
	postgresinfra "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	rabbitmqinfra "github.com/janickiy/meet-space/internal/infrastructure/rabbitmq"
	redisinfra "github.com/janickiy/meet-space/internal/infrastructure/redis"
	s3storage "github.com/janickiy/meet-space/internal/infrastructure/storage/s3"
	"github.com/janickiy/meet-space/internal/operations"
	"github.com/janickiy/meet-space/internal/usecase/recorder"
)

// RunRecordWorker запускает воркер с настроенными зависимостями и сообщает о сбоях при запуске или завершении работы.
//
// @returns:
//   - result1: nil в случае успеха; в противном случае — ошибка валидации, доступа, транспорта или сохранения данных.
func RunRecordWorker() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	mediaConfig, err := config.LoadMedia()
	if err != nil {
		return err
	}
	realtimeConfig, err := config.LoadRealtime()
	if err != nil {
		return err
	}
	compositeConfig, err := config.LoadComposite()
	if err != nil {
		return err
	}
	logger := log.New(operations.LogWriter{}, "", 0)
	if err := operations.ValidateStorage(cfg.StoragePath, cfg.Operations.DiskMinBytes); err != nil {
		return err
	}
	if _, err := exec.LookPath(cfg.FFmpegPath); err != nil {
		return fmt.Errorf("FFMPEG_PATH executable unavailable")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := postgresinfra.Connect(cfg.PostgresDSN)
	if err != nil {
		return err
	}
	if err := postgresinfra.RunStartupMigrations(db, "database/migrations"); err != nil {
		return err
	}
	s3Client, err := s3storage.NewClient(ctx, cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket, cfg.MinIOUseSSL)
	if err != nil {
		return err
	}

	repository := postgresinfra.NewRecordRepository(db)
	redisClient, err := redisinfra.NewClient(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return err
	}
	defer redisClient.Close()
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(cfg.Operations.DBMaxOpen)
	sqlDB.SetMaxIdleConns(cfg.Operations.DBMaxIdle)
	sqlDB.SetConnMaxLifetime(cfg.Operations.DBLifetime)
	conferenceLock := redisinfra.NewConferenceLock(redisClient, cfg.RecordLockTTL)
	store := redisinfra.NewRealtimeStore(redisClient, realtimeConfig.Namespace)
	composites := recorder.NewCompositeService(recorder.CompositeOptions{
		Repository: repository, Registry: redisinfra.NewMediaRegistry(redisClient, mediaConfig.Namespace), S3: s3Client,
		StoragePath: cfg.StoragePath, FFmpegPath: cfg.FFmpegPath, WorkerID: cfg.WorkerID, InternalSecret: mediaConfig.InternalSecret,
		Config: compositeConfig, ConferenceLock: conferenceLock, Logger: logger,
		Publish: func(ctx context.Context, record records.Record, kind string) error {
			event := recordingWorkerEvent(kind, record)
			return store.Publish(ctx, realtime.Bus{Kind: "event", ConferenceID: record.ConferenceID, Event: &event})
		},
	})
	service := recorder.NewWorkerService(repository, composites)
	composites.Start(ctx)
	defer func() {
		stop()
		shutdown, cancel := context.WithTimeout(context.Background(), cfg.Operations.ShutdownTimeout)
		defer cancel()
		if err := composites.WaitContext(shutdown); err != nil {
			slog.Error("recorder shutdown timed out", "event_type", "service.shutdown_timeout")
		}
	}()
	consumer, err := rabbitmqinfra.NewConsumer(ctx, rabbitmqinfra.Options{
		URL:         cfg.RabbitMQURL,
		Exchange:    cfg.RabbitMQExchange,
		Queue:       cfg.RabbitMQQueue,
		RoutingKey:  cfg.RabbitMQRoutingKey,
		ConsumerTag: cfg.WorkerID,
		Logger:      logger,
	})
	if err != nil {
		return err
	}
	defer consumer.Close()
	ops := operations.New("record-worker", cfg.WorkerID, cfg.Operations, map[string]operations.Check{"postgres": sqlDB.PingContext, "redis": func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }, "minio": s3Client.Check, "rabbitmq": consumer.Check, "disk": operations.DiskCheck(cfg.StoragePath, cfg.Operations.DiskMinBytes)})
	ops.ConfigureDrain(func() { consumer.BeginDrain(); composites.BeginDrain() }, func() int { return composites.Active() + consumer.Active() })
	ops.Run(ctx)
	go runProfiling(ctx, ops)
	go sampleDatabase(ctx, sqlDB)
	go sampleRecorder(ctx, composites)

	errCh := make(chan error, 2)
	go func() {
		errCh <- consumer.Consume(ctx, service.HandleCommand)
	}()
	httpDone := make(chan struct{})
	go func() { defer close(httpDone); errCh <- serveWorkerHTTP(ctx, cfg.WorkerPort, ops) }()
	defer func() {
		ops.Drain()
		stop()
		select {
		case <-httpDone:
		case <-time.After(cfg.Operations.ShutdownTimeout + time.Second):
			slog.Error("HTTP drain timed out", "event_type", "service.shutdown_timeout")
		}
	}()
	logger.Printf("record-worker is ready; http=:%d storage=%s minio=%s/%s rabbitmq_queue=%s", cfg.WorkerPort, cfg.StoragePath, cfg.MinIOEndpoint, cfg.MinIOBucket, cfg.RabbitMQQueue)

	select {
	case <-ctx.Done():
		ops.Drain()
		return nil
	case err := <-errCh:
		if err != nil && ctx.Err() == nil {
			return err
		}

		return nil
	}
}

// serveWorkerHTTP serves operational endpoints without recording-control routes.
// ctx controls shutdown, port selects the internal listener, and ops provides
// readiness, metrics, version, and graceful-drain endpoints.
func serveWorkerHTTP(ctx context.Context, port int, ops *operations.Runtime) error {
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", port), Handler: newRecordWorkerHandler(ops), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 45 * time.Minute, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192,
	}
	shutdownDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		defer close(shutdownDone)
		ops.Drain()
		shutdown, done := context.WithTimeout(context.Background(), ops.Config.ShutdownTimeout)
		defer done()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("record-worker HTTP server: %w", err)
	}
	if ctx.Err() != nil {
		<-shutdownDone
	}
	return nil
}

// sampleRecorder обновляет ограниченные метрики активности рабочих процессов записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - service: Служба приложения, вызываемая этим адаптером.
func sampleRecorder(ctx context.Context, service *recorder.CompositeService) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		operations.State("recordings_active", float64(service.Active()))
		select {
		case <-ctx.Done():
			slog.Info("recorder draining", "event_type", "service.draining")
			return
		case <-ticker.C:
		}
	}
}

// newRecordWorkerHandler creates the operational HTTP surface for ops.
// Recording commands are consumed from RabbitMQ; removed /records routes return 404.
func newRecordWorkerHandler(ops *operations.Runtime) http.Handler {
	mux := http.NewServeMux()
	ops.Register(mux)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	return mux
}

// recordingWorkerEvent формирует событие записи, отправляемое после зафиксированного изменения
// жизненного цикла.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//
// @returns:
//   - result1: Результирующий конверт.
func recordingWorkerEvent(kind string, record records.Record) realtime.Envelope {
	return realtime.Event(kind, record.ConferenceID, map[string]any{
		"recordingId": record.UUID, "conferenceId": record.ConferenceID,
		"status": records.PublicStatus(record.Status), "mode": record.Mode,
		"error": record.ErrorMessage, "requestedBy": record.RequestedBy,
	})
}

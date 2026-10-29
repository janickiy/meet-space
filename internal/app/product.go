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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/janickiy/meet-space/internal/config"
	pg "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	"github.com/janickiy/meet-space/internal/infrastructure/providers"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	"github.com/janickiy/meet-space/internal/operations"
	integrations "github.com/janickiy/meet-space/internal/usecase/integrations"
	jobrunner "github.com/janickiy/meet-space/internal/usecase/jobs"
	"gorm.io/gorm"
)

// productServices группирует операции с продуктами без изменения жизненного цикла записи RTP
// или FFmpeg.
type productServices struct {
	integrations *integrations.Service
}

// newProductServices создает зависимости уведомлений и внешних интеграций.
// API ставит задания в очередь, отдельный процесс доставляет их провайдерам.
//
// @Parameters:
//   - cfg: управляемая оператором конфигурация и ограничения обработки.
//   - db: дескриптор базы данных, используемый для операции или транзакции.
//
// @returns:
//   - result1: результирующие сервисы для продуктов.
//   - result2: nil в случае успешного выполнения; в противном случае — ошибка валидации, доступа, транспортировки или сохранения
//     данных.
func newProductServices(cfg config.Config, db *gorm.DB) (productServices, error) {
	c := cfg.StageSeven
	local := cfg.IsLocal() || cfg.AppEnv == "test"
	adapter := func(p config.ProviderConfig) providers.AdapterConfig {
		return providers.AdapterConfig{Mode: p.Mode, Endpoint: p.Endpoint, Secret: p.Token, Timeout: c.ProviderTimeout, AllowHTTP: local}
	}
	channels, err := providers.NewIntegrations(providers.IntegrationConfig{Email: adapter(c.Email), Push: adapter(c.Push), Calendar: adapter(c.Calendar), MockConnectAllowed: local,
		SMTP:  providers.SMTPConfig{Host: c.SMTP.Host, Port: c.SMTP.Port, Username: c.SMTP.Username, Password: c.SMTP.Password, From: c.SMTP.From, TLSMode: c.SMTP.TLSMode, Timeout: c.SMTP.Timeout},
		OAuth: providers.OAuthConfig{AuthorizationURL: c.OAuthAuthURL, TokenURL: c.OAuthTokenURL, RevokeURL: c.OAuthRevokeURL, ClientID: c.OAuthClientID, ClientSecret: c.OAuthClientSecret, RedirectURL: c.OAuthRedirectURL, Scopes: c.OAuthScopes, Timeout: c.ProviderTimeout, AllowHTTP: local}})
	if err != nil {
		return productServices{}, err
	}
	channels.Email = measuredEmail{channels.Email}
	channels.Push = measuredPush{channels.Push}
	channels.Calendar = measuredCalendar{channels.Calendar}
	tokenCipher, err := security.NewProviderTokens(c.EncryptionKey)
	if err != nil {
		return productServices{}, err
	}
	var cipher integrations.TokenCipher
	if tokenCipher != nil {
		cipher = tokenCipher
	}
	deliveryTimeout := c.ProviderTimeout
	if c.Email.Mode == "smtp" && c.SMTP.Timeout > deliveryTimeout {
		deliveryTimeout = c.SMTP.Timeout
	}
	integrationService, err := integrations.NewService(pg.NewIntegrationRepository(db), channels, cipher, integrations.Options{PublicURL: c.PublicURL, ReminderOffsets: c.ReminderOffsets, ProviderTimeout: deliveryTimeout, MockConnectAllowed: local})
	if err != nil {
		return productServices{}, err
	}
	return productServices{integrations: integrationService}, nil
}

// RunProductWorker запускает продукт-воркер с настроенными зависимостями и передает информацию о сбоях при запуске
// или завершении работы.
//
// @returns:
//   - result1: nil при успешном выполнении; в противном случае — ошибка валидации, доступа, транспорта или сохранения
//     данных.
func RunProductWorker() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := pg.Connect(cfg.PostgresDSN)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(cfg.Operations.DBMaxOpen)
	sqlDB.SetMaxIdleConns(cfg.Operations.DBMaxIdle)
	sqlDB.SetConnMaxLifetime(cfg.Operations.DBLifetime)
	if err := pg.RunStartupMigrations(db, "database/migrations"); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	services, err := newProductServices(cfg, db)
	if err != nil {
		return err
	}
	checks := map[string]operations.Check{"postgres": sqlDB.PingContext}
	ops := operations.New("product-worker", hostname(), cfg.Operations, checks)
	ops.Run(ctx)
	go runProfiling(ctx, ops)
	go sampleDatabase(ctx, sqlDB)
	repo := pg.NewJobRepository(db)
	i := jobrunner.Handler{Handle: services.integrations.Handle, Fail: services.integrations.FailJob}
	s := cfg.StageSeven
	deliveryTimeout := s.ProviderTimeout
	if s.Email.Mode == "smtp" && s.SMTP.Timeout > deliveryTimeout {
		deliveryTimeout = s.SMTP.Timeout
	}
	runner, err := jobrunner.New(repo, []jobrunner.Pool{
		{Kind: "integrations.conference", Concurrency: 1, MaxAttempts: s.MaxAttempts, Timeout: s.ProviderTimeout, Handler: i}, {Kind: "integrations.event", Concurrency: 1, MaxAttempts: s.MaxAttempts, Timeout: s.ProviderTimeout, Handler: i},
		{Kind: "integrations.delivery", Concurrency: s.DeliveryWorkers, MaxAttempts: s.MaxAttempts, Timeout: deliveryTimeout, Handler: i},
		{Kind: "integrations.invitation", Concurrency: s.DeliveryWorkers, MaxAttempts: s.MaxAttempts, Timeout: deliveryTimeout, Handler: i}, {Kind: "integrations.calendar", Concurrency: s.CalendarWorkers, MaxAttempts: s.MaxAttempts, Timeout: s.ProviderTimeout * 3, Handler: i},
		{Kind: "integrations.invitation_reminder", Concurrency: s.DeliveryWorkers, MaxAttempts: s.MaxAttempts, Timeout: deliveryTimeout, Handler: i},
	}, s.PollInterval, operations.Product)
	if err != nil {
		return err
	}
	workerDone := make(chan struct{})
	ops.ConfigureDrain(runner.BeginDrain, runner.Active)
	go func() { defer close(workerDone); runner.Run(ctx) }()
	tickDone := make(chan struct{})
	go func() {
		defer close(tickDone)
		runProductTicks(ctx, services.integrations, repo, s.PollInterval)
	}()
	mux := http.NewServeMux()
	ops.Register(mux)
	server := &http.Server{Addr: fmt.Sprintf(":%d", s.Port), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	failed := make(chan error, 1)
	go func() { failed <- server.ListenAndServe() }()
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-failed:
	}
	ops.Drain()
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), cfg.Operations.ShutdownTimeout)
	defer cancel()
	serverErr := server.Shutdown(shutdown)
	for _, done := range []<-chan struct{}{workerDone, tickDone} {
		select {
		case <-done:
		case <-shutdown.Done():
			return errors.Join(serverErr, shutdown.Err())
		}
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, serverErr)
}

// runProductTicks выполняет периодическое обслуживание продукта независимо от записи на носитель
// жизненного цикла.
//
// @Parameters:
//   - ctx: контекст, управляющий отменой, сроками и временем жизни операции.
//   - service: сервис приложения, вызываемый этим адаптером.
//   - repo: зависимость от персистентности, используемая операцией приложения.
//   - interval: интервал между итерациями ограниченного обслуживания.
func runProductTicks(ctx context.Context, service *integrations.Service, repo *pg.JobRepository, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		tickCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := service.Tick(tickCtx); err != nil && ctx.Err() == nil {
			slog.Warn("product scheduler unavailable", "event_type", "product.scheduler.failed")
		}
		counts, err := repo.Counts(tickCtx)
		cancel()
		if err == nil {
			for _, kind := range []string{"integrations.conference", "integrations.event", "integrations.delivery", "integrations.invitation", "integrations.invitation_reminder", "integrations.calendar"} {
				for _, state := range []string{"queued", "processing", "failed"} {
					operations.ProductQueue(kind, state, 0)
				}
			}
			for _, count := range counts {
				operations.ProductQueue(count.Kind, count.State, count.Count)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

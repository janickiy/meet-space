package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/janickiy/meet-space/internal/buildinfo"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/realtime"
	"github.com/janickiy/meet-space/internal/domain/records"
	healthinfra "github.com/janickiy/meet-space/internal/infrastructure/health"
	postgresinfra "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	"github.com/janickiy/meet-space/internal/operations"
	"gorm.io/gorm"
)

type adminStorage interface {
	RemoveRecording(context.Context, records.Record) error
}
type adminEvents interface {
	Broadcast(context.Context, realtime.Envelope) error
}
type adminCached struct {
	value   any
	expires time.Time
}
type adminResources struct {
	repository *postgresinfra.AdminRepository
	storage    adminStorage
	events     adminEvents
	settings   admin.Settings
	checks     map[string]operations.Check
	clients    []*healthinfra.HTTPReady
	mu         sync.Mutex
	cache      map[string]adminCached
	cacheGates map[string]chan struct{}
	cacheEpoch uint64
}

func newAdminResources(db *gorm.DB, cfg config.Config, rtc config.RealtimeConfig, media config.MediaConfig, ops *operations.Runtime, storage adminStorage, events adminEvents) *adminResources {
	r := &adminResources{repository: postgresinfra.NewAdminRepository(db), storage: storage, events: events, cache: map[string]adminCached{}, checks: map[string]operations.Check{}}
	r.checks["API"] = func(context.Context) error {
		if ops.Ready() {
			return nil
		}
		return errors.New("not ready")
	}
	for name, key := range map[string]string{"PostgreSQL": "postgres", "Redis": "redis", "RabbitMQ": "rabbitmq"} {
		if check := ops.Checks[key]; check != nil {
			r.checks[name] = check
		}
	}
	for name, base := range map[string]string{"Media Worker": media.WorkerInternalURL, "Record Worker": fmt.Sprintf("http://record-worker:%d", cfg.WorkerPort), "Product Worker": fmt.Sprintf("http://product-worker:%d", cfg.StageSeven.Port)} {
		client := healthinfra.NewHTTPReady(base)
		r.clients = append(r.clients, client)
		r.checks[name] = func(ctx context.Context) error {
			if client.Ready(ctx) {
				return nil
			}
			return errors.New("not ready")
		}
	}
	r.settings = adminEffectiveSettings(cfg, rtc, media)
	return r
}

func (r *adminResources) Close() {
	for _, c := range r.clients {
		c.Close()
	}
}

func (r *adminResources) Query(ctx context.Context, resource string, q admin.Query) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	switch resource {
	case "conferences":
		return r.repository.Conferences(ctx, q)
	case "recordings":
		return r.repository.Recordings(ctx, q)
	case "settings":
		return r.settings, nil
	case "dashboard", "chats", "files", "system":
		return r.cachedQuery(ctx, resource, func() (any, error) {
			switch resource {
			case "dashboard":
				v, err := r.repository.Dashboard(ctx)
				if err != nil {
					return nil, err
				}
				// Reuse the same bounded probe cache as the System page.
				state, err := r.Query(ctx, "system", admin.Query{})
				if err != nil {
					return nil, err
				}
				v.Services = state.(admin.System).Services
				return v, nil
			case "chats":
				return r.repository.ChatSummary(ctx)
			case "files":
				return r.repository.FileSummary(ctx)
			default:
				return r.system(ctx), nil
			}
		})
	default:
		return nil, apperrors.ErrNotFound
	}
}

// Only the four fixed aggregate keys reach this cache. Each key has a
// cancellation-aware gate; the map mutex is never held during I/O.
func (r *adminResources) cachedQuery(ctx context.Context, key string, fetch func() (any, error)) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	if cached, ok := r.cache[key]; ok && time.Now().Before(cached.expires) {
		r.mu.Unlock()
		return cached.value, nil
	}
	if r.cacheGates == nil {
		r.cacheGates = make(map[string]chan struct{}, 4)
	}
	gate := r.cacheGates[key]
	if gate == nil {
		gate = make(chan struct{}, 1)
		r.cacheGates[key] = gate
	}
	r.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case gate <- struct{}{}:
	}
	defer func() { <-gate }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	if cached, ok := r.cache[key]; ok && time.Now().Before(cached.expires) {
		r.mu.Unlock()
		return cached.value, nil
	}
	epoch := r.cacheEpoch
	r.mu.Unlock()
	value, err := fetch()
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		r.mu.Lock()
		if r.cacheEpoch == epoch {
			if r.cache == nil {
				r.cache = make(map[string]adminCached, 4)
			}
			r.cache[key] = adminCached{value: value, expires: time.Now().Add(10 * time.Second)}
		}
		r.mu.Unlock()
	}
	return value, err
}

func (r *adminResources) invalidateCache() {
	r.mu.Lock()
	r.cacheEpoch++
	r.cache = map[string]adminCached{}
	r.mu.Unlock()
}

func (r *adminResources) system(ctx context.Context) admin.System {
	names := []string{"API", "Media Worker", "Record Worker", "Product Worker", "PostgreSQL", "Redis", "RabbitMQ"}
	out := admin.System{Services: make([]admin.ServiceHealth, len(names))}
	var wg sync.WaitGroup
	for i, name := range names {
		out.Services[i] = admin.ServiceHealth{Name: name, Status: "unknown"}
		check := r.checks[name]
		if check == nil {
			continue
		}
		wg.Add(1)
		go func(i int, check operations.Check) {
			defer wg.Done()
			op, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			start := time.Now()
			err := check(op)
			elapsed := time.Since(start).Milliseconds()
			now := time.Now().UTC()
			status := "healthy"
			if err != nil {
				status = "unavailable"
			}
			out.Services[i].Status = status
			out.Services[i].LatencyMS = &elapsed
			out.Services[i].CheckedAt = &now
		}(i, check)
	}
	wg.Wait()
	if version := buildinfo.Current().Version; version != "" {
		out.Services[0].Version = &version
	}
	// Host/container CPU, RAM and remote object-store capacity are not measured by
	// this API. A process heap or local API filesystem would be misleading here.
	return out
}

func (r *adminResources) DeleteRecording(ctx context.Context, id string, audit admin.AuditInput) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	record, err := r.repository.DeleteAdminRecording(ctx, id, audit, r.storage.RemoveRecording)
	if err != nil {
		return err
	}
	if record.UUID == "" {
		return nil // An idempotent retry must not publish a second deletion event.
	}
	r.invalidateCache()
	if r.events != nil && record.PlatformConferenceID != nil {
		_ = r.events.Broadcast(ctx, realtime.Event("recording.deleted", *record.PlatformConferenceID, map[string]any{"recordingId": record.UUID, "conferenceId": *record.PlatformConferenceID, "status": records.PublicStatus(record.Status), "mode": record.Mode, "requestedBy": record.RequestedBy}))
	}
	return nil
}

func (r *adminResources) UpdateSettings(context.Context, json.RawMessage, admin.AuditInput) (any, error) {
	return nil, apperrors.New(apperrors.ErrConflict, "Эти параметры задаются конфигурацией сервера. Изменение без перезапуска пока не поддерживается.")
}

func adminEffectiveSettings(cfg config.Config, rtc config.RealtimeConfig, media config.MediaConfig) admin.Settings {
	server := "Задаётся на сервере; изменение требует перезапуска соответствующего сервиса."
	items := []admin.Setting{
		{Key: "registration", Label: "Регистрация пользователей", Description: "Регистрация включена в текущей версии платформы.", Value: true, Type: "boolean", Group: "general", Configured: true},
		{Key: "publicUrl", Label: "Адрес платформы", Description: server, Value: cfg.StageSeven.PublicURL, Type: "string", Group: "general", Configured: true},
		{Key: "attachmentMaxBytes", Label: "Максимальный размер вложения", Description: "Ограничение действующего сервиса чатов, в байтах.", Value: chat.MaxAttachmentBytes, Type: "number", Group: "limits", Configured: true},
		{Key: "mediaMaxPeers", Label: "Лимит медиаподключений", Description: server, Value: media.MaxPeers, Type: "number", Group: "limits", Configured: true},
		{Key: "mediaMaxRooms", Label: "Лимит медиакомнат", Description: server, Value: media.MaxRooms, Type: "number", Group: "limits", Configured: true},
		{Key: "recordingRetentionDays", Label: "Срок хранения записей, дней", Description: "Существующая политика автоматически удаляет завершённые записи через 7 дней.", Value: 7, Type: "number", Group: "storage", Configured: true},
		{Key: "smtpHost", Label: "SMTP-сервер", Description: server, Value: cfg.StageSeven.SMTP.Host, Type: "string", Group: "mail", Configured: cfg.StageSeven.SMTP.Host != ""},
		{Key: "smtpPort", Label: "SMTP-порт", Description: server, Value: cfg.StageSeven.SMTP.Port, Type: "number", Group: "mail", Configured: cfg.StageSeven.SMTP.Port > 0},
		{Key: "smtpPassword", Label: "Пароль SMTP", Description: "Значение секрета никогда не передаётся в браузер.", Value: nil, Type: "string", Group: "mail", Secret: true, Configured: cfg.StageSeven.SMTP.Password != ""},
		{Key: "iceServerCount", Label: "Количество STUN/TURN серверов", Description: server, Value: len(rtc.ICE.ICEServers), Type: "number", Group: "network", Configured: true},
	}
	return admin.Settings{Items: items}
}

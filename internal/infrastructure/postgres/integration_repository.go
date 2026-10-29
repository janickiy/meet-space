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
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	d "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	u "github.com/janickiy/meet-space/internal/usecase/integrations"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IntegrationRepository сохраняет настройки, зашифрованные учетные данные, данные о надежной
// доставке и сопоставлениях с календарем независимо от обработки медиафайлов.
type IntegrationRepository struct{ db *gorm.DB }

// NewIntegrationRepository создает репозиторий интеграции с зависимостями и ограничениями,
// необходимыми для интеграции.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданное или загруженное репозиторий интеграции.
func NewIntegrationRepository(db *gorm.DB) *IntegrationRepository {
	return &IntegrationRepository{db: db}
}

// Preferences считывает сохраненные настройки доставки или переписки действующего аккаунта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Результирующие настройки.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) Preferences(ctx context.Context, userID string) (d.Preferences, error) {
	p := d.DefaultPreferences(userID)
	err := r.db.WithContext(ctx).Where("user_id=?", userID).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return d.DefaultPreferences(userID), nil
	}
	return p, err
}

// SavePreferences проверяет и сохраняет настройки доставки аккаунта, не инициируя доставку.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - p: Состояние настроек, обработанное в ходе этой операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) SavePreferences(ctx context.Context, p d.Preferences) error {
	p.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"invitation", "reminder", "recording", "email", "push", "updated_at"})}).Create(&p).Error
}

// Devices возвращает зарегистрированные устройства доставки аккаунта, не раскрывая их
// конфиденциальные учетные данные.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - active: Выбор только активных устройств, не отозванных из списка.
//
// @returns:
//   - result1: Собранные значения устройств.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) Devices(ctx context.Context, userID string, active bool) ([]d.Device, error) {
	items := []d.Device{}
	q := r.db.WithContext(ctx).Where("user_id=?", userID)
	if active {
		q = q.Where("revoked_at IS NULL AND disabled_at IS NULL")
	}
	err := q.Order("created_at,id").Limit(100).Find(&items).Error
	return items, err
}

// SaveDevice сохраняет устройство доставки, принадлежащее аккаунту, с защищенным токеном
// провайдера.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - device: Устройство доставки, принадлежащее аккаунту, и его защищенные метаданные
//     провайдера.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) SaveDevice(ctx context.Context, device d.Device) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", device.UserID+":devices").Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&d.Device{}).Where("user_id=? AND revoked_at IS NULL AND disabled_at IS NULL AND id<>?", device.UserID, device.ID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 100 {
			return apperrors.ErrConflict
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.Assignments(map[string]any{"token_ciphertext": device.TokenCiphertext, "platform": device.Platform, "updated_at": time.Now().UTC(), "revoked_at": nil, "disabled_at": nil})}).Create(&device).Error
	})
}

// RevokeDevice отзывает только выбранное устройство доставки действующего аккаунта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) RevokeDevice(ctx context.Context, userID, id string) error {
	result := r.db.WithContext(ctx).Model(&d.Device{}).Where("user_id=? AND id=?", userID, id).Updates(map[string]any{"revoked_at": time.Now().UTC(), "token_ciphertext": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

// Connections возвращает подключения провайдера действующего аккаунта с исключенными из общего
// доступа конфиденциальными учетными данными.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: Собранные значения подключений календаря.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) Connections(ctx context.Context, userID string) ([]d.CalendarConnection, error) {
	items := []d.CalendarConnection{}
	err := r.db.WithContext(ctx).Where("user_id=?", userID).Order("created_at,id").Limit(10).Find(&items).Error
	return items, err
}

// SaveConnection сохраняет подтвержденное подключение к провайдеру, принадлежащее учетной
// записи, и зашифрованные учетные данные.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - c: Значение подключения к календарю, обработанное в ходе этой операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) SaveConnection(ctx context.Context, c d.CalendarConnection) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"status", "access_ciphertext", "refresh_ciphertext", "scopes", "expires_at", "updated_at"})}).Create(&c).Error
}

// RefreshConnection обновляет учетные данные провайдера только для ожидаемого состояния
// подключения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - c: Значение подключения к календарю, обработанное в ходе этой операции.
//   - previous: Предыдущие учетные данные или идентификационные данные, использованные для
//     отклонения устаревшего обновления.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) RefreshConnection(ctx context.Context, c d.CalendarConnection, previous string) error {
	result := r.db.WithContext(ctx).Model(&d.CalendarConnection{}).Where("id=? AND user_id=? AND status='connected' AND refresh_ciphertext=?", c.ID, c.UserID, previous).Updates(map[string]any{"access_ciphertext": c.AccessCiphertext, "refresh_ciphertext": c.RefreshCiphertext, "expires_at": c.ExpiresAt, "scopes": c.Scopes})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return jobs.Error{Code: "calendar_grant_changed", Retryable: true}
	}
	return nil
}

// RevokeConnection отзывает подключение к провайдеру, принадлежащее учетной записи, и делает
// недействительной его дальнейшую синхронизацию.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) RevokeConnection(ctx context.Context, userID, id string) error {
	result := r.db.WithContext(ctx).Model(&d.CalendarConnection{}).Where("user_id=? AND id=?", userID, id).Updates(map[string]any{"status": "revoked", "access_ciphertext": "", "refresh_ciphertext": "", "expires_at": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

// SaveOAuthState сохраняет кратковременное одноразовое состояние OAuth, привязанное к учетной
// записи и провайдеру.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - state: Текущее состояние или обновлённое валидированное состояние.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) SaveOAuthState(ctx context.Context, state d.OAuthState) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", state.UserID+":oauth").Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM calendar_oauth_states WHERE id IN (SELECT id FROM calendar_oauth_states WHERE expires_at<now() OR used_at IS NOT NULL ORDER BY expires_at,id LIMIT 1000)`).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&d.OAuthState{}).Where("user_id=?", state.UserID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 10 {
			return apperrors.ErrConflict
		}
		return tx.Create(&state).Error
	})
}

// TakeOAuthState атомарно использует соответствующее одноразовое состояние OAuth, чтобы
// предотвратить повторный вызов функции обратного вызова.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - provider: Идентификатор провайдера, выбранный оператором, или фиксированная техническая
//     метка провайдера.
//   - hash: Ранее вычисленный дайджест, используемый для проверки подлинности или целостности.
//
// @returns:
//   - result1: Результирующее состояние авторизации.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) TakeOAuthState(ctx context.Context, userID, provider, hash string) (d.OAuthState, error) {
	var value d.OAuthState
	result := r.db.WithContext(ctx).Raw(`UPDATE calendar_oauth_states SET used_at=now() WHERE user_id=? AND provider=? AND state_hash=? AND used_at IS NULL AND expires_at>now() RETURNING *`, userID, provider, hash).Scan(&value)
	if result.Error != nil {
		return value, result.Error
	}
	if result.RowsAffected == 0 {
		return value, apperrors.ErrInvalidInput
	}
	return value, nil
}

// CalendarMappings возвращает сохраненные сопоставления между конференциями и календарями для
// действующей учетной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Собранные значения сопоставлений между календарями.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) CalendarMappings(ctx context.Context, userID, conferenceID string) ([]d.CalendarMapping, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("conference_participants").Where("user_id=? AND conference_id=? AND role IN ('owner','co_host') AND admission_state='admitted' AND status IN ('joined','left')", userID, conferenceID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, apperrors.ErrForbidden
	}
	items := []d.CalendarMapping{}
	err := r.db.WithContext(ctx).Table("calendar_event_mappings m").Where("m.conference_id=?", conferenceID).Where(`EXISTS(SELECT 1 FROM conference_participants p WHERE p.conference_id=m.conference_id AND p.user_id=? AND p.role IN ('owner','co_host') AND p.admission_state='admitted' AND p.status IN ('joined','left'))`, userID).Limit(10).Find(&items).Error
	return items, err
}

// Conference загружает сохраненные данные о конференциях, необходимые для интеграции и
// синхронизации.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Результирующий снимок конференции.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) Conference(ctx context.Context, id string) (u.ConferenceSnapshot, error) {
	var value u.ConferenceSnapshot
	err := r.db.WithContext(ctx).Table("conferences").Where("id=?", id).Take(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = apperrors.ErrNotFound
	}
	return value, err
}

// MappingsForSync возвращает сопоставления между календарями, подходящие для текущего
// формирования расписания.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Собранные значения сопоставлений между календарями.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) MappingsForSync(ctx context.Context, conferenceID string) ([]d.CalendarMapping, error) {
	items := []d.CalendarMapping{}
	err := r.db.WithContext(ctx).Where("conference_id=?", conferenceID).Limit(10).Find(&items).Error
	return items, err
}

// integrationLease проверяет, кому принадлежит арендованное задание интеграции, прежде чем
// зафиксировать его результат.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func integrationLease(tx *gorm.DB, job jobs.Job) error {
	var id string
	result := tx.Raw(`SELECT id FROM background_jobs WHERE id=? AND state='processing' AND lease_token=? AND lease_until>clock_timestamp() FOR UPDATE`, job.ID, job.LeaseToken).Scan(&id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return jobs.ErrLeaseLost
	}
	return nil
}

// SaveMapping сохраняет сопоставление событий провайдера только до тех пор, пока активна аренда
// задания и версия расписания.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - m: Подтвержденный манифест резервного копирования или состояние медиафайлов в
//     соответствии с их типом.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) SaveMapping(ctx context.Context, job jobs.Job, m d.CalendarMapping) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := integrationLease(tx, job); err != nil {
			return err
		}
		var allowed int64
		if err := tx.Table("conferences c").Joins("JOIN calendar_connections conn ON conn.user_id=c.owner_id AND conn.id=? AND conn.status='connected'", m.ConnectionID).Where("c.id=? AND c.integration_version=?", job.ConferenceID, job.Version).Count(&allowed).Error; err != nil {
			return err
		}
		if allowed == 0 {
			return jobs.ErrSkip
		}
		return tx.Exec(`INSERT INTO calendar_event_mappings(conference_id,connection_id,provider,external_calendar_id,external_event_id,sync_status,source_version,last_synced_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(conference_id,connection_id) DO UPDATE SET external_event_id=EXCLUDED.external_event_id,sync_status=EXCLUDED.sync_status,source_version=EXCLUDED.source_version,last_synced_at=EXCLUDED.last_synced_at WHERE calendar_event_mappings.source_version<=EXCLUDED.source_version`, m.ConferenceID, m.ConnectionID, m.Provider, m.ExternalCalendarID, m.ExternalEventID, m.SyncStatus, m.SourceVersion, m.LastSyncedAt).Error
	})
}

// AcquireCalendar запрашивает текущую аренду для синхронизации календаря для одного воркера.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//
// @returns:
//   - result1: Обратный вызов, используемый для завершения операции или освобождения
//     ресурсов.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) AcquireCalendar(ctx context.Context, conferenceID string) (func(), error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	key := "calendar-sync:" + conferenceID
	var acquired bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !acquired {
		_ = conn.Close()
		return nil, jobs.Error{Code: "calendar_sync_busy", Retryable: true, RetryAfter: time.Second}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := conn.ExecContext(cleanup, "SELECT pg_advisory_unlock(hashtextextended($1,0))", key); err != nil {
				// Never Возвращает сеанс с неизвестным состоянием консультативной блокировки в
				// общий пул подключений.
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
			_ = conn.Close()
		})
	}, nil
}

// FailCalendars записывает информацию о сбое при ограниченной синхронизации для затронутых
// сопоставлений календарей.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - code: Ограниченный публичный статус или код ошибки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) FailCalendars(ctx context.Context, job jobs.Job, code string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := integrationLease(tx, job); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO calendar_event_mappings(conference_id,connection_id,provider,external_calendar_id,sync_status,source_version)
	SELECT c.id,conn.id,conn.provider,conn.calendar_id,'failed',? FROM conferences c JOIN calendar_connections conn ON conn.user_id=c.owner_id AND conn.status='connected'
	WHERE c.id=? AND c.integration_version=? ON CONFLICT(conference_id,connection_id) DO UPDATE SET sync_status='failed',source_version=EXCLUDED.source_version WHERE calendar_event_mappings.source_version<=EXCLUDED.source_version`, job.Version, job.ConferenceID, job.Version).Error
	})
}

// Fanout создаёт задания по доставке с дедупликацией для получателей, разрешённых сохранёнными
// настройками.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - event: Доверенное событие для публикации или обработки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) Fanout(ctx context.Context, job jobs.Job, event string) error {
	switch event {
	case "conference.invited", "conference.rescheduled", "conference.cancelled", "conference.soon":
	default:
		return jobs.Error{Code: "integration_event"}
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := integrationLease(tx, job); err != nil {
			return err
		}
		var c u.ConferenceSnapshot
		if err := tx.Table("conferences").Where("id=?", job.ConferenceID).Take(&c).Error; err != nil {
			return err
		}
		if c.IntegrationVersion != job.Version {
			return jobs.ErrSkip
		}
		if event == "conference.soon" && (c.Status != "scheduled" || c.ScheduledAt == nil || !c.ScheduledAt.After(time.Now())) {
			return jobs.ErrSkip
		}
		var payload map[string]any
		if json.Unmarshal(job.Payload, &payload) != nil {
			return jobs.Error{Code: "integration_payload"}
		}
		payload["conferenceId"] = job.ConferenceID
		if c.ScheduledAt != nil {
			payload["scheduledAt"] = c.ScheduledAt
		}
		delete(payload, "event")
		kind := event
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		// Привязывает каждый пакет к 100 спискам участников и сохраняет задание по продолжению
		// работы в рамках той же транзакции.
		type recipient struct {
			ID     string
			UserID string
		}
		q := tx.Table("conference_participants p").Select("p.id,p.user_id").Where("p.conference_id=? AND p.user_id IS NOT NULL AND p.status NOT IN ('kicked','rejected')", job.ConferenceID)
		q = q.Where("NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))")
		if event == "conference.invited" {
			q = q.Where("NOT EXISTS(SELECT 1 FROM conference_invitations i WHERE i.conference_id=p.conference_id AND i.user_id=p.user_id)")
		}
		if cursor, ok := payload["cursorParticipantId"].(string); ok && cursor != "" {
			q = q.Where("p.id>?::uuid", cursor)
		}
		q = q.Where("p.admission_state IN ('admitted','waiting')")
		if job.UserID != nil {
			q = q.Where("p.user_id=?", *job.UserID)
		}
		var recipients []recipient
		if err := q.Order("p.id").Limit(100).Scan(&recipients).Error; err != nil {
			return err
		}
		for _, recipient := range recipients {
			userID := recipient.UserID
			p := d.DefaultPreferences(userID)
			err := tx.Where("user_id=?", userID).Take(&p).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if !p.Allows(kind) {
				continue
			}
			entity := job.ConferenceID
			key := fmt.Sprintf("integration-notification:%s:%s:%d:%s", event, entity, job.Version, userID)
			if event == "conference.soon" {
				offset, _ := payload["offsetSec"].(float64)
				if int64(offset) == 900 {
					key = fmt.Sprintf("soon:%s:%d", c.ID, c.ScheduledAt.UnixMicro())
				} else {
					key = fmt.Sprintf("reminder:%s:%d:%d", c.ID, c.ScheduledAt.UnixMicro(), int64(offset))
				}
			}
			live := "p.admission_state IN ('admitted','waiting') AND p.status NOT IN ('kicked','rejected') AND c.integration_version=?"
			preference := ""
			switch kind {
			case "conference.invited", "conference.rescheduled", "conference.cancelled":
				preference = "invitation"
			case "conference.soon":
				preference = "reminder"
			}
			if preference != "" {
				live += " AND COALESCE((SELECT pref." + preference + " FROM notification_preferences pref WHERE pref.user_id=p.user_id),TRUE)"
			}
			live += " AND NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))"
			args := []any{kind, string(encoded), key, userID, job.ConferenceID, job.Version}
			if err := tx.Exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key) SELECT gen_random_uuid(),p.user_id,?,?::jsonb,? FROM conference_participants p JOIN conferences c ON c.id=p.conference_id WHERE p.user_id=? AND p.conference_id=? AND `+live+` ON CONFLICT(user_id,dedup_key) DO NOTHING`, args...).Error; err != nil {
				return err
			}
		}
		if len(recipients) == 100 {
			payload["event"] = event
			payload["cursorParticipantId"] = recipients[len(recipients)-1].ID
			encoded, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("integration-fanout:%s:%s:%d:%v:%s", event, job.EntityID, job.Version, payload["offsetSec"], recipients[len(recipients)-1].ID)
			if err := tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,version,payload,dedup_key) VALUES('integrations.event',?,?,?,?,?::jsonb,?) ON CONFLICT(dedup_key) DO NOTHING`, job.EntityID, job.ConferenceID, job.UserID, job.Version, string(encoded), key).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ScheduleReminders создаёт задания по напоминаниям для конференций, попадающих в настроенное
// окно напоминаний.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - offsets: Настраивает смещение напоминаний относительно запланированного времени
//     проведения конференции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) ScheduleReminders(ctx context.Context, offsets []time.Duration) error {
	if err := r.db.WithContext(ctx).Exec(`DELETE FROM calendar_oauth_states WHERE id IN (SELECT id FROM calendar_oauth_states WHERE expires_at<now() OR used_at IS NOT NULL ORDER BY expires_at,id LIMIT 1000)`).Error; err != nil {
		return err
	}
	for _, offset := range offsets {
		seconds := int64(offset / time.Second)
		// Reminder jobs use invitation IDs so external recipients do not need accounts.
		// A schedule version invalidates queued mail after a reschedule or cancellation.
		// Late invitations keep their immediate email without a second, back-to-back email.
		if err := r.db.WithContext(ctx).Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,version,payload,dedup_key,available_at)
		SELECT 'integrations.invitation_reminder',i.id,c.id,i.user_id,c.integration_version,
		jsonb_build_object('scheduledAt',c.scheduled_at,'offsetSec',?::bigint),
		'invitation-reminder:'||i.id::text||':'||c.integration_version::text||':'||?::bigint::text,
		GREATEST(now(),c.scheduled_at-(?*interval '1 second'))
		FROM conference_invitations i JOIN conferences c ON c.id=i.conference_id
		WHERE c.kind='meeting' AND c.status='scheduled' AND c.scheduled_at>now() AND i.status='sent'
		AND c.scheduled_at-(?*interval '1 second')>=GREATEST(i.sent_at,c.integration_version_at)
		AND NOT EXISTS(SELECT 1 FROM background_jobs j WHERE j.dedup_key='invitation-reminder:'||i.id::text||':'||c.integration_version::text||':'||?::bigint::text)
		ORDER BY c.scheduled_at,i.id LIMIT 1000 ON CONFLICT(dedup_key) DO NOTHING`, seconds, seconds, seconds, seconds, seconds).Error; err != nil {
			return err
		}
		if err := r.db.WithContext(ctx).Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key,available_at)
		SELECT 'integrations.event',c.id,c.id,c.integration_version,jsonb_build_object('event','conference.soon','scheduledAt',c.scheduled_at,'offsetSec',?::bigint),
		'reminder-job:'||c.id::text||':'||c.integration_version::text||':'||?::bigint::text,
		GREATEST(now(),c.scheduled_at-(?*interval '1 second')) FROM conferences c
		WHERE c.status='scheduled' AND c.scheduled_at>now() AND c.scheduled_at-(?*interval '1 second')>=c.integration_version_at
		AND NOT EXISTS(SELECT 1 FROM background_jobs j WHERE j.dedup_key='reminder-job:'||c.id::text||':'||c.integration_version::text||':'||?::bigint::text)
		ORDER BY c.scheduled_at,c.id LIMIT 1000 ON CONFLICT(dedup_key) DO NOTHING`, seconds, seconds, seconds, seconds, seconds).Error; err != nil {
			return err
		}
	}
	return nil
}

// Delivery загружает метаданные уведомлений и доставки для конкретного получателя для
// арендованного задания.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: Результирующая доставка.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) Delivery(ctx context.Context, job jobs.Job) (u.Delivery, error) {
	var result u.Delivery
	if job.UserID == nil {
		return result, jobs.Error{Code: "delivery_user"}
	}
	if err := r.db.WithContext(ctx).Where("id=? AND user_id=?", job.EntityID, *job.UserID).Take(&result.Notification).Error; err != nil {
		return result, err
	}
	if err := r.db.WithContext(ctx).Table("users").Where("id=?", *job.UserID).Pluck("email", &result.Email).Error; err != nil {
		return result, err
	}
	p, err := r.Preferences(ctx, *job.UserID)
	if err != nil {
		return result, err
	}
	result.Preferences = p
	conference, err := r.Conference(ctx, job.ConferenceID)
	if err != nil {
		return result, err
	}
	kind := result.Notification.Type
	if kind == "conference.soon" {
		var invitations int64
		if err := r.db.WithContext(ctx).Table("conference_invitations").Where("conference_id=? AND user_id=?", job.ConferenceID, *job.UserID).Count(&invitations).Error; err != nil {
			return result, err
		}
		result.ExplicitInvitation = invitations > 0
	}
	if kind == "conference.soon" || kind == "conference.rescheduled" || kind == "conference.invited" {
		if conference.Status != "scheduled" || conference.ScheduledAt == nil {
			return result, nil
		}
		if result.Notification.Payload.ScheduledAt != nil && !result.Notification.Payload.ScheduledAt.Equal(*conference.ScheduledAt) {
			return result, nil
		}
		if kind == "conference.soon" && !conference.ScheduledAt.After(time.Now()) {
			return result, nil
		}
	}
	if kind == "conference.cancelled" && conference.Status != "cancelled" {
		return result, nil
	}
	if kind == "recording.ready" {
		var available int64
		if err := r.db.WithContext(ctx).Table("record").Where("uuid=? AND platform_conference_id=? AND deleted_at IS NULL AND status IN ('ready','partial_ready')", result.Notification.Payload.RecordingID, job.ConferenceID).Count(&available).Error; err != nil {
			return result, err
		}
		if available == 0 {
			return result, nil
		}
	}
	var count int64
	q := r.db.WithContext(ctx).Table("conference_participants").Where("user_id=? AND conference_id=? AND status NOT IN ('kicked','rejected')", *job.UserID, job.ConferenceID)
	q = q.Where("NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=conference_participants.conference_id AND cp.user_id=conference_participants.user_id AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))")
	if result.Notification.Type == "conference.invited" || result.Notification.Type == "conference.rescheduled" || result.Notification.Type == "conference.cancelled" || result.Notification.Type == "conference.soon" {
		q = q.Where("admission_state IN ('admitted','waiting')")
	} else if result.Notification.Type == "admission.decided" {
		q = q.Where("admission_state IN ('admitted','rejected')")
	} else {
		q = q.Where("admission_state='admitted' AND status IN ('joined','left')")
	}
	if err := q.Count(&count).Error; err != nil {
		return result, err
	}
	result.Allowed = count > 0
	return result, nil
}

// CompleteDelivery записывает результат доставки только для текущего арендованного задания.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - status: Состояние жизненного цикла или статус ответа на запись.
//   - code: Ограниченный публичный статус или код ошибки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) CompleteDelivery(ctx context.Context, job jobs.Job, status, code string) error {
	if job.UserID == nil {
		return jobs.Error{Code: "delivery_user"}
	}
	var payload struct {
		Channel string `json:"channel"`
	}
	if json.Unmarshal(job.Payload, &payload) != nil {
		return jobs.Error{Code: "delivery_payload"}
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := integrationLease(tx, job); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO integration_deliveries(job_id,user_id,channel,status,error_code) VALUES(?,?,?,?,?) ON CONFLICT(job_id) DO NOTHING`, job.ID, *job.UserID, payload.Channel, status, code).Error
	})
}

// ScheduleCalendarSync ставит в очередь задания по дедупликации календарей для текущей версии
// расписания конференций.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *IntegrationRepository) ScheduleCalendarSync(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key) SELECT 'integrations.calendar',id,id,integration_version,'{}'::jsonb,'calendar-connect:'||id::text||':'||integration_version::text||':'||? FROM conferences WHERE owner_id=? AND status='scheduled' ORDER BY scheduled_at,id LIMIT 1000 ON CONFLICT(dedup_key) DO NOTHING`, userID, userID).Error
}

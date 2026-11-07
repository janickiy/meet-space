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
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/directory"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// DirectoryRepository сохраняет связь с каталогом, не связывая аккаунты по совпадению email.
type DirectoryRepository struct{ db *gorm.DB }

func NewDirectoryRepository(db *gorm.DB) *DirectoryRepository {
	return &DirectoryRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

type ldapIdentity struct {
	ProviderID string `gorm:"primaryKey"`
	ExternalID string `gorm:"primaryKey"`
	UserID     string
	Login      string
	LastSeenAt time.Time
}

func (ldapIdentity) TableName() string { return "ldap_identities" }

// lockLDAPSettings сериализует изменение настроек с применением данных и выдачей SID.
func lockLDAPSettings(tx *gorm.DB, revision int64) error {
	var row struct {
		Revision int64
		Enabled  bool
	}
	result := tx.Raw("SELECT revision, COALESCE((values->>'ldapEnabled')::boolean,false) AS enabled FROM platform_settings WHERE singleton FOR SHARE").Scan(&row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 || !row.Enabled {
		return apperrors.New(apperrors.ErrUnavailable, "Вход через LDAP отключён")
	}
	if row.Revision != revision {
		return apperrors.New(apperrors.ErrConflict, "Настройки LDAP изменились. Повторите операцию.")
	}
	return nil
}

// Все изменения привязок каталога используют один порядок блокировок.
func lockLDAPDirectory(tx *gorm.DB) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(1297301840, 1)").Error
}

func validateLDAPEntry(entry directory.Entry) (directory.Entry, error) {
	entry.Email = users.NormalizeEmail(entry.Email)
	entry.Login = strings.TrimSpace(entry.Login)
	if entry.ExternalID == "" || len(entry.ExternalID) > 512 || strings.ContainsRune(entry.ExternalID, 0) || !utf8.ValidString(entry.ExternalID) || entry.Login == "" || len(entry.Login) > 254 || strings.ContainsRune(entry.Login, 0) || !utf8.ValidString(entry.Login) || users.ValidateEmail(entry.Email) != nil {
		return entry, apperrors.New(apperrors.ErrInvalidInput, "В записи LDAP отсутствует допустимый идентификатор, логин или email")
	}
	if entry.DisplayName == "" {
		entry.DisplayName = entry.Login
	}
	var err error
	entry.DisplayName, err = users.NormalizeDisplayName(entry.DisplayName)
	if err != nil {
		return entry, apperrors.New(apperrors.ErrInvalidInput, "Недопустимое имя пользователя LDAP")
	}
	return entry, nil
}

func directoryWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return apperrors.New(apperrors.ErrConflict, "Email LDAP уже связан с другой учётной записью. Автоматическое объединение аккаунтов запрещено.")
	}
	return err
}

// UpsertLDAP выполняется только после успешной проверки пароля в каталоге.
func (r *DirectoryRepository) UpsertLDAP(ctx context.Context, entry directory.Entry, revision int64) (users.User, error) {
	entry, err := validateLDAPEntry(entry)
	if err != nil {
		return users.User{}, err
	}
	if entry.Disabled {
		return users.User{}, apperrors.ErrAccountBlocked
	}
	var user users.User
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockLDAPSettings(tx, revision); err != nil {
			return err
		}
		if err := lockLDAPDirectory(tx); err != nil {
			return err
		}
		var now time.Time
		if err := tx.Raw("SELECT clock_timestamp()").Scan(&now).Error; err != nil {
			return err
		}
		var writeErr error
		user, _, _, writeErr = applyLDAPEntry(tx, entry, now, nil)
		return writeErr
	})
	user.LDAPRevision = revision
	return user, directoryWriteError(err)
}

// applyLDAPEntry обновляет профиль по неизменяемой привязке; cutoff защищает от устаревшего снимка.
func applyLDAPEntry(tx *gorm.DB, entry directory.Entry, seenAt time.Time, cutoff *time.Time) (users.User, bool, bool, error) {
	var identity ldapIdentity
	err := tx.Where("provider_id='ldap' AND external_id=?", entry.ExternalID).Take(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user := users.User{ID: uuid.NewString(), Email: entry.Email, DisplayName: &entry.DisplayName, AuthSource: "ldap", LDAPDisabled: entry.Disabled}
		if err := tx.Create(&user).Error; err != nil {
			return user, false, false, err
		}
		identity = ldapIdentity{ProviderID: "ldap", ExternalID: entry.ExternalID, UserID: user.ID, Login: entry.Login, LastSeenAt: seenAt}
		return user, true, false, tx.Create(&identity).Error
	}
	if err != nil {
		return users.User{}, false, false, err
	}
	var user users.User
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", identity.UserID).Take(&user).Error; err != nil {
		return user, false, false, err
	}
	if user.AuthSource != "ldap" || user.DeletedAt != nil {
		return user, false, false, apperrors.ErrConflict
	}
	if cutoff != nil && identity.LastSeenAt.After(*cutoff) {
		return user, false, false, nil
	}
	disabled := entry.Disabled && !user.LDAPDisabled
	changes := map[string]any{"email": entry.Email, "display_name": entry.DisplayName, "ldap_disabled": entry.Disabled, "updated_at": gorm.Expr("clock_timestamp()")}
	if user.LDAPDisabled != entry.Disabled {
		changes["auth_version"] = gorm.Expr("auth_version+1")
	}
	if disabled {
		changes["auth_invalid_before"] = gorm.Expr("clock_timestamp()")
	}
	if err = tx.Model(&users.User{}).Where("id=?", user.ID).Updates(changes).Error; err != nil {
		return user, false, false, err
	}
	if disabled {
		if err = revokeLDAPSessions(tx, []string{user.ID}); err != nil {
			return user, false, false, err
		}
	}
	if err = tx.Model(&ldapIdentity{}).Where("provider_id='ldap' AND external_id=?", entry.ExternalID).Updates(map[string]any{"login": entry.Login, "last_seen_at": seenAt}).Error; err != nil {
		return user, false, false, err
	}
	err = tx.Where("id=?", user.ID).Take(&user).Error
	return user, false, disabled, err
}

func revokeLDAPSessions(tx *gorm.DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Exec("UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE user_id IN ?", ids).Error; err != nil {
		return err
	}
	return tx.Exec("UPDATE auth_legacy_tokens SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE user_id IN ?", ids).Error
}

// ApplyLDAPSnapshot применяет только полный снимок одной транзакцией; ошибка откатывает все изменения.
func (r *DirectoryRepository) ApplyLDAPSnapshot(ctx context.Context, entries []directory.Entry, revision int64, startedAt time.Time) (directory.LDAPApplyResult, error) {
	var out directory.LDAPApplyResult
	if startedAt.IsZero() || len(entries) > directory.MaxUsers {
		return out, apperrors.ErrInvalidInput
	}
	validated := make([]directory.Entry, 0, len(entries))
	ids, emails := make(map[string]bool), make(map[string]bool)
	for _, entry := range entries {
		value, err := validateLDAPEntry(entry)
		if err != nil {
			return out, err
		}
		if ids[value.ExternalID] || emails[value.Email] {
			return out, apperrors.New(apperrors.ErrConflict, "LDAP вернул повторяющийся идентификатор или email")
		}
		ids[value.ExternalID], emails[value.Email] = true, true
		validated = append(validated, value)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockLDAPSettings(tx, revision); err != nil {
			return err
		}
		if job, ok := directory.SyncLeaseFromContext(ctx); ok {
			var id string
			result := tx.Raw("SELECT id FROM background_jobs WHERE id=? AND kind=? AND state='processing' AND lease_token=? AND lease_until>clock_timestamp() FOR UPDATE", job.ID, directory.SyncJobKind, job.LeaseToken).Scan(&id)
			if result.Error != nil {
				return result.Error
			}
			if id == "" {
				return jobs.ErrLeaseLost
			}
		}
		if err := lockLDAPDirectory(tx); err != nil {
			return err
		}
		var state struct{ LastSnapshotAt *time.Time }
		if err := tx.Raw("SELECT last_snapshot_at FROM ldap_directory_state WHERE singleton FOR UPDATE").Scan(&state).Error; err != nil {
			return err
		}
		if state.LastSnapshotAt != nil && !startedAt.After(*state.LastSnapshotAt) {
			return apperrors.New(apperrors.ErrConflict, "Снимок LDAP устарел")
		}
		for _, entry := range validated {
			user, created, disabled, err := applyLDAPEntry(tx, entry, startedAt, &startedAt)
			if err != nil {
				return err
			}
			if created {
				out.Created++
			} else {
				out.Updated++
			}
			if disabled {
				out.Disabled++
				out.DisabledUserIDs = append(out.DisabledUserIDs, user.ID)
			}
		}
		externalIDs := make([]string, 0, len(ids))
		for id := range ids {
			externalIDs = append(externalIDs, id)
		}
		missing := tx.Table("ldap_identities i").Select("i.user_id").Joins("JOIN users u ON u.id=i.user_id").Where("i.provider_id='ldap' AND i.last_seen_at<=? AND u.auth_source='ldap' AND u.deleted_at IS NULL AND NOT u.ldap_disabled", startedAt)
		if len(externalIDs) > 0 {
			missing = missing.Where("i.external_id NOT IN ?", externalIDs)
		}
		var absent []string
		if err := missing.Scan(&absent).Error; err != nil {
			return err
		}
		if len(absent) > 0 {
			if err := tx.Model(&users.User{}).Where("id IN ?", absent).Updates(map[string]any{"ldap_disabled": true, "auth_invalid_before": gorm.Expr("clock_timestamp()"), "auth_version": gorm.Expr("auth_version+1"), "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
				return err
			}
			if err := revokeLDAPSessions(tx, absent); err != nil {
				return err
			}
			out.Disabled += len(absent)
			out.DisabledUserIDs = append(out.DisabledUserIDs, absent...)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Даже заблокированная строка задания может потерять аренду по времени.
		if job, ok := directory.SyncLeaseFromContext(ctx); ok {
			var valid bool
			if err := tx.Raw("SELECT EXISTS(SELECT 1 FROM background_jobs WHERE id=? AND kind=? AND state='processing' AND lease_token=? AND lease_until>clock_timestamp())", job.ID, directory.SyncJobKind, job.LeaseToken).Scan(&valid).Error; err != nil {
				return err
			}
			if !valid {
				return jobs.ErrLeaseLost
			}
		}
		return tx.Exec("UPDATE ldap_directory_state SET last_snapshot_at=? WHERE singleton", startedAt).Error
	})
	if err != nil {
		return directory.LDAPApplyResult{}, directoryWriteError(err)
	}
	return out, nil
}

package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/settings"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type SettingsRepository struct{ db *gorm.DB }

type platformSettingsRow struct {
	Singleton                  bool `gorm:"primaryKey"`
	Revision                   int64
	Values                     json.RawMessage `gorm:"type:jsonb"`
	SMTPPasswordCiphertext     string
	LDAPBindPasswordCiphertext string
}

func (platformSettingsRow) TableName() string { return "platform_settings" }
func (r platformSettingsRow) state() settings.State {
	return settings.State{Revision: r.Revision, Overrides: r.Values, SMTPPasswordCiphertext: r.SMTPPasswordCiphertext, LDAPBindPasswordCiphertext: r.LDAPBindPasswordCiphertext}
}
func NewSettingsRepository(db *gorm.DB) *SettingsRepository {
	return &SettingsRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}
func (r *SettingsRepository) Load(ctx context.Context) (settings.State, error) {
	var row platformSettingsRow
	err := r.db.WithContext(ctx).Where("singleton=TRUE").Take(&row).Error
	return row.state(), err
}
func (r *SettingsRepository) Save(ctx context.Context, input settings.SaveInput, audit admin.AuditInput) (settings.State, error) {
	var row platformSettingsRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminMutationGuard(tx); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("singleton=TRUE").Take(&row).Error; err != nil {
			return err
		}
		if row.Revision != input.ExpectedRevision {
			return apperrors.New(apperrors.ErrConflict, "Настройки уже изменены. Обновите страницу и повторите изменения.")
		}
		var previous, next struct {
			LDAPEnabled bool `json:"ldapEnabled"`
		}
		if err := json.Unmarshal(row.Values, &previous); err != nil {
			return err
		}
		if err := json.Unmarshal(input.Overrides, &next); err != nil {
			return err
		}
		modeChanged := previous.LDAPEnabled != next.LDAPEnabled
		if err := tx.Model(&row).Updates(map[string]any{
			"revision": row.Revision + 1, "values": gorm.Expr("?::jsonb", string(input.Overrides)),
			"smtp_password_ciphertext": input.SMTPPasswordCiphertext, "updated_at": gorm.Expr("clock_timestamp()"),
			"ldap_bind_password_ciphertext": input.LDAPBindPasswordCiphertext,
		}).Error; err != nil {
			return err
		}
		if modeChanged || ldapSecurityChanged(input.Fields) {
			// Смена источника или правил доступа немедленно аннулирует прежние SID.
			if err := tx.Exec(`UPDATE users SET auth_version=auth_version+1, auth_invalid_before=clock_timestamp(), updated_at=clock_timestamp() WHERE (auth_source='ldap' OR (? AND auth_source='local')) AND guest_conference_id IS NULL AND deleted_at IS NULL`, modeChanged).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE user_id IN (SELECT id FROM users WHERE (auth_source='ldap' OR (? AND auth_source='local')) AND guest_conference_id IS NULL)`, modeChanged).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE auth_legacy_tokens SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE user_id IN (SELECT id FROM users WHERE (auth_source='ldap' OR (? AND auth_source='local')) AND guest_conference_id IS NULL)`, modeChanged).Error; err != nil {
				return err
			}
		}
		audit.Action = "settings.update"
		audit.ObjectType = "settings"
		audit.ObjectID = "platform"
		audit.Metadata = map[string]any{"fields": input.Fields}
		if err := auditInsert(tx, audit); err != nil {
			return err
		}
		return tx.Where("singleton=TRUE").Take(&row).Error
	})
	return row.state(), err
}

func ldapSecurityChanged(fields []string) bool {
	for _, field := range fields {
		if strings.HasPrefix(field, "ldap") && field != "ldapSyncIntervalMinutes" && field != "ldapTimeoutSeconds" {
			return true
		}
	}
	return false
}

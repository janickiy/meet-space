package postgres

import (
	"context"
	"encoding/json"

	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/settings"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type SettingsRepository struct{ db *gorm.DB }

type platformSettingsRow struct {
	Singleton              bool `gorm:"primaryKey"`
	Revision               int64
	Values                 json.RawMessage `gorm:"type:jsonb"`
	SMTPPasswordCiphertext string
}

func (platformSettingsRow) TableName() string { return "platform_settings" }
func (r platformSettingsRow) state() settings.State {
	return settings.State{Revision: r.Revision, Overrides: r.Values, SMTPPasswordCiphertext: r.SMTPPasswordCiphertext}
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
		if err := tx.Model(&row).Updates(map[string]any{
			"revision": row.Revision + 1, "values": gorm.Expr("?::jsonb", string(input.Overrides)),
			"smtp_password_ciphertext": input.SMTPPasswordCiphertext, "updated_at": gorm.Expr("clock_timestamp()"),
		}).Error; err != nil {
			return err
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

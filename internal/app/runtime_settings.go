package app

import (
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/chat"
	domain "github.com/janickiy/meet-space/internal/domain/settings"
	postgresinfra "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	settingsusecase "github.com/janickiy/meet-space/internal/usecase/settings"
	"gorm.io/gorm"
)

func newRuntimeSettings(cfg config.Config, db *gorm.DB) (*settingsusecase.Service, error) {
	media, err := config.LoadMedia()
	if err != nil {
		return nil, err
	}
	return runtimeSettingsWithMedia(cfg, media, db)
}
func runtimeSettingsWithMedia(cfg config.Config, media config.MediaConfig, db *gorm.DB) (*settingsusecase.Service, error) {
	protector, err := security.NewProviderTokens(cfg.StageSeven.EncryptionKey)
	if err != nil {
		return nil, err
	}
	var cipher settingsusecase.Cipher
	if protector != nil {
		cipher = protector
	}
	smtp := cfg.StageSeven.SMTP
	if smtp.Port == 0 {
		smtp.Port = 465
	}
	if smtp.TLSMode == "" {
		smtp.TLSMode = "tls"
	}
	defaults := domain.Values{
		AttachmentMaxBytes: chat.MaxAttachmentBytes, MediaMaxPeers: media.MaxPeers, MediaMaxRooms: media.MaxRooms, RecordingRetentionDays: domain.DefaultRecordingRetentionDays,
		SMTPEnabled: cfg.StageSeven.Email.Mode == "smtp", SMTPHost: smtp.Host, SMTPPort: smtp.Port, SMTPUsername: smtp.Username,
		SMTPFrom: smtp.From, SMTPTLSMode: smtp.TLSMode,
	}
	return settingsusecase.NewService(postgresinfra.NewSettingsRepository(db), defaults, smtp.Password, cipher, cfg.StageSeven.PublicURL), nil
}

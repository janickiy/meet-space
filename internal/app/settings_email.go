package app

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	domainsettings "github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/infrastructure/providers"
)

type smtpSettings interface {
	domainsettings.Reader
	SMTP(context.Context) (domainsettings.SMTPConfig, error)
}
type settingsEmail struct {
	settings     smtpSettings
	fallback     integrations.EmailProvider
	fallbackMode string
}

func (p settingsEmail) Mode(ctx context.Context) (string, error) {
	values, err := p.settings.Snapshot(ctx)
	if err != nil {
		return "", err
	}
	if values.SMTPEnabled {
		return "smtp", nil
	}
	if p.fallbackMode != "smtp" {
		return p.fallbackMode, nil
	}
	return "noop", nil
}
func (p settingsEmail) Send(ctx context.Context, message integrations.EmailMessage) error {
	values, err := p.settings.SMTP(ctx)
	if err != nil {
		return err
	}
	if !values.Enabled {
		if p.fallbackMode != "smtp" && p.fallback != nil {
			return p.fallback.Send(ctx, message)
		}
		return jobs.ErrSkip
	}
	sender, err := providers.NewSMTPEmail(providers.SMTPConfig{Host: values.Host, Port: values.Port, Username: values.Username, Password: values.Password, From: values.From, TLSMode: values.TLSMode, Timeout: 15 * time.Second})
	if err != nil {
		return jobs.Error{Code: "smtp_configuration_invalid", Retryable: false}
	}
	return measuredEmail{sender}.Send(ctx, message)
}

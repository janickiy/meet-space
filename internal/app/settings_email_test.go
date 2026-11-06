package app

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/domain/settings"
)

type mailSettingsFixture struct {
	values settings.Values
	smtp   settings.SMTPConfig
	err    error
}

func (p *mailSettingsFixture) Snapshot(context.Context) (settings.Values, error) {
	return p.values, p.err
}
func (p *mailSettingsFixture) SMTP(context.Context) (settings.SMTPConfig, error) {
	return p.smtp, p.err
}

type mailFallbackFixture struct{ calls int }

func (p *mailFallbackFixture) Send(context.Context, integrations.EmailMessage) error {
	p.calls++
	return nil
}
func TestSettingsEmailModeAndDisableApplyWithoutRestart(t *testing.T) {
	ctx := context.Background()
	settings := &mailSettingsFixture{}
	fallback := &mailFallbackFixture{}
	provider := settingsEmail{settings: settings, fallback: fallback, fallbackMode: "smtp"}
	if mode, err := provider.Mode(ctx); err != nil || mode != "noop" {
		t.Fatal(mode, err)
	}
	if err := provider.Send(ctx, integrations.EmailMessage{}); !errors.Is(err, jobs.ErrSkip) || fallback.calls != 0 {
		t.Fatal("disabled SMTP still sends", err)
	}
	settings.values.SMTPEnabled = true
	if mode, err := provider.Mode(ctx); err != nil || mode != "smtp" {
		t.Fatal("runtime enabling not reflected", mode, err)
	}
	settings.values.SMTPEnabled = false
	if mode, err := provider.Mode(ctx); err != nil || mode != "noop" {
		t.Fatal("runtime disabling not reflected", mode, err)
	}
	settings.err = errors.New("synthetic database unavailable")
	if _, err := provider.Mode(ctx); err == nil {
		t.Fatal("settings failure hidden")
	}
	if err := provider.Send(ctx, integrations.EmailMessage{}); err == nil {
		t.Fatal("send ignores settings failure")
	}
}
func TestSettingsEmailKeepsExistingHTTPProvider(t *testing.T) {
	fallback := &mailFallbackFixture{}
	provider := settingsEmail{settings: &mailSettingsFixture{}, fallback: fallback, fallbackMode: "http"}
	if mode, err := provider.Mode(context.Background()); err != nil || mode != "http" {
		t.Fatal(mode, err)
	}
	if err := provider.Send(context.Background(), integrations.EmailMessage{}); err != nil || fallback.calls != 1 {
		t.Fatal(err)
	}
}

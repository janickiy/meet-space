package integrations

import (
	"context"
	"errors"
	"testing"

	d "github.com/janickiy/meet-space/internal/domain/integrations"
)

type dynamicMailFixture struct {
	mode string
	err  error
}

func (p *dynamicMailFixture) Mode(context.Context) (string, error)       { return p.mode, p.err }
func (p *dynamicMailFixture) Send(context.Context, d.EmailMessage) error { return nil }
func TestEmailCapabilitiesFollowRuntimeMode(t *testing.T) {
	provider := &dynamicMailFixture{mode: "noop"}
	service := Service{adapters: d.Providers{Email: provider, Capabilities: d.Capabilities{Email: "noop"}}}
	if enabled, err := service.emailEnabled(context.Background()); err != nil || enabled {
		t.Fatal(enabled, err)
	}
	provider.mode = "smtp"
	if enabled, err := service.emailEnabled(context.Background()); err != nil || !enabled {
		t.Fatal(enabled, err)
	}
	if service.Capabilities().Email != "smtp" {
		t.Fatal("capabilities stale after SMTP enable")
	}
	provider.err = errors.New("synthetic unavailable")
	if _, err := service.emailEnabled(context.Background()); err == nil {
		t.Fatal("failed settings treated as success")
	}
}

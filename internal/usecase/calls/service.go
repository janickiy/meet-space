package calls

import (
	"context"
	"log/slog"
	"time"

	domain "github.com/janickiy/meet-space/internal/domain/calls"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

type Repository interface {
	Create(context.Context, string, string, string, string) (domain.Call, bool, error)
	Get(context.Context, string, string) (domain.Call, error)
	Active(context.Context, string) (*domain.Call, error)
	Change(context.Context, string, string, string, string) (domain.Call, bool, error)
	Expiring(context.Context) ([]string, error)
}
type Bus interface {
	Publish(context.Context, string, realtime.Envelope) error
}
type Observer interface{ ConferenceChanged(context.Context, string) }
type Service struct {
	Repo     Repository
	Bus      Bus
	Observer Observer
}

func (s *Service) notify(ctx context.Context, c domain.Call) {
	event := realtime.Event("call.updated", "", map[string]any{"call": c})
	// Pubsub is an acceleration only: every client also restores authoritative
	// state after reconnect and periodically while a call is displayed.
	for _, user := range []string{c.CallerUserID, c.CalleeUserID} {
		if s.Bus != nil {
			if err := s.Bus.Publish(ctx, user, event); err != nil {
				slog.Warn("call delivery failed", "call_id", c.ID, "error", err)
			}
			_ = s.Bus.Publish(ctx, user, realtime.Event("conversation.updated", "", map[string]any{"conversationId": c.ConversationID, "type": "direct"}))
		}
	}
	if c.ConferenceID != nil && s.Observer != nil {
		s.Observer.ConferenceChanged(ctx, *c.ConferenceID)
	}
}
func (s *Service) Create(ctx context.Context, actor, target, request, client string) (domain.Call, bool, error) {
	c, created, err := s.Repo.Create(ctx, actor, target, request, client)
	if err == nil && created {
		s.notify(ctx, c)
	}
	return c, created, err
}
func (s *Service) Get(ctx context.Context, actor, id string) (domain.Call, error) {
	c, err := s.Repo.Get(ctx, actor, id)
	if err != nil || !c.Expired(time.Now().UTC()) {
		return c, err
	}
	return s.Change(ctx, actor, id, "expire", "")
}
func (s *Service) Active(ctx context.Context, actor string) (*domain.Call, error) {
	c, err := s.Repo.Active(ctx, actor)
	if err != nil || c == nil || !c.Expired(time.Now().UTC()) {
		return c, err
	}
	_, err = s.Change(ctx, actor, c.ID, "expire", "")
	return nil, err
}
func (s *Service) Change(ctx context.Context, actor, id, action, client string) (domain.Call, error) {
	c, changed, err := s.Repo.Change(ctx, actor, id, action, client)
	if err == nil && changed {
		s.notify(ctx, c)
	}
	return c, err
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		op, cancel := context.WithTimeout(ctx, 10*time.Second)
		ids, err := s.Repo.Expiring(op)
		if err != nil && ctx.Err() == nil {
			slog.Warn("call expiry failed", "error", err)
		}
		for _, id := range ids {
			if _, err := s.Change(op, "", id, "expire", ""); err != nil && ctx.Err() == nil {
				slog.Warn("call expiry failed", "call_id", id, "error", err)
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

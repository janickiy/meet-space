package personal

import (
	"context"
	"time"

	domain "github.com/janickiy/meet-space/internal/domain/personal"
)

// GroupMemberReader authorizes the actor before returning current group members.
type GroupMemberReader interface {
	GroupMembers(context.Context, string, string) ([]domain.Member, error)
}

// ReadGroupMembers enriches an authorized membership projection with optional
// presence. Unlike a direct peer-presence request, the member list remains useful
// when presence is unavailable: unknown statuses retain their existing nil value.
func ReadGroupMembers(ctx context.Context, groups GroupMemberReader, presence PresenceReader, actor, id string) ([]domain.Member, error) {
	items, err := groups.GroupMembers(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if presence != nil && len(items) > 0 {
		ids := make([]string, 0, len(items))
		for _, member := range items {
			ids = append(ids, member.ID)
		}
		lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
		online, presenceErr := presence.Online(lookup, ids)
		cancel()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if presenceErr == nil {
			for i := range items {
				if value, known := online[items[i].ID]; known {
					items[i].Online = &value
				}
			}
		}
	}
	return items, nil
}

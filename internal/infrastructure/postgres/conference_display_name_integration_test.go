package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/users"
)

func TestConferenceDisplaysRenamedAccountInExistingMeetingAndChat(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	oldName := "12345678"
	const newName = "Саша"
	const guestAlias = "Гость встречи"
	owner := users.User{
		ID: uuid.NewString(), Email: "owner@example.invalid",
		DisplayName: &oldName, PasswordHash: "synthetic",
	}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	meetingID, ownerMembershipID := uuid.NewString(), uuid.NewString()
	invite := strings.Repeat("a", 32)
	if err := db.Exec("INSERT INTO conferences(id,owner_id,title,invite_code,status,started_at) VALUES(?,?,'Name update',?,'active',now())",
		meetingID, owner.ID, invite).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,admission_state,joined_at) VALUES(?,?,?,?,'owner','joined','admitted',now())",
		ownerMembershipID, meetingID, owner.ID, oldName).Error; err != nil {
		t.Fatal(err)
	}
	guest, guestMembership, err := NewGuestRepository(db).JoinGuest(ctx, invite, guestAlias, "")
	if err != nil {
		t.Fatal(err)
	}

	meetingChat := NewChatRepository(db)
	send := func(senderID, body, replyTo string) chat.Message {
		t.Helper()
		request, fingerprint, err := chat.NormalizeSend(chat.SendRequest{
			ClientRequestID: uuid.NewString(), Text: body, ReplyTo: replyTo,
		})
		if err != nil {
			t.Fatal(err)
		}
		message, _, err := meetingChat.Send(ctx, senderID, meetingID, request, fingerprint)
		if err != nil {
			t.Fatal(err)
		}
		return message
	}
	message := send(owner.ID, "Existing message", "")
	reply := send(guest.ID, "Existing reply", message.ID)
	if message.SenderName != oldName || reply.ReplyPreview == nil || reply.ReplyPreview.SenderName != oldName {
		t.Fatalf("unexpected sender before profile change: %q, reply preview: %+v", message.SenderName, reply.ReplyPreview)
	}

	if _, err := NewUserRepository(db).UpdateDisplayName(ctx, owner.ID, newName); err != nil {
		t.Fatal(err)
	}
	// The meeting alias remains authoritative for a guest even if the user row differs.
	if err := db.Model(&guest).Update("display_name", "Имя вне встречи").Error; err != nil {
		t.Fatal(err)
	}

	repo := NewConferenceRepository(db)
	membership, err := repo.Membership(ctx, meetingID, owner.ID)
	if err != nil || membership.ID != ownerMembershipID || membership.DisplayName != newName {
		t.Fatalf("existing membership: id=%q name=%q error=%v", membership.ID, membership.DisplayName, err)
	}
	checkNames := func(source string, people []conferences.Participant, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", source, err)
		}
		names := make(map[string]string, len(people))
		for _, person := range people {
			names[person.ID] = person.DisplayName
		}
		if names[ownerMembershipID] != newName || names[guestMembership.ID] != guestAlias {
			t.Fatalf("%s: owner=%q guest=%q", source, names[ownerMembershipID], names[guestMembership.ID])
		}
	}
	visible, err := repo.ParticipantsVisible(ctx, meetingID, guest.ID, 20, 0)
	checkNames("participant list", visible, err)
	_, roster, err := NewSessionRepository(db).Roster(ctx, meetingID)
	checkNames("WebSocket roster", roster, err)

	history, err := meetingChat.List(ctx, guest.ID, meetingID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Items) != 2 {
		t.Fatalf("chat history contains %d messages, want 2", len(history.Items))
	}
	original, response := history.Items[0], history.Items[1]
	if original.ID != message.ID || original.Text != message.Text || original.SenderName != newName {
		t.Fatalf("existing message: id=%q text=%q sender=%q", original.ID, original.Text, original.SenderName)
	}
	if response.ID != reply.ID || response.SenderName != guestAlias || response.ReplyPreview == nil ||
		response.ReplyPreview.ID != message.ID || response.ReplyPreview.SenderName != newName {
		t.Fatalf("existing reply: id=%q sender=%q preview=%+v", response.ID, response.SenderName, response.ReplyPreview)
	}
}

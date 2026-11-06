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
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Проверяем настоящий запрос списка и счётчика на истории переписки.
// Время выводится как измерение, но не служит нестабильным порогом теста.
func TestAuditNotificationHistoryUsesMessageIndex(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor, peer := uuid.NewString(), uuid.NewString()
	for _, id := range []string{actor, peer} {
		exec("INSERT INTO users(id,email,password_hash) VALUES(?,?,'synthetic')", id, id+"@example.invalid")
	}
	conversation, _, err := NewPersonalRepository(db).GetOrCreate(ctx, actor, peer)
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO conversation_messages(id,conversation_id,sender_user_id,client_request_id,request_fingerprint,text)
 SELECT md5('audit-message-'||g)::uuid,?,?,md5('audit-request-'||g)::uuid,repeat('0',64),'Message '||g FROM generate_series(1,30000) g`, conversation.ID, peer)
	exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key)
 SELECT gen_random_uuid(),?,'chat.message',jsonb_build_object('conversationId',conversation_id,'messageId',id),id::text
 FROM conversation_messages ORDER BY sequence DESC LIMIT 100`, actor)
	for _, table := range []string{"conversation_messages", "conversation_members", "conversations", "notifications"} {
		exec("ANALYZE " + table)
	}
	trace := &adminQueryTrace{Interface: logger.Discard}
	page, err := NewNotificationRepository(db.Session(&gorm.Session{Logger: trace})).List(ctx, actor, "", 50)
	if err != nil || len(page.Items) != 50 || page.UnreadCount != 100 {
		t.Fatalf("list items=%d unread=%d err=%v", len(page.Items), page.UnreadCount, err)
	}
	if len(trace.statements) != 2 {
		t.Fatalf("expected page + count, got %d", len(trace.statements))
	}
	for i, statement := range trace.statements {
		var last string
		for run := 0; run < 5; run++ {
			if err := db.Raw("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) " + statement).Scan(&last).Error; err != nil {
				t.Fatal(err)
			}
			var plan []map[string]any
			if err := json.Unmarshal([]byte(last), &plan); err != nil {
				t.Fatal(err)
			}
			t.Logf("query=%d run=%d execution_ms=%v", i, run, plan[0]["Execution Time"])
		}
		// ID lookup must not walk all messages once for each visible notification.
		var plans []struct{ Plan map[string]any }
		if err := json.Unmarshal([]byte(last), &plans); err != nil {
			t.Fatal(err)
		}
		found := 0
		idCondition := regexp.MustCompile(`(^|[ (])([a-z_]+\.)?id\s*=`)
		var inspect func(map[string]any)
		inspect = func(node map[string]any) {
			if node["Relation Name"] == "conversation_messages" {
				found++
				t.Logf("query=%d node=%v loops=%v rows_removed=%v shared_hits=%v index=%v", i, node["Node Type"], node["Actual Loops"], node["Rows Removed by Filter"], node["Shared Hit Blocks"], node["Index Name"])
				cond, _ := node["Index Cond"].(string)
				if !idCondition.MatchString(cond) {
					t.Errorf("message lookup must use its ID in index condition; plan=%s", last)
				}
			}
			if children, ok := node["Plans"].([]any); ok {
				for _, child := range children {
					inspect(child.(map[string]any))
				}
			}
		}
		inspect(plans[0].Plan)
		if found == 0 {
			t.Fatal("message plan node missing")
		}
	}
}

// Исходный предикат служит независимым эталоном прав доступа для повреждённых JSON ID.
func legacyNotificationVisibility(alias string) string {
	return `NOT EXISTS (SELECT 1 FROM conference_chat_preferences cp
		WHERE cp.user_id=` + alias + `.user_id AND cp.conference_id::text=` + alias + `.payload->>'conferenceId'
		AND (cp.left_at IS NOT NULL OR NOT cp.notifications_enabled))
		AND (` + alias + `.type<>'chat.message' OR (
		 EXISTS(SELECT 1 FROM conversations c
		 JOIN conversation_members member ON member.conversation_id=c.id AND member.user_id=` + alias + `.user_id
		 JOIN conversation_messages m ON m.conversation_id=c.id AND m.id::text=` + alias + `.payload->>'messageId'
		 WHERE c.id::text=` + alias + `.payload->>'conversationId' AND c.deleted_at IS NULL
		 AND member.left_at IS NULL AND member.hidden_at IS NULL AND member.notifications_enabled
		 AND m.deleted_at IS NULL AND m.sender_user_id<>member.user_id AND m.sequence>member.history_cleared_through)
		 OR (COALESCE(` + alias + `.payload->>'conversationId','')='' AND
		 EXISTS(SELECT 1 FROM chat_messages m JOIN conference_participants p ON p.conference_id=m.conference_id
		 WHERE m.id::text=` + alias + `.payload->>'messageId' AND m.conference_id::text=` + alias + `.payload->>'conferenceId'
		 AND p.user_id=` + alias + `.user_id AND p.admission_state='admitted' AND p.status IN ('joined','left')
		 AND m.deleted_at IS NULL AND m.sender_user_id<>p.user_id))))`
}

func TestAuditNotificationVisibilityPreservesCanonicalIDsAndACL(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	actor, peer := uuid.NewString(), uuid.NewString()
	for _, id := range []string{actor, peer} {
		exec("INSERT INTO users(id,email,password_hash) VALUES(?,?,'synthetic')", id, id+"@example.invalid")
	}
	conversation, _, err := NewPersonalRepository(db).GetOrCreate(ctx, actor, peer)
	if err != nil {
		t.Fatal(err)
	}
	message := "abcdefab-cdef-4abc-8def-abcdefabcdef"
	exec("INSERT INTO conversation_messages(id,conversation_id,sender_user_id,client_request_id,request_fingerprint,text) VALUES(?,?,?,gen_random_uuid(),repeat('0',64),'Hi')", message, conversation.ID, peer)
	nid := uuid.NewString()
	exec("INSERT INTO notifications(id,user_id,type,payload,dedup_key) VALUES(?,?,'chat.message','{}','audit')", nid, actor)
	compare := func(want bool) {
		t.Helper()
		var current, legacy bool
		for _, target := range []struct {
			sql  string
			dest *bool
		}{{notificationVisibilitySQL("n"), &current}, {legacyNotificationVisibility("n"), &legacy}} {
			if err := db.Raw("SELECT ("+target.sql+") FROM notifications n WHERE id=?", nid).Scan(target.dest).Error; err != nil {
				t.Fatal(err)
			}
		}
		if current != legacy || current != want {
			t.Fatalf("visibility current=%v legacy=%v want=%v", current, legacy, want)
		}
	}
	set := func(payload map[string]any) {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		exec("UPDATE notifications SET payload=?::jsonb WHERE id=?", string(raw), nid)
	}
	for _, field := range []string{"messageId", "conversationId"} {
		for _, value := range []any{nil, "", "not-a-uuid", "{}", 123, map[string]any{"id": message}, strings.ToUpper(message), strings.ReplaceAll(message, "-", ""), "{" + message + "}", message + "\n"} {
			payload := map[string]any{"messageId": message, "conversationId": conversation.ID}
			payload[field] = value
			set(payload)
			compare(false)
		}
	}
	set(map[string]any{"messageId": message, "conversationId": conversation.ID})
	compare(true)
	for _, change := range []struct{ set, reset string }{
		{"UPDATE conversation_messages SET deleted_at=now()", "UPDATE conversation_messages SET deleted_at=NULL"},
		{"UPDATE conversation_members SET notifications_enabled=false", "UPDATE conversation_members SET notifications_enabled=true"},
		{"UPDATE conversation_members SET hidden_at=now()", "UPDATE conversation_members SET hidden_at=NULL"},
		{"UPDATE conversation_members SET history_cleared_through=100000", "UPDATE conversation_members SET history_cleared_through=0"},
	} {
		exec(change.set)
		compare(false)
		exec(change.reset)
		compare(true)
	}
	// Самому автору сообщения уведомление недоступно.
	exec("UPDATE notifications SET user_id=?", peer)
	compare(false)
	exec("UPDATE notifications SET user_id=?", actor)
	group, _, err := NewPersonalRepository(db).CreateGroup(ctx, peer, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Audit", MemberIDs: []string{actor}})
	if err != nil {
		t.Fatal(err)
	}
	gm := uuid.NewString()
	exec("INSERT INTO conversation_messages(id,conversation_id,sender_user_id,client_request_id,request_fingerprint) VALUES(?,?,?,gen_random_uuid(),repeat('0',64))", gm, group.ID, peer)
	set(map[string]any{"conversationId": group.ID, "messageId": gm})
	compare(true)
	exec("UPDATE conversation_members SET left_at=now() WHERE conversation_id=? AND user_id=?", group.ID, actor)
	compare(false)
	exec("UPDATE conversation_members SET left_at=NULL WHERE conversation_id=? AND user_id=?", group.ID, actor)
	compare(true)
	if _, err := NewPersonalRepository(db).DeleteGroup(ctx, peer, group.ID); err != nil {
		t.Fatal(err)
	}
	compare(false)

	conference := uuid.NewString()
	exec("INSERT INTO conferences(id,owner_id,title,invite_code) VALUES(?,?,'Audit',?)", conference, peer, strings.ReplaceAll(uuid.NewString(), "-", ""))
	exec("INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,joined_at) VALUES(gen_random_uuid(),?,?,'Actor','participant','joined',now())", conference, actor)
	cm := uuid.NewString()
	exec("INSERT INTO chat_messages(id,conference_id,sender_user_id,client_request_id,request_fingerprint) VALUES(?,?,?,gen_random_uuid(),repeat('0',64))", cm, conference, peer)
	for _, field := range []string{"messageId", "conferenceId"} {
		for _, value := range []any{nil, "bad", 123, strings.ToUpper(conference), conference + "\n"} {
			payload := map[string]any{"conferenceId": conference, "messageId": cm}
			payload[field] = value
			set(payload)
			compare(false)
		}
	}
	set(map[string]any{"conferenceId": conference, "messageId": cm})
	compare(true)
	for _, status := range []string{"left", "kicked", "rejected", "joined"} {
		admission := status
		if status == "joined" || status == "left" {
			admission = "admitted"
		}
		exec("UPDATE conference_participants SET status=?,admission_state=?", status, admission)
		compare(status == "left" || status == "joined")
	}
	exec("UPDATE conference_participants SET admission_state='waiting',status='waiting'")
	compare(false)
	exec("UPDATE conference_participants SET admission_state='admitted',status='joined'")
	compare(true)
	exec("INSERT INTO conference_chat_preferences(conference_id,user_id,notifications_enabled) VALUES(?,?,false)", conference, actor)
	compare(false)
	exec("UPDATE conference_chat_preferences SET notifications_enabled=true")
	compare(true)
	exec("UPDATE conference_chat_preferences SET left_at=now()")
	compare(false)
	exec("UPDATE conference_chat_preferences SET left_at=NULL")
	compare(true)
	exec("UPDATE chat_messages SET deleted_at=now()")
	compare(false)
}

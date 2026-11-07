// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package ldap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/directory"
)

func TestLocalLLDAPReadOnly(t *testing.T) {
	if os.Getenv("LDAP_DEV_INTEGRATION") != "1" {
		t.Skip("set LDAP_DEV_INTEGRATION=1 to test the local LLDAP stand")
	}
	stand := filepath.Join("..", "..", "..", "tools", "ldap-dev", "secrets")
	secretData, err := os.ReadFile(filepath.Join(stand, "credentials.json"))
	if err != nil {
		t.Fatal("read local test credentials:", err)
	}
	var credentials map[string]string
	if err := json.Unmarshal(secretData, &credentials); err != nil {
		t.Fatal("decode test credentials:", err)
	}
	ca, err := os.ReadFile(filepath.Join(stand, "tls", "ca.crt"))
	if err != nil {
		t.Fatal("read local test CA:", err)
	}
	cfg := directory.Config{
		Mode: directory.ModeGeneric, ServerURL: "ldaps://127.0.0.1:6360",
		BaseDN: "dc=meetspace,dc=test", UsersDN: "ou=people,dc=meetspace,dc=test",
		UserFilter: "(objectClass=person)", LoginAttribute: "uid", IDAttribute: "entryUUID",
		EmailAttribute: "mail", NameAttribute: "cn", GroupDN: "cn=meetspace-users,ou=groups,dc=meetspace,dc=test",
		GroupMembershipAttribute: "memberOf", BindDN: "uid=svc-meetspace,ou=people,dc=meetspace,dc=test",
		BindPassword: credentials["svc-meetspace"], CA: string(ca), SyncIntervalMinutes: 60, TimeoutSeconds: 10,
	}
	client := NewClient()
	entries, err := client.Scan(context.Background(), cfg)
	if err != nil || len(entries) != 2 {
		t.Fatalf("read-only scan failed: %v, count=%d", err, len(entries))
	}
	empty := cfg
	empty.UserFilter = "(&(objectClass=person)(uid=__missing_test_user__))"
	zero, err := client.Scan(context.Background(), empty)
	if err != nil || len(zero) != 0 {
		t.Fatalf("complete empty snapshot rejected: %v, count=%d", err, len(zero))
	}
	check, err := client.Test(context.Background(), empty)
	if err != nil || !check.Reachable || check.MatchedUsers != 0 {
		t.Fatalf("empty test result incorrect: %+v %v", check, err)
	}
	entry, err := client.Authenticate(context.Background(), cfg, "alice", credentials["alice"])
	if err != nil || entry.Login != "alice" {
		t.Fatal("allowed user authentication failed:", err)
	}
	if _, err := client.Authenticate(context.Background(), cfg, "outsider", credentials["outsider"]); !errors.Is(err, directory.ErrNotAllowed) {
		t.Fatal("out-of-group user accepted or misclassified:", err)
	}
	if _, err := client.Authenticate(context.Background(), cfg, "alice", "wrong-test-password"); !errors.Is(err, directory.ErrInvalidCredentials) {
		t.Fatal("wrong password accepted or misclassified:", err)
	}
}

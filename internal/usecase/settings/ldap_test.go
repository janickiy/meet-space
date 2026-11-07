// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package settings

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/admin"
	domain "github.com/janickiy/meet-space/internal/domain/settings"
)

func TestLDAPBindPasswordEncryptedAndWriteOnly(t *testing.T) {
	repo := &testRepository{state: domain.State{Overrides: json.RawMessage(`{}`)}}
	svc := NewService(repo, testDefaults(), "", testCipher(t))
	ctx := context.Background()
	patch := `{"revision":0,"changes":{"ldapEnabled":true,"ldapMode":"generic","ldapServerURL":"ldaps://ldap.example.test:636","ldapBaseDN":"dc=example,dc=test","ldapUsersDN":"ou=people,dc=example,dc=test","ldapUserFilter":"(objectClass=person)","ldapLoginAttribute":"uid","ldapIDAttribute":"entryUUID","ldapEmailAttribute":"mail","ldapNameAttribute":"cn","ldapGroupDN":"cn=members,ou=groups,dc=example,dc=test","ldapGroupMembershipAttribute":"memberOf","ldapBindDN":"uid=reader,ou=people,dc=example,dc=test","ldapBindPassword":"SyntheticLDAPSecret"}}`
	view, err := svc.Update(ctx, testPatch(t, patch), admin.AuditInput{})
	if err != nil {
		t.Fatal(err)
	}
	if repo.state.LDAPBindPasswordCiphertext == "" || strings.Contains(repo.state.LDAPBindPasswordCiphertext, "SyntheticLDAPSecret") || strings.Contains(string(repo.state.Overrides), "SyntheticLDAPSecret") {
		t.Fatal("LDAP bind password is not isolated in ciphertext")
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "SyntheticLDAPSecret") || strings.Contains(string(encoded), repo.state.LDAPBindPasswordCiphertext) {
		t.Fatal("LDAP secret leaked in admin JSON")
	}
	for _, item := range view.Items {
		if item.Key == "ldapBindPassword" && (item.Value != nil || !item.Secret || !item.Configured) {
			t.Fatal("unsafe secret metadata")
		}
	}
	config, err := svc.LDAP(ctx)
	if err != nil || !config.Enabled || config.BindPassword != "SyntheticLDAPSecret" {
		t.Fatal("LDAP config decryption failed", err)
	}
	encoded, _ = json.Marshal(config)
	if strings.Contains(string(encoded), "SyntheticLDAPSecret") {
		t.Fatal("LDAP config JSON leaked bind password")
	}
	secret := repo.state.LDAPBindPasswordCiphertext
	if _, err := svc.Update(ctx, testPatch(t, `{"revision":1,"changes":{"ldapBindPassword":"","ldapTimeoutSeconds":20}}`), admin.AuditInput{}); err != nil {
		t.Fatal(err)
	}
	if repo.state.LDAPBindPasswordCiphertext != secret {
		t.Fatal("blank bind password erased or rotated secret")
	}
}

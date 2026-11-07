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
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
	"github.com/janickiy/meet-space/internal/domain/directory"
)

func TestLoginFilterEscapesMetacharacters(t *testing.T) {
	c := directory.Config{UserFilter: "(objectClass=person)", LoginAttribute: "uid"}
	filter := loginFilter(c, "alice*)(uid=*)")
	if strings.Contains(filter, "(uid=*)") || !strings.Contains(filter, `\2a`) || !strings.Contains(filter, `\28`) || !strings.Contains(filter, `\29`) {
		t.Fatalf("login escaped incorrectly: %s", filter)
	}
	if _, err := ldap.CompileFilter(filter); err != nil {
		t.Fatal(err)
	}
}

func TestStartTLSCancellationClosesRawTransport(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	requestSent := make(chan struct{})
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var first [1]byte
		if _, err := conn.Read(first[:]); err == nil {
			close(requestSent)
		}
		_, _ = io.Copy(io.Discard, conn)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		conn *ldap.Conn
		done func()
		err  error
	}
	result := make(chan outcome, 1)
	go func() {
		conn, done, err := open(ctx, directory.Config{ServerURL: "ldap://" + listener.Addr().String(), TimeoutSeconds: 2})
		result <- outcome{conn: conn, done: done, err: err}
	}()
	select {
	case <-requestSent: // StartTLS был отправлен; сервер намеренно не отвечает.
	case <-time.After(2 * time.Second):
		t.Fatal("StartTLS request was not sent")
	}
	cancel()
	select {
	case got := <-result:
		if got.done != nil {
			got.done()
		}
		if got.conn != nil || !errors.Is(got.err, directory.ErrUnavailable) {
			t.Fatalf("cancelled StartTLS returned usable connection: %v", got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartTLS did not stop after context cancellation")
	}
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("raw transport stayed open after cancellation")
	}
}

func TestParseEntryPreservesImmutableIDsAndDisabledAD(t *testing.T) {
	generic := directory.Config{Mode: directory.ModeGeneric, IDAttribute: "entryUUID", LoginAttribute: "uid", EmailAttribute: "mail", NameAttribute: "cn"}
	raw := ldap.NewEntry("uid=alice,ou=people,dc=example,dc=test", map[string][]string{
		"entryUUID": {"123E4567-E89B-12D3-A456-426614174000"}, "uid": {"alice"}, "mail": {"alice@example.test"}, "cn": {"Alice"},
	})
	e, err := parseEntry(generic, raw)
	if err != nil || e.ExternalID != "123e4567-e89b-12d3-a456-426614174000" || e.Disabled {
		t.Fatalf("generic parse: %+v %v", e, err)
	}
	ad := directory.Config{Mode: directory.ModeAD, IDAttribute: "objectGUID", LoginAttribute: "sAMAccountName", EmailAttribute: "mail", NameAttribute: "displayName"}
	raw = ldap.NewEntry("cn=Alice,ou=people,dc=example,dc=test", map[string][]string{
		"objectGUID":     {string([]byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})},
		"sAMAccountName": {"alice"}, "mail": {"alice@example.test"}, "displayName": {"Alice"},
		"userAccountControl": {"514"}, "accountExpires": {"0"},
	})
	e, err = parseEntry(ad, raw)
	if err != nil || e.ExternalID != "00112233-4455-6677-8899-aabbccddeeff" || !e.Disabled {
		t.Fatalf("AD parse: %+v %v", e, err)
	}
	expired := ldap.NewEntry("cn=Alice,ou=people,dc=example,dc=test", map[string][]string{
		"objectGUID":     {string([]byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})},
		"sAMAccountName": {"alice"}, "mail": {"alice@example.test"}, "displayName": {"Alice"},
		"userAccountControl": {"512"}, "accountExpires": {"116444736000000000"},
	})
	e, err = parseEntry(ad, expired)
	if err != nil || !e.Disabled {
		t.Fatalf("expired AD account accepted: %+v %v", e, err)
	}
	missingExpiry := ldap.NewEntry("cn=Alice,ou=people,dc=example,dc=test", map[string][]string{
		"objectGUID":     {string([]byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})},
		"sAMAccountName": {"alice"}, "mail": {"alice@example.test"}, "displayName": {"Alice"}, "userAccountControl": {"512"},
	})
	if _, err := parseEntry(ad, missingExpiry); !errors.Is(err, directory.ErrIncompleteScan) {
		t.Fatal("missing AD expiration state accepted", err)
	}
	missing := ldap.NewEntry("cn=Alice,ou=people,dc=example,dc=test", map[string][]string{
		"sAMAccountName": {"alice"}, "mail": {"alice@example.test"}, "displayName": {"Alice"}, "userAccountControl": {"512"},
	})
	if _, err := parseEntry(ad, missing); !errors.Is(err, directory.ErrIncompleteScan) {
		t.Fatal("missing ID accepted", err)
	}
}

type emailConnectionProbe struct {
	results     []*ldap.SearchResult
	requests    []*ldap.SearchRequest
	binds       []string
	userBindErr error
}

func (p *emailConnectionProbe) Bind(dn, password string) error {
	p.binds = append(p.binds, dn)
	if len(p.binds) > 1 {
		return p.userBindErr
	}
	return nil
}
func (p *emailConnectionProbe) Search(request *ldap.SearchRequest) (*ldap.SearchResult, error) {
	p.requests = append(p.requests, request)
	if len(p.requests) > len(p.results) {
		return nil, errors.New("unexpected search")
	}
	return p.results[len(p.requests)-1], nil
}

func TestEmailAuthenticationKeepsLoginIdentityAndChecksMembership(t *testing.T) {
	cfg := directory.Config{Mode: directory.ModeGeneric, UserFilter: "(objectClass=person)", LoginAttribute: "uid", EmailAttribute: "mail", IDAttribute: "entryUUID", NameAttribute: "cn", UsersDN: "ou=people,dc=example,dc=test", GroupMembershipAttribute: "memberOf", GroupDN: "cn=allowed,ou=groups,dc=example,dc=test", BindDN: "cn=service,dc=example,dc=test"}
	raw := ldap.NewEntry("uid=employee,ou=people,dc=example,dc=test", map[string][]string{
		"entryUUID": {"123e4567-e89b-12d3-a456-426614174000"}, "uid": {"employee"}, "mail": {"employee@example.test"}, "cn": {"Employee"},
	})
	other := ldap.NewEntry("uid=other,ou=people,dc=example,dc=test", nil)
	for _, test := range []struct {
		name                   string
		first, allowed         []*ldap.Entry
		bindErr, errorExpected error
		searches, binds        int
	}{
		{"allowed", []*ldap.Entry{raw}, []*ldap.Entry{raw}, nil, nil, 2, 2},
		{"missing", nil, nil, nil, directory.ErrInvalidCredentials, 1, 1},
		{"duplicate email", []*ldap.Entry{raw, other}, nil, nil, directory.ErrIncompleteScan, 1, 1},
		{"outside group", []*ldap.Entry{raw}, nil, nil, directory.ErrNotAllowed, 2, 1},
		{"membership changed", []*ldap.Entry{raw}, []*ldap.Entry{other}, nil, directory.ErrNotAllowed, 2, 1},
		{"wrong password", []*ldap.Entry{raw}, []*ldap.Entry{raw}, ldap.NewError(ldap.LDAPResultInvalidCredentials, errors.New("invalid")), directory.ErrInvalidCredentials, 2, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn := &emailConnectionProbe{results: []*ldap.SearchResult{{Entries: test.first}, {Entries: test.allowed}}, userBindErr: test.bindErr}
			entry, err := authenticateConnection(conn, cfg, cfg.EmailAttribute, "employee@example.test", "password")
			if !errors.Is(err, test.errorExpected) || len(conn.requests) != test.searches || len(conn.binds) != test.binds {
				t.Fatalf("err=%v searches=%d binds=%d", err, len(conn.requests), len(conn.binds))
			}
			if test.errorExpected == nil && (entry.Login != "employee" || entry.Email != "employee@example.test" || entry.ExternalID != "123e4567-e89b-12d3-a456-426614174000") {
				t.Fatal("email lookup rewrote the stable directory identity")
			}
			for i, request := range conn.requests {
				if request.SizeLimit != 2 || !strings.Contains(request.Filter, "(mail=employee@example.test)") || strings.Contains(request.Filter, "(uid=") {
					t.Fatalf("wrong email lookup: %s", request.Filter)
				}
				if i == 1 && !strings.Contains(request.Filter, "(memberOf=cn=allowed,ou=groups,dc=example,dc=test)") {
					t.Fatal("email lookup skipped configured group restriction")
				}
			}
		})
	}
}

func TestEmailFilterEscapesMetacharacters(t *testing.T) {
	cfg := directory.Config{UserFilter: "(objectClass=person)", EmailAttribute: "mail", LoginAttribute: "uid"}
	filter := identityFilter(cfg, cfg.EmailAttribute, "person*)(mail=*)@example.test", false)
	if strings.Contains(filter, "(mail=*)") || !strings.Contains(filter, `\2a`) {
		t.Fatalf("email input escaped incorrectly: %s", filter)
	}
	if _, err := ldap.CompileFilter(filter); err != nil {
		t.Fatal(err)
	}
}

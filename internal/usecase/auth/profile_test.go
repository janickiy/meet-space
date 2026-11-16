package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	settings "github.com/janickiy/meet-space/internal/domain/settings"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type profileUsers struct {
	auditUsers
	user    users.User
	calls   int
	version int64
	patch   users.UpdateProfileRequest
}

func (r *profileUsers) GetByID(context.Context, string) (users.User, error) { return r.user, nil }
func (r *profileUsers) UpdateProfile(_ context.Context, _ string, p users.UpdateProfileRequest, version int64) (users.User, error) {
	r.calls++
	r.version = version
	r.patch = p
	if p.DisplayName != nil {
		r.user.DisplayName = p.DisplayName
	}
	if p.Bio != nil {
		r.user.Bio = *p.Bio
	}
	return r.user, nil
}

type profilePolicy bool

func (p profilePolicy) Snapshot(context.Context) (settings.Values, error) {
	return settings.Values{LDAPEnabled: bool(p)}, nil
}
func TestProfileBioLDAPAndGuestScope(t *testing.T) {
	bio, name := "О себе", "Новое имя"
	guest := "00000000-0000-4000-8000-000000000001"
	for _, tc := range []struct {
		name, source      string
		guest             *string
		blocked, ldapMode bool
		patch             users.UpdateProfileRequest
		want              error
	}{
		{"local-bio", "local", nil, false, false, users.UpdateProfileRequest{Bio: &bio}, nil},
		{"local-both", "local", nil, false, false, users.UpdateProfileRequest{DisplayName: &name, Bio: &bio}, nil},
		{"ldap-bio", "ldap", nil, false, true, users.UpdateProfileRequest{Bio: &bio}, nil},
		{"ldap-name", "ldap", nil, false, true, users.UpdateProfileRequest{DisplayName: &name, Bio: &bio}, apperrors.ErrForbidden},
		{"guest", "local", &guest, false, false, users.UpdateProfileRequest{Bio: &bio}, apperrors.ErrForbidden},
		{"blocked", "local", nil, true, false, users.UpdateProfileRequest{Bio: &bio}, apperrors.ErrAccountBlocked},
		{"wrong-mode", "local", nil, false, true, users.UpdateProfileRequest{Bio: &bio}, apperrors.ErrUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &profileUsers{user: users.User{ID: guest, AuthSource: tc.source, GuestConferenceID: tc.guest, IsBlocked: tc.blocked, AuthVersion: 9}}
			svc, err := NewService(repo, &blockingPasswords{}, auditTokens{})
			if err != nil {
				t.Fatal(err)
			}
			svc.WithAuthenticationPolicy(profilePolicy(tc.ldapMode))
			view, err := svc.UpdateProfile(context.Background(), guest, tc.patch)
			if !errors.Is(err, tc.want) {
				t.Fatalf("unexpected profile error: %v", err)
			}
			if tc.want != nil {
				if repo.calls != 0 {
					t.Fatal("rejected profile persisted")
				}
				return
			}
			if repo.calls != 1 || repo.version != 9 || view.Bio == nil || *view.Bio != bio {
				t.Fatal("partial patch or security version lost")
			}
		})
	}
}

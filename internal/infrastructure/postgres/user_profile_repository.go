package postgres

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpdateProfile locks policy before the account, matching authentication and
// administrative mode changes. Partial patches never overwrite omitted fields.
func (r *UserRepository) UpdateProfile(ctx context.Context, id string, input users.UpdateProfileRequest, expectedAuthVersion int64) (users.User, error) {
	patch, err := input.Normalized()
	if err != nil {
		return users.User{}, err
	}
	var user users.User
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ldapOnly, err := lockAuthenticationMode(tx)
		if err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NULL", id).Take(&user).Error; err != nil {
			return mapNotFound(err)
		}
		if user.GuestConferenceID != nil {
			return apperrors.ErrForbidden
		}
		if user.AccessBlocked() {
			return apperrors.ErrAccountBlocked
		}
		if user.IsLDAP() != ldapOnly || user.AuthVersion != expectedAuthVersion {
			return apperrors.ErrUnauthorized
		}
		if user.IsLDAP() && patch.DisplayName != nil {
			return apperrors.New(apperrors.ErrForbidden, "Корпоративное имя управляется в LDAP / Active Directory.")
		}
		changes := map[string]any{}
		if patch.DisplayName != nil {
			changes["display_name"] = *patch.DisplayName
		}
		if patch.Bio != nil {
			changes["bio"] = *patch.Bio
		}
		// Table/map deliberately persists the profile's explicit fields; Bio is
		// read-only on the general User model to preserve pre-migration fixtures.
		// The users updated_at trigger timestamps this write within the transaction.
		if err := tx.Table("users").Where("id=?", id).Updates(changes).Error; err != nil {
			return err
		}
		return tx.Where("id=?", id).Take(&user).Error
	})
	return user, err
}

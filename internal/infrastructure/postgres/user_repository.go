package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// UserRepository persists account identity and credential metadata through GORM.
type UserRepository struct{ db *gorm.DB }

// NewUserRepository constructs the user repository with the dependencies and limits required for
// user.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded user repository.
func NewUserRepository(db *gorm.DB) *UserRepository {
	// An INSERT error must not cause GORM to print password_hash parameters.
	return &UserRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

// Create persists the validated account identity and protected password hash.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Validated account record whose public identity or server session is being processed.
//
// Returns:
//   - result 1: The resulting user.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *UserRepository) Create(ctx context.Context, user users.User) (users.User, error) {
	err := r.db.WithContext(ctx).Create(&user).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
		return users.User{}, apperrors.New(apperrors.ErrConflict, "email is already registered")
	}
	return user, err
}

// GetByID loads the user by ID.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting user.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *UserRepository) GetByID(ctx context.Context, id string) (users.User, error) {
	var user users.User
	err := r.db.WithContext(ctx).Where("id = ? AND password_hash<>''", id).Take(&user).Error
	return user, mapNotFound(err)
}

// GetByEmail loads the user by email.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - email: Account or delivery email address.
//
// Returns:
//   - result 1: The resulting user.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (users.User, error) {
	var user users.User
	err := r.db.WithContext(ctx).Where("email = ? AND password_hash<>''", email).Take(&user).Error
	return user, mapNotFound(err)
}

// UpdateDisplayName updates only the public name of the account resolved from the authenticated
// subject.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - userID: Acting account identifier used to scope access.
//   - name: Name of the setting, cookie, resource or provider being addressed.
//
// Returns:
//   - result 1: The resulting user.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *UserRepository) UpdateDisplayName(ctx context.Context, userID, name string) (users.User, error) {
	result := r.db.WithContext(ctx).Model(&users.User{}).Where("id = ? AND password_hash<>''", userID).Update("display_name", name)
	if result.Error != nil {
		return users.User{}, result.Error
	}
	if result.RowsAffected == 0 {
		return users.User{}, apperrors.ErrNotFound
	}
	return r.GetByID(ctx, userID)
}

// mapNotFound converts GORM's missing-row error to the application not-found error.
//
// Parameters:
//   - err: Error to classify, wrap or report safely.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.ErrNotFound
	}
	return err
}

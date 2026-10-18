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

// UserRepository сохраняет идентификационные данные учетной записи и метаданные учетных данных
// через GORM.
type UserRepository struct{ db *gorm.DB }

// NewUserRepository создает репозиторий пользователей с зависимостями и ограничениями,
// необходимыми для работы с пользователями.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий пользователей.
func NewUserRepository(db *gorm.DB) *UserRepository {
	// Ошибка INSERT не должна приводить к тому, что GORM выводит параметры password_hash.
	return &UserRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

// Create сохраняет подтвержденные данные учетной записи и защищенный хэш пароля.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Подтвержденная запись учетной записи, для которой обрабатывается публичная
//     идентификация или сеанс на сервере.
//
// @returns:
//   - result1: Полученный пользователь.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *UserRepository) Create(ctx context.Context, user users.User) (users.User, error) {
	err := r.db.WithContext(ctx).Create(&user).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
		return users.User{}, apperrors.New(apperrors.ErrConflict, "email is already registered")
	}
	return user, err
}

// GetByID загружает пользователя с помощью ID.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученный пользователь.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *UserRepository) GetByID(ctx context.Context, id string) (users.User, error) {
	var user users.User
	err := r.db.WithContext(ctx).Where("id = ? AND password_hash<>''", id).Take(&user).Error
	return user, mapNotFound(err)
}

// GetByEmail загружает пользователя по электронной почте.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - email: Адрес электронной почты для входа в аккаунт или доставки.
//
// @returns:
//   - result1: Полученный пользователь.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (users.User, error) {
	var user users.User
	err := r.db.WithContext(ctx).Where("email = ? AND password_hash<>''", email).Take(&user).Error
	return user, mapNotFound(err)
}

// UpdateDisplayName обновляет только публичное имя учетной записи, полученное из субъекта
// аутентификации.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - name: Имя параметра, файла cookie, ресурса или провайдера, к которому осуществляется
//     обращение.
//
// @returns:
//   - result1: Полученный пользователь.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// mapNotFound преобразует ошибку GORM, связанную с отсутствующей строкой, в ошибку «приложение
// не найдено».
//
// @Parameters:
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.ErrNotFound
	}
	return err
}

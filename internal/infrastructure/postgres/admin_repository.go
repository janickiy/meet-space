package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// AdminRepository shares the existing database but never user authentication state.
type AdminRepository struct{ db *gorm.DB }

func NewAdminRepository(db *gorm.DB) *AdminRepository {
	return &AdminRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}
func (r *AdminRepository) ByUsername(ctx context.Context, u string) (admin.Administrator, error) {
	var a admin.Administrator
	err := r.db.WithContext(ctx).Where("username = ?", u).Take(&a).Error
	return a, mapNotFound(err)
}
func (r *AdminRepository) ByID(ctx context.Context, id string) (admin.Administrator, error) {
	var a admin.Administrator
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&a).Error
	return a, mapNotFound(err)
}
func (r *AdminRepository) Status(ctx context.Context) (*admin.Administrator, error) {
	var a admin.Administrator
	err := r.db.WithContext(ctx).Order("CASE WHEN status='active' THEN 0 ELSE 1 END, created_at, id").Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &a, err
}
func adminLock(tx *gorm.DB) error { return tx.Exec("SELECT pg_advisory_xact_lock(748239106)").Error }

// adminMutationGuard rechecks the authenticated HTTP capability inside the write
// transaction. A request waiting for its body, password work, or a database lock
// cannot commit after logout, account blocking, or password reset has committed.
// Lock order is administrator -> session -> mutation target. Administrator CRUD
// takes the existing advisory lock first; unrelated record/user writes need no
// global lock. Trusted server-only CLI calls have no HTTP authorization context.
func adminMutationGuard(tx *gorm.DB) error {
	auth, present := admin.AuthorizationFromContext(tx.Statement.Context)
	if !present {
		return nil
	}
	var actor admin.Administrator
	err := tx.Select("id,status").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=?", auth.AdministratorID).Take(&actor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.ErrUnauthorized
	}
	if err != nil {
		return err
	}
	if actor.Status != "active" {
		return apperrors.ErrUnauthorized
	}
	var session admin.Session
	err = tx.Select("id").Clauses(clause.Locking{Strength: "SHARE"}).Where("administrator_id=? AND token_hash=? AND expires_at>clock_timestamp()", actor.ID, auth.SessionHash).Take(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.ErrUnauthorized
	}
	return err
}

func adminError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" {
		return apperrors.New(apperrors.ErrConflict, "Этот логин уже используется")
	}
	return mapNotFound(err)
}
func auditInsert(tx *gorm.DB, a admin.AuditInput) error {
	// All caller-supplied metadata is allowlisted. Passwords, cookies and tokens have no audit representation.
	safe := map[string]any{}
	for _, key := range []string{"status", "fields", "source", "recordingId", "conferenceId", "httpStatus"} {
		if v, ok := a.Metadata[key]; ok {
			safe[key] = v
		}
	}
	metadata, err := json.Marshal(safe)
	if err != nil {
		return err
	}
	if len(metadata) > 4096 {
		metadata = []byte(`{}`)
	}
	var actor *string
	if a.AdministratorID != "" {
		actor = &a.AdministratorID
	}
	event := admin.AuditEvent{ID: uuid.NewString(), AdministratorID: actor, Username: truncateAdmin(a.Username, 40), Action: truncateAdmin(a.Action, 80), ObjectType: truncateAdmin(a.ObjectType, 60), ObjectID: truncateAdmin(a.ObjectID, 100), IP: truncateAdmin(a.IP, 64), UserAgent: truncateAdmin(a.UserAgent, 500), Metadata: metadata, CreatedAt: time.Now().UTC()}
	return tx.Create(&event).Error
}
func truncateAdmin(v string, n int) string {
	r := []rune(v)
	if len(r) > n {
		return string(r[:n])
	}
	return v
}
func (r *AdminRepository) Audit(ctx context.Context, a admin.AuditInput) error {
	return auditInsert(r.db.WithContext(ctx), a)
}
func (r *AdminRepository) Bootstrap(ctx context.Context, a admin.Administrator) (admin.Administrator, error) {
	var out admin.Administrator
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminLock(tx); err != nil {
			return err
		}
		err := tx.Order("created_at,id").Take(&out).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(&a).Error; err != nil {
			return adminError(err)
		}
		out = a
		return auditInsert(tx, admin.AuditInput{AdministratorID: a.ID, Username: a.Username, Action: "administrator.bootstrap", ObjectType: "administrator", ObjectID: a.ID, Metadata: map[string]any{"source": "server-cli"}})
	})
	return out, err
}
func (r *AdminRepository) Create(ctx context.Context, a admin.Administrator, audit admin.AuditInput) (admin.Administrator, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminLock(tx); err != nil {
			return err
		}
		if err := adminMutationGuard(tx); err != nil {
			return err
		}
		if err := tx.Create(&a).Error; err != nil {
			return adminError(err)
		}
		audit.Action = "administrator.create"
		audit.ObjectType = "administrator"
		audit.ObjectID = a.ID
		return auditInsert(tx, audit)
	})
	return a, err
}
func activeAdminGuard(tx *gorm.DB, target admin.Administrator) error {
	if target.Status != "active" {
		return nil
	}
	var n int64
	if err := tx.Model(&admin.Administrator{}).Where("status='active'").Count(&n).Error; err != nil {
		return err
	}
	if n <= 1 {
		return apperrors.New(apperrors.ErrConflict, "Нельзя заблокировать или удалить последнего активного администратора")
	}
	return nil
}
func (r *AdminRepository) Update(ctx context.Context, id string, input admin.UpdateRequest, audit admin.AuditInput) (admin.Administrator, error) {
	var a admin.Administrator
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminLock(tx); err != nil {
			return err
		}
		if err := adminMutationGuard(tx); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", id).Take(&a).Error; err != nil {
			return mapNotFound(err)
		}
		changes := map[string]any{}
		fields := []string{}
		if input.Username != nil {
			changes["username"] = *input.Username
			fields = append(fields, "username")
		}
		if input.Name != nil {
			changes["name"] = *input.Name
			fields = append(fields, "name")
		}
		if input.Email != nil {
			if *input.Email == "" {
				changes["email"] = nil
			} else {
				changes["email"] = *input.Email
			}
			fields = append(fields, "email")
		}
		audit.Action = "administrator.update"
		if input.Status != nil && *input.Status != a.Status {
			if *input.Status == "blocked" {
				if audit.AdministratorID == a.ID {
					return apperrors.New(apperrors.ErrConflict, "Нельзя заблокировать собственную учётную запись")
				}
				if err := activeAdminGuard(tx, a); err != nil {
					return err
				}
				audit.Action = "administrator.block"
			} else {
				audit.Action = "administrator.unblock"
			}
			changes["status"] = *input.Status
			fields = append(fields, "status")
			if err := tx.Where("administrator_id=?", id).Delete(&admin.Session{}).Error; err != nil {
				return err
			}
		}
		if len(changes) > 0 {
			if err := tx.Model(&a).Updates(changes).Error; err != nil {
				return adminError(err)
			}
		}
		audit.ObjectType = "administrator"
		audit.ObjectID = id
		audit.Metadata = map[string]any{"fields": fields}
		if err := auditInsert(tx, audit); err != nil {
			return err
		}
		return tx.Where("id=?", id).Take(&a).Error
	})
	return a, err
}
func (r *AdminRepository) Delete(ctx context.Context, id string, audit admin.AuditInput) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminLock(tx); err != nil {
			return err
		}
		if err := adminMutationGuard(tx); err != nil {
			return err
		}
		var a admin.Administrator
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", id).Take(&a).Error; err != nil {
			return mapNotFound(err)
		}
		if audit.AdministratorID == a.ID {
			return apperrors.New(apperrors.ErrConflict, "Нельзя удалить собственную учётную запись")
		}
		if err := activeAdminGuard(tx, a); err != nil {
			return err
		}
		if err := tx.Delete(&a).Error; err != nil {
			return err
		}
		audit.Action = "administrator.delete"
		audit.ObjectType = "administrator"
		audit.ObjectID = id
		return auditInsert(tx, audit)
	})
}

// expectedHash binds a self-service password change to the password verified before hashing.
func (r *AdminRepository) ChangePassword(ctx context.Context, id, hash, expectedHash string, audit admin.AuditInput) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminLock(tx); err != nil {
			return err
		}
		if err := adminMutationGuard(tx); err != nil {
			return err
		}
		var a admin.Administrator
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", id).Take(&a).Error; err != nil {
			return mapNotFound(err)
		}
		if audit.AdministratorID == a.ID && expectedHash == "" && audit.Action != "administrator.recover" {
			return apperrors.ErrUnauthorized
		}
		if expectedHash != "" && a.PasswordHash != expectedHash {
			return apperrors.ErrUnauthorized
		}
		changes := map[string]any{"password_hash": hash}
		if audit.Action == "administrator.recover" {
			changes["status"] = "active"
		}
		if err := tx.Model(&a).Updates(changes).Error; err != nil {
			return err
		}
		if err := tx.Where("administrator_id=?", id).Delete(&admin.Session{}).Error; err != nil {
			return err
		}
		if audit.Action == "" {
			audit.Action = "administrator.password"
		}
		audit.ObjectType = "administrator"
		audit.ObjectID = id
		return auditInsert(tx, audit)
	})
}
func (r *AdminRepository) CreateSession(ctx context.Context, s admin.Session, expectedHash string, audit admin.AuditInput) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var a admin.Administrator
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", s.AdministratorID).Take(&a).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperrors.ErrUnauthorized
			}
			return err
		}
		if a.Status != "active" || a.PasswordHash != expectedHash {
			return apperrors.ErrUnauthorized
		}
		// Opportunistic cleanup must not wait for another administrator's active
		// transaction, especially while this login holds its own account lock.
		if err := tx.Exec(`DELETE FROM administrator_sessions WHERE id IN (SELECT id FROM administrator_sessions WHERE expires_at<=now() ORDER BY expires_at LIMIT 500 FOR UPDATE SKIP LOCKED)`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM administrator_sessions WHERE administrator_id=? AND id NOT IN (SELECT id FROM administrator_sessions WHERE administrator_id=? ORDER BY created_at DESC LIMIT 19)`, a.ID, a.ID).Error; err != nil {
			return err
		}
		if err := tx.Create(&s).Error; err != nil {
			return err
		}
		if err := tx.Model(&a).Update("last_login_at", time.Now().UTC()).Error; err != nil {
			return err
		}
		audit.AdministratorID = a.ID
		audit.Username = a.Username
		audit.Action = "auth.login"
		audit.ObjectType = "administrator"
		audit.ObjectID = a.ID
		return auditInsert(tx, audit)
	})
}
func (r *AdminRepository) Authenticate(ctx context.Context, hash string) (admin.Administrator, error) {
	var a admin.Administrator
	err := r.db.WithContext(ctx).Model(&admin.Administrator{}).Select("administrators.*").Joins("JOIN administrator_sessions s ON s.administrator_id=administrators.id").Where("s.token_hash=? AND s.expires_at>now() AND administrators.status='active'", hash).Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return a, apperrors.ErrUnauthorized
	}
	return a, err
}
func (r *AdminRepository) RevokeSession(ctx context.Context, hash string, audit admin.AuditInput) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize session deletion with administrative account mutations and
		// duplicate logouts before acquiring the shared authorization locks.
		if err := adminLock(tx); err != nil {
			return err
		}
		if err := adminMutationGuard(tx); err != nil {
			return err
		}
		if err := tx.Where("token_hash=?", hash).Delete(&admin.Session{}).Error; err != nil {
			return err
		}
		audit.Action = "auth.logout"
		audit.ObjectType = "administrator"
		audit.ObjectID = audit.AdministratorID
		return auditInsert(tx, audit)
	})
}
func (r *AdminRepository) LoginAllowed(ctx context.Context, ip, username string) (bool, error) {
	allowed := true
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM administrator_login_limits WHERE key_hash IN (SELECT key_hash FROM administrator_login_limits WHERE window_start<now()-interval '1 day' ORDER BY window_start LIMIT 500 FOR UPDATE SKIP LOCKED)").Error; err != nil {
			return err
		}
		for _, scope := range []struct {
			key string
			max int
		}{{"ip:" + ip, 30}, {"username:" + username, 10}} {
			h := sha256.Sum256([]byte(scope.key))
			var n int
			err := tx.Raw(`INSERT INTO administrator_login_limits(key_hash,window_start,attempts) VALUES (?,now(),1)
   ON CONFLICT(key_hash) DO UPDATE SET attempts=CASE WHEN administrator_login_limits.window_start<now()-interval '15 minutes' THEN 1 ELSE administrator_login_limits.attempts+1 END,window_start=CASE WHEN administrator_login_limits.window_start<now()-interval '15 minutes' THEN now() ELSE administrator_login_limits.window_start END RETURNING attempts`, hex.EncodeToString(h[:])).Scan(&n).Error
			if err != nil {
				return err
			}
			if n > scope.max {
				allowed = false
				return nil // Do not create unbounded username buckets once this IP is throttled.
			}
		}
		return nil
	})
	return allowed, err
}
func adminOrder(q admin.Query, allowed map[string]string, def string) string {
	column := allowed[q.Sort]
	if column == "" {
		column = def
	}
	return column + " " + q.Normalized().Order + ", id " + q.Normalized().Order
}
func (r *AdminRepository) List(ctx context.Context, q admin.Query) (admin.Page[admin.Administrator], error) {
	q = q.Normalized()
	out := admin.NewPage[admin.Administrator](q)
	db := r.db.WithContext(ctx).Model(&admin.Administrator{})
	if q.Q != "" {
		like := "%" + escapeAdminLike(q.Q) + "%"
		db = db.Where("username ILIKE ? OR name ILIKE ?", like, like)
	}
	if q.Status != "" && q.Status != "all" {
		db = db.Where("status=?", q.Status)
	}
	if err := db.Count(&out.Total).Error; err != nil {
		return out, err
	}
	err := db.Order(adminOrder(q, map[string]string{"username": "username", "name": "name", "createdAt": "created_at", "lastLoginAt": "last_login_at", "status": "status"}, "created_at")).Offset(q.Offset()).Limit(q.PageSize).Find(&out.Items).Error
	return out, err
}
func escapeAdminLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}
func (r *AdminRepository) Users(ctx context.Context, q admin.Query) (admin.Page[admin.UserView], error) {
	q = q.Normalized()
	out := admin.NewPage[admin.UserView](q)
	db := r.db.WithContext(ctx).Table("users").Where("deleted_at IS NULL AND guest_conference_id IS NULL")
	if q.Q != "" {
		like := "%" + escapeAdminLike(q.Q) + "%"
		predicate := "(display_name ILIKE ? OR email ILIKE ?)"
		args := []any{like, like}
		if id, err := uuid.Parse(q.Q); err == nil {
			// Keep the UUID comparison typed: id::text forced a full scan even
			// when all other branches could use their trigram index.
			predicate = "(" + predicate + " OR id=?)"
			args = append(args, id)
		}
		db = db.Where(predicate, args...)
	}
	if q.Status == "active" || q.Status == "blocked" {
		db = db.Where("(is_blocked OR ldap_disabled)=?", q.Status == "blocked")
	}
	if err := db.Count(&out.Total).Error; err != nil {
		return out, err
	}
	err := db.Select("id,COALESCE(display_name,'') AS name,NULLIF(email,'') AS email,CASE WHEN is_blocked OR ldap_disabled THEN 'blocked' ELSE 'active' END AS status,created_at").Order(adminOrder(q, map[string]string{"name": "display_name", "email": "email", "createdAt": "created_at", "status": "(is_blocked OR ldap_disabled)"}, "created_at")).Offset(q.Offset()).Limit(q.PageSize).Scan(&out.Items).Error
	return out, err
}
func (r *AdminRepository) SetUserStatus(ctx context.Context, id, status string, audit admin.AuditInput) (admin.UserView, error) {
	var out admin.UserView
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminMutationGuard(tx); err != nil {
			return err
		}
		var exists string
		result := tx.Raw("SELECT id FROM users WHERE id=? AND deleted_at IS NULL AND guest_conference_id IS NULL FOR UPDATE", id).Scan(&exists)
		if result.Error != nil {
			return result.Error
		}
		if exists == "" {
			return apperrors.ErrNotFound
		}
		if status == "active" {
			var directoryDisabled bool
			if err := tx.Raw("SELECT ldap_disabled FROM users WHERE id=?", id).Scan(&directoryDisabled).Error; err != nil {
				return err
			}
			if directoryDisabled {
				return apperrors.New(apperrors.ErrConflict, "Доступ пользователя отключён в LDAP. Восстановите его в каталоге и выполните синхронизацию.")
			}
		}
		updates := map[string]any{"is_blocked": status == "blocked"}
		if status == "blocked" {
			updates["auth_invalid_before"] = gorm.Expr("clock_timestamp()")
			updates["auth_version"] = gorm.Expr("auth_version+1")
		}
		if err := tx.Table("users").Where("id=?", id).Updates(updates).Error; err != nil {
			return err
		}
		if status == "blocked" {
			if err := tx.Exec("UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=?", id).Error; err != nil {
				return err
			}
			if err := tx.Exec("UPDATE auth_legacy_tokens SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=?", id).Error; err != nil {
				return err
			}
		}
		audit.Action = "user." + map[bool]string{true: "block", false: "unblock"}[status == "blocked"]
		audit.ObjectType = "user"
		audit.ObjectID = id
		audit.Metadata = map[string]any{"status": status}
		if err := auditInsert(tx, audit); err != nil {
			return err
		}
		return tx.Table("users").Select("id,COALESCE(display_name,'') AS name,NULLIF(email,'') AS email,CASE WHEN is_blocked OR ldap_disabled THEN 'blocked' ELSE 'active' END AS status,created_at").Where("id=?", id).Scan(&out).Error
	})
	return out, err
}
func (r *AdminRepository) AuditLog(ctx context.Context, q admin.Query) (admin.Page[admin.AuditEvent], error) {
	q = q.Normalized()
	out := admin.NewPage[admin.AuditEvent](q)
	db := r.db.WithContext(ctx).Model(&admin.AuditEvent{})
	if q.Q != "" {
		like := "%" + escapeAdminLike(q.Q) + "%"
		db = db.Where("username ILIKE ? OR action ILIKE ? OR object_id ILIKE ? OR ip ILIKE ?", like, like, like, like)
	}
	if q.Action != "" {
		db = db.Where("action=?", q.Action)
	}
	if q.ObjectType != "" {
		db = db.Where("object_type=?", q.ObjectType)
	}
	if err := db.Count(&out.Total).Error; err != nil {
		return out, err
	}
	err := db.Order(adminOrder(q, map[string]string{"createdAt": "created_at", "action": "action", "username": "username"}, "created_at")).Offset(q.Offset()).Limit(q.PageSize).Find(&out.Items).Error
	return out, err
}

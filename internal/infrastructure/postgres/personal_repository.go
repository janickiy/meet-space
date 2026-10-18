package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"
	"time"
	"unicode/utf8"
)

type PersonalRepository struct{ db *gorm.DB }

// NewPersonalRepository constructs the personal repository with the dependencies and limits
// required for personal and group conversations.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded personal repository.
func NewPersonalRepository(db *gorm.DB) *PersonalRepository {
	return &PersonalRepository{db: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})}
}

// Account checks that the acting identity is a registered non-guest account.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) Account(ctx context.Context, user string) error {
	var n int64
	err := r.db.WithContext(ctx).Table("users").Where("id=? AND guest_conference_id IS NULL AND password_hash<>''", user).Count(&n).Error
	if err != nil {
		return err
	}
	if n != 1 {
		return apperrors.ErrForbidden
	}
	return nil
}

// GetOrCreate returns the existing direct conversation or creates its unique account-pair
// conversation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - target: Target state, account or decoded value, as defined by its type.
//
// Returns:
//   - result 1: The resulting conversation.
//   - result 2: Whether this request created new state rather than reusing an idempotent result.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) GetOrCreate(ctx context.Context, user, target string) (personal.Conversation, bool, error) {
	if user == target {
		return personal.Conversation{}, false, apperrors.New(apperrors.ErrInvalidInput, "self messaging is unavailable")
	}
	lo, hi := user, target
	if lo > hi {
		lo, hi = hi, lo
	}
	id := uuid.NewString()
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Table("users").Where("id IN ? AND guest_conference_id IS NULL AND password_hash<>''", []string{lo, hi}).Count(&n).Error; err != nil {
			return err
		}
		if n != 2 {
			return apperrors.ErrNotFound
		}
		result := tx.Exec("INSERT INTO conversations(id,user_low_id,user_high_id) VALUES(?,?,?) ON CONFLICT(user_low_id,user_high_id) DO NOTHING", id, lo, hi)
		if result.Error != nil {
			return result.Error
		}
		created = result.RowsAffected == 1
		if !created {
			var row struct{ ID string }
			if err := tx.Table("conversations").Select("id").Where("user_low_id=? AND user_high_id=?", lo, hi).Take(&row).Error; err != nil {
				return err
			}
			id = row.ID
			if err := authorizeDirect(tx, user, id, true); err != nil {
				return err
			}
			return tx.Table("conversation_members").Where("conversation_id=? AND user_id=? AND hidden_at IS NOT NULL", id, user).
				Updates(map[string]any{"hidden_at": nil, "updated_at": gorm.Expr("clock_timestamp()")}).Error
		}
		return tx.Exec("INSERT INTO conversation_members(conversation_id,user_id) VALUES(?,?),(?,?)", id, lo, id, hi).Error
	})
	if err != nil {
		return personal.Conversation{}, false, err
	}
	item, err := r.Get(ctx, user, id)
	return item, created, err
}

// A bounded page uses one projection (including sender/member count/unread), not per-row calls.
const personalUnreadPredicate = `x.conversation_id=c.id AND x.sequence>GREATEST(m.last_read_sequence,m.history_cleared_through) AND x.sender_user_id<>m.user_id AND x.deleted_at IS NULL`
const personalUnreadSQL = `(SELECT count(*) FROM conversation_messages x WHERE ` + personalUnreadPredicate + `)`
const personalActivitySQL = `COALESCE(lm.created_at,c.created_at)`
const personalProjection = `SELECT c.id,c.type,c.created_at,c.updated_at,lm.created_at AS last_message_at,lm.id AS last_message_id,
 ` + personalActivitySQL + ` AS activity_at,u.id AS peer_id,
 COALESCE(NULLIF(u.display_name,''),'Пользователь') AS peer_name,
 COALESCE(c.name,'') AS name,c.description,c.created_by,c.avatar_version,m.role AS my_role,m.notifications_enabled,m.history_cleared_through,
 CASE WHEN c.type='group' THEN (SELECT count(*) FROM conversation_members gm WHERE gm.conversation_id=c.id AND gm.left_at IS NULL) ELSE 0 END AS member_count,
 lm.sender_user_id AS last_sender_id,COALESCE(NULLIF(su.display_name,''),'Пользователь') AS last_sender_name,
 CASE WHEN lm.deleted_at IS NOT NULL THEN 'Сообщение удалено'
 WHEN lm.kind='call' THEN CASE WHEN lm.sender_user_id=m.user_id THEN 'Исходящий звонок' ELSE 'Входящий звонок' END
 ELSE COALESCE(NULLIF(left(lm.text,160),''),CASE WHEN lm.id IS NULL THEN '' ELSE 'Вложение' END) END AS preview,
 ` + personalUnreadSQL + ` AS unread_count
 FROM conversation_members m JOIN conversations c ON c.id=m.conversation_id
 LEFT JOIN users u ON c.type='direct' AND u.id=CASE WHEN c.user_low_id=m.user_id THEN c.user_high_id ELSE c.user_low_id END
 LEFT JOIN conversation_messages lm ON lm.id=c.last_message_id AND lm.sequence>m.history_cleared_through
 LEFT JOIN users su ON su.id=lm.sender_user_id
 WHERE m.user_id=? AND m.left_at IS NULL AND m.hidden_at IS NULL AND c.deleted_at IS NULL AND c.type IN('direct','group')`

type personalRow struct {
	ID, Type, PeerName, Preview, Name, Description, MyRole, LastSenderName string
	PeerID, CreatedBy, AvatarVersion, LastSenderID                         *string
	CreatedAt, UpdatedAt, ActivityAt                                       time.Time
	LastMessageAt                                                          *time.Time
	LastMessageID                                                          *string
	UnreadCount, MemberCount                                               int64
	NotificationsEnabled                                                   bool
	HistoryClearedThrough                                                  int64
}

// view builds the public, member-specific view of the stored resource.
//
// Returns:
//   - result 1: The resulting conversation.
func (row personalRow) view() personal.Conversation {
	item := personal.Conversation{ID: row.ID, Type: row.Type, Name: row.Name, Description: row.Description,
		CreatedBy: row.CreatedBy, UpdatedAt: row.UpdatedAt, MemberCount: row.MemberCount, AvatarVersion: row.AvatarVersion,
		CreatedAt: row.CreatedAt, ActivityAt: row.ActivityAt, LastMessageAt: row.LastMessageAt, LastMessageID: row.LastMessageID,
		Preview: row.Preview, UnreadCount: row.UnreadCount, NotificationsEnabled: row.NotificationsEnabled,
		HistoryClearedThrough: row.HistoryClearedThrough}
	if row.Type == "direct" && row.PeerID != nil {
		item.Peer = &personal.Peer{ID: *row.PeerID, DisplayName: row.PeerName}
	}
	if row.Type == "group" {
		item.MyRole = personal.Role(row.MyRole)
	}
	if row.LastSenderID != nil {
		item.LastSender = &personal.Peer{ID: *row.LastSenderID, DisplayName: row.LastSenderName}
	}
	return item
}

// Get loads the requested personal within the caller's authorized scope.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting conversation.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) Get(ctx context.Context, user, id string) (personal.Conversation, error) {
	return personalView(r.db.WithContext(ctx), user, id)
}

// personalView builds the member-specific personal-conversation response.
//
// A write response is projected while its conversation lock is still held. A
// subsequent revoke cannot turn a committed mutation into an apparent failure.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//   - user: Acting account identifier used to scope access.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting conversation.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func personalView(db *gorm.DB, user, id string) (personal.Conversation, error) {
	var row personalRow
	result := db.Raw(personalProjection+" AND c.id=?", user, id).Scan(&row)
	if result.Error != nil {
		return personal.Conversation{}, result.Error
	}
	if result.RowsAffected != 1 {
		return personal.Conversation{}, apperrors.ErrForbidden
	}
	return row.view(), nil
}

type personalCursor struct {
	User   string
	At     time.Time
	ID     string
	Filter personal.ListFilter
}

// List returns a bounded page of personal records using the supplied pagination and filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - cursor: Opaque continuation boundary from the previous page.
//   - limit: Maximum number of results or operations in this batch.
//
// Returns:
//   - result 1: The resulting page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) List(ctx context.Context, user, cursor string, limit int) (personal.Page, error) {
	return r.ListFiltered(ctx, user, cursor, limit, personal.ListFilter{})
}

// ListFiltered returns a member-scoped conversation page using the validated personal, group or
// unread filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - cursor: Opaque continuation boundary from the previous page.
//   - limit: Maximum number of results or operations in this batch.
//   - filter: Validated filters restricting the result set.
//
// Returns:
//   - result 1: The resulting page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) ListFiltered(ctx context.Context, user, cursor string, limit int, filter personal.ListFilter) (personal.Page, error) {
	page := personal.Page{Items: []personal.Conversation{}}
	if limit < 1 || limit > 100 {
		return page, apperrors.ErrInvalidInput
	}
	filter, err := personal.NormalizeListFilter(filter)
	if err != nil {
		return page, err
	}
	query := personalProjection
	args := []any{user}
	if filter.Type != "" {
		query += " AND c.type=?"
		args = append(args, filter.Type)
	}
	if filter.UnreadOnly {
		// Filtering needs only the first visible unread message, not a full count
		// for every candidate conversation. Numeric projections stay unchanged.
		query += " AND EXISTS(SELECT 1 FROM conversation_messages x WHERE " + personalUnreadPredicate + ")"
	}
	if filter.Search != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(filter.Search)
		query += " AND (CASE WHEN c.type='group' THEN c.name ELSE COALESCE(NULLIF(u.display_name,''),'Пользователь') END) ILIKE ?"
		args = append(args, "%"+escaped+"%")
	}
	if cursor != "" {
		var c personalCursor
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		if len(cursor) > 1024 || e != nil || json.Unmarshal(raw, &c) != nil || c.User != user || c.At.IsZero() || c.Filter != filter {
			return page, apperrors.ErrInvalidInput
		}
		if _, e = uuid.Parse(c.ID); e != nil {
			return page, apperrors.ErrInvalidInput
		}
		query += " AND (" + personalActivitySQL + ",c.id)<(?,?)"
		args = append(args, c.At, c.ID)
	}
	query += " ORDER BY " + personalActivitySQL + " DESC,c.id DESC LIMIT ?"
	args = append(args, limit+1)
	var rows []personalRow
	if err = r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return page, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		raw, _ := json.Marshal(personalCursor{User: user, At: last.ActivityAt, ID: last.ID, Filter: filter})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	for _, row := range rows {
		page.Items = append(page.Items, row.view())
	}
	// The sidebar badge stays the total across all active direct/group conversations.
	err = r.db.WithContext(ctx).Raw(`SELECT count(*) FROM conversation_members m JOIN conversations c ON c.id=m.conversation_id AND c.deleted_at IS NULL JOIN conversation_messages x ON x.conversation_id=m.conversation_id AND x.sequence>GREATEST(m.last_read_sequence,m.history_cleared_through) AND x.sender_user_id<>m.user_id AND x.deleted_at IS NULL WHERE m.user_id=? AND m.left_at IS NULL AND m.hidden_at IS NULL`, user).Scan(&page.UnreadCount).Error
	return page, err
}

// Members returns the current member identifiers or member views for the authorized
// conversation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The collected string values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) Members(ctx context.Context, id string) ([]string, error) {
	ids := []string{}
	err := r.db.WithContext(ctx).Table("conversation_members m").Joins("JOIN conversations c ON c.id=m.conversation_id").Where("m.conversation_id=? AND m.left_at IS NULL AND c.deleted_at IS NULL", id).Order("m.user_id").Pluck("m.user_id", &ids).Error
	return ids, err
}

// ConversationType returns whether the persisted conversation is direct or group-scoped.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: Persisted direct or group conversation kind.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) ConversationType(ctx context.Context, id string) (string, error) {
	var row struct{ Type string }
	err := r.db.WithContext(ctx).Table("conversations").Select("type").Where("id=?", id).Take(&row).Error
	if err != nil {
		return "", mapNotFound(err)
	}
	return row.Type, nil
}

// HasActiveMembership checks the account's current conversation membership immediately before
// protected access or delivery.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//   - user: Acting account identifier used to scope access.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) HasActiveMembership(ctx context.Context, id, user string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("conversation_members m").Joins("JOIN conversations c ON c.id=m.conversation_id").Joins("JOIN users u ON u.id=m.user_id").Where("m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL AND c.deleted_at IS NULL AND c.type IN('direct','group') AND u.guest_conference_id IS NULL AND u.password_hash<>''", id, user).Count(&count).Error
	return count == 1, err
}

// Search searches personal and group conversations using bounded input and the caller's
// visibility filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - query: Search text or validated query filters.
//
// Returns:
//   - result 1: The collected peer values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *PersonalRepository) Search(ctx context.Context, user, query string) ([]personal.Peer, error) {
	query = strings.TrimSpace(query)
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 100 || strings.ContainsRune(query, 0) {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "search requires 2 to 100 characters")
	}
	query = strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query)
	peers := []personal.Peer{}
	err := r.db.WithContext(ctx).Table("users").Select("id,display_name").Where("id<>? AND guest_conference_id IS NULL AND password_hash<>'' AND display_name ILIKE ?", user, "%"+query+"%").Order("display_name,id").Limit(20).Scan(&peers).Error
	return peers, err
}

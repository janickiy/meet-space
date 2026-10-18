package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/personal"
	assets "github.com/janickiy/meet-space/internal/usecase/personal"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type PersonalAssetRepository struct{ db *gorm.DB }

// NewPersonalAssetRepository создаёт репозиторий личных активов с зависимостями и
// ограничениями, необходимыми для работы с личными активами.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий личных активов.
func NewPersonalAssetRepository(db *gorm.DB) *PersonalAssetRepository {
	return &PersonalAssetRepository{db: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})}
}

type assetConversation struct {
	ID, Type                 string
	DeletedAt                *time.Time
	AvatarKey, AvatarVersion *string
}

// lockAssetConversation блокирует авторизованную группу перед изменением метаданных её аватара.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - lock: Режим блокировки или требование блокировки для операции с базой данных.
//
// @returns:
//   - result1: Результирующий диалог об активе.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func lockAssetConversation(tx *gorm.DB, id, lock string) (assetConversation, error) {
	var row assetConversation
	// Режим блокировки — это внутренняя константа, не требующая ввода данных.  Проверка
	// членства в группе выполняется в следующем операторе SQL после завершения всех
	// параллельных мутаций.
	result := tx.Raw("SELECT id,type,deleted_at,avatar_key,avatar_version FROM conversations WHERE id=? FOR "+lock, id).Scan(&row)
	if result.Error != nil {
		return row, result.Error
	}
	if row.ID == "" {
		return row, apperrors.ErrNotFound
	}
	return row, nil
}

// authorizeAsset проверяет членство в группе перед предоставлением доступа к приватному аватару
// или вложению.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - row: Состояние диалога об активе, обработанное в ходе этой операции.
//   - manage: Требует ли операция разрешения на управление группами.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func authorizeAsset(tx *gorm.DB, user string, row assetConversation, manage bool) error {
	if row.Type != "group" || row.DeletedAt != nil {
		return apperrors.ErrNotFound
	}
	var member struct{ Role string }
	result := tx.Raw(`SELECT m.role FROM conversation_members m JOIN users u ON u.id=m.user_id
	 WHERE m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL AND u.guest_conference_id IS NULL AND u.password_hash<>''`, row.ID, user).Scan(&member)
	if result.Error != nil {
		return result.Error
	}
	if member.Role == "" || (manage && member.Role != "owner" && member.Role != "admin") {
		return apperrors.ErrForbidden
	}
	return nil
}

// BeginAvatar создает ожидающую публикации публикацию с аватаром после блокировки
// авторизованной группы.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - asset: Личные метаданные аватара или вложения.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalAssetRepository) BeginAvatar(ctx context.Context, user, id string, asset assets.Asset) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockAssetConversation(tx, id, "UPDATE")
		if err != nil {
			return err
		}
		if err = authorizeAsset(tx, user, row, true); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO conversation_avatar_objects(id,conversation_id,object_key,content_type,size,checksum)
		 VALUES(?,?,?,?,?,?)`, asset.Version, id, asset.Key, asset.ContentType, asset.Size, asset.Checksum).Error
	})
}

// ActivateAvatar атомарно делает текущий загруженный аватар видимым в групповом чате.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - version: Версия или значение ограждения, используемое для предотвращения устаревания прав
//     собственности.
//
// @returns:
//   - result1: Полученная беседа.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalAssetRepository) ActivateAvatar(ctx context.Context, user, id, version string) (personal.Conversation, error) {
	var item personal.Conversation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockAssetConversation(tx, id, "UPDATE")
		if err != nil {
			return err
		}
		if err = authorizeAsset(tx, user, row, true); err != nil {
			return err
		}
		var pending struct{ Key string }
		result := tx.Raw(`SELECT object_key AS key FROM conversation_avatar_objects WHERE id=? AND conversation_id=?
		 AND state='pending' AND cleanup_after>clock_timestamp() FOR UPDATE`, version, id).Scan(&pending)
		if result.Error != nil {
			return result.Error
		}
		if pending.Key == "" {
			return apperrors.ErrConflict
		}
		if row.AvatarVersion != nil {
			if err = tx.Exec(`UPDATE conversation_avatar_objects SET state='retired',cleanup_after=clock_timestamp(),cleanup_token=NULL,updated_at=clock_timestamp() WHERE id=? AND state='active'`, *row.AvatarVersion).Error; err != nil {
				return err
			}
		}
		if err = tx.Exec(`UPDATE conversations SET avatar_key=?,avatar_version=?,metadata_version=metadata_version+1,updated_at=clock_timestamp() WHERE id=?`, pending.Key, version, id).Error; err != nil {
			return err
		}
		if err = tx.Exec(`UPDATE conversation_avatar_objects SET state='active',updated_at=clock_timestamp() WHERE id=?`, version).Error; err != nil {
			return err
		}
		item, err = (&PersonalRepository{db: tx}).Get(ctx, user, id)
		return err
	})
	return item, err
}

// ClearAvatar очищает активные метаданные аватара группы и определяет устаревшие объекты для
// удаления.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученная беседа.
//   - result2: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalAssetRepository) ClearAvatar(ctx context.Context, user, id string) (personal.Conversation, bool, error) {
	var item personal.Conversation
	changed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockAssetConversation(tx, id, "UPDATE")
		if err != nil {
			return err
		}
		if err = authorizeAsset(tx, user, row, true); err != nil {
			return err
		}
		if row.AvatarVersion != nil {
			if err = tx.Exec(`UPDATE conversation_avatar_objects SET state='retired',cleanup_after=clock_timestamp(),cleanup_token=NULL,updated_at=clock_timestamp() WHERE id=? AND state='active'`, *row.AvatarVersion).Error; err != nil {
				return err
			}
			changed = true
			if err = tx.Exec(`UPDATE conversations SET avatar_key=NULL,avatar_version=NULL,metadata_version=metadata_version+1,updated_at=clock_timestamp() WHERE id=?`, id).Error; err != nil {
				return err
			}
		}
		item, err = (&PersonalRepository{db: tx}).Get(ctx, user, id)
		return err
	})
	return item, changed, err
}

// attachedGroupAsset загружает завершенное вложение, которое на самом деле прикреплено к
// видимому групповому сообщению.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Результирующий актив.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func attachedGroupAsset(tx *gorm.DB, conversation, id string) (assets.Asset, error) {
	var item assets.Asset
	result := tx.Raw(`SELECT a.object_key AS key,a.filename,a.mime_type AS content_type,a.size,a.checksum,a.conversation_id
	 FROM conversation_attachments a JOIN conversation_messages m ON m.id=a.message_id AND m.conversation_id=a.conversation_id
	 WHERE a.id=? AND a.conversation_id=? AND a.status='attached' AND m.deleted_at IS NULL`, id, conversation).Scan(&item)
	if result.Error != nil {
		return item, result.Error
	}
	if item.Key == "" {
		return item, apperrors.ErrNotFound
	}
	return item, nil
}

// GroupAttachment загружает завершенное вложение только в том случае, если на него ссылается
// видимое групповое сообщение.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: Результирующий актив.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalAssetRepository) GroupAttachment(ctx context.Context, user, conversation, id string) (bool, assets.Asset, error) {
	handled := true
	var item assets.Asset
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockAssetConversation(tx, conversation, "SHARE")
		if err != nil {
			return err
		}
		if row.Type == "direct" {
			handled = false
			return nil
		}
		if err = authorizeAsset(tx, user, row, false); err != nil {
			return err
		}
		item, err = attachedGroupAsset(tx, conversation, id)
		return err
	})
	return handled, item, err
}

// VisitAsset выполняет операцию с активом в рамках области видимости, пока актив и его
// идентификационные данные действительны.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conversation: Идентификатор чата, ограничивающий область действия операции.
//   - attachment: Идентификатор личного вложения или метаданные, в зависимости от его типа.
//   - visit: Обратный вызов, вызываемый, пока актив и его идентификационные данные
//     действительны.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalAssetRepository) VisitAsset(ctx context.Context, user, conversation, attachment string, visit func(assets.Asset) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockAssetConversation(tx, conversation, "SHARE")
		if err != nil {
			return err
		}
		if err = authorizeAsset(tx, user, row, false); err != nil {
			return err
		}
		var item assets.Asset
		if attachment != "" {
			item, err = attachedGroupAsset(tx, conversation, attachment)
		} else {
			if row.AvatarVersion == nil || row.AvatarKey == nil {
				return apperrors.ErrNotFound
			}
			result := tx.Raw(`SELECT id AS version,conversation_id,object_key AS key,content_type,size,checksum FROM conversation_avatar_objects
			 WHERE id=? AND conversation_id=? AND object_key=? AND state='active'`, *row.AvatarVersion, conversation, *row.AvatarKey).Scan(&item)
			err = result.Error
			if err == nil && item.Key == "" {
				err = apperrors.ErrNotFound
			}
		}
		if err != nil {
			return err
		}
		return visit(item)
	})
}

// AvatarCleanupCandidates возвращает пакет устаревших объектов групповых аватаров.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Собранные строковые значения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalAssetRepository) AvatarCleanupCandidates(ctx context.Context, limit int) ([]string, error) {
	if limit < 1 || limit > 25 {
		return nil, apperrors.ErrInvalidInput
	}
	// Сохраняйте краткую историю сверки, не допуская бесконечного роста успешно очищенных
	// метаданных. Никогда не удаляйте текущий указатель или активный объект жизненного цикла.
	if err := r.db.WithContext(ctx).Exec(`WITH stale AS (
	 SELECT a.id FROM conversation_avatar_objects a
	 WHERE a.state='cleaned' AND a.updated_at<clock_timestamp()-INTERVAL '7 days'
	 AND NOT EXISTS(SELECT 1 FROM conversations c WHERE c.avatar_version=a.id)
	 ORDER BY a.updated_at,a.id LIMIT ? FOR UPDATE OF a SKIP LOCKED
	) DELETE FROM conversation_avatar_objects WHERE id IN(SELECT id FROM stale)`, limit).Error; err != nil {
		return nil, err
	}
	var ids []string
	err := r.db.WithContext(ctx).Raw(`SELECT a.id FROM conversation_avatar_objects a JOIN conversations c ON c.id=a.conversation_id
	 WHERE (a.state IN('pending','retired','cleaning') AND a.cleanup_after<=clock_timestamp())
	 OR (a.state='active' AND c.deleted_at IS NOT NULL)
	 ORDER BY a.cleanup_after,a.id LIMIT ?`, limit).Scan(&ids).Error
	return ids, err
}

// ClaimAvatarCleanup запрашивает один устаревший объект-аватар, чтобы параллельные рабочие
// процессы не могли удалить текущий аватар.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - version: Версия или значение ограждения, используемое для предотвращения устаревания прав
//     собственности.
//
// @returns:
//   - result1: Созданный или загруженный ресурс.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalAssetRepository) ClaimAvatarCleanup(ctx context.Context, version string) (*assets.Asset, error) {
	var item *assets.Asset
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var parent struct{ ConversationID string }
		if err := tx.Raw("SELECT conversation_id FROM conversation_avatar_objects WHERE id=?", version).Scan(&parent).Error; err != nil {
			return err
		}
		if parent.ConversationID == "" {
			return nil
		}
		row, err := lockAssetConversation(tx, parent.ConversationID, "UPDATE")
		if err != nil {
			return err
		}
		var candidate struct {
			assets.Asset
			State        string
			CleanupAfter time.Time
		}
		result := tx.Raw(`SELECT id AS version,conversation_id,object_key AS key,content_type,size,checksum,state,cleanup_after
		 FROM conversation_avatar_objects WHERE id=? FOR UPDATE`, version).Scan(&candidate)
		if result.Error != nil {
			return result.Error
		}
		// Текущий аватар никогда не запрашивается. Группа с обратимым удалением больше не
		// доступна для чтения, и её активный объект может быть атомарно удалён.
		current := row.AvatarVersion != nil && *row.AvatarVersion == version && row.AvatarKey != nil && *row.AvatarKey == candidate.Key
		if current && row.DeletedAt == nil {
			return nil
		}
		if current {
			if err = tx.Exec("UPDATE conversations SET avatar_key=NULL,avatar_version=NULL WHERE id=?", row.ID).Error; err != nil {
				return err
			}
		} else if !(candidate.State == "active" && row.DeletedAt != nil) && ((candidate.State != "pending" && candidate.State != "retired" && candidate.State != "cleaning") || candidate.CleanupAfter.After(time.Now())) {
			return nil
		}
		candidate.CleanupToken = uuid.NewString()
		if err = tx.Exec(`UPDATE conversation_avatar_objects SET state='cleaning',cleanup_token=?,cleanup_after=clock_timestamp()+INTERVAL '1 minute',updated_at=clock_timestamp() WHERE id=?`, candidate.CleanupToken, version).Error; err != nil {
			return err
		}
		item = &candidate.Asset
		return nil
	})
	return item, err
}

// FinishAvatarCleanup записывает, был ли успешно удалён запрошенный объект-аватар.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - item: Состояние ресурса, обработанное в ходе этой операции.
//   - success: Успешно ли завершена очистка запрошенного объекта.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *PersonalAssetRepository) FinishAvatarCleanup(ctx context.Context, item assets.Asset, success bool) error {
	state := "retired"
	if success {
		state = "cleaned"
	}
	result := r.db.WithContext(ctx).Exec(`UPDATE conversation_avatar_objects SET state=?,cleanup_token=NULL,cleanup_after=clock_timestamp()+INTERVAL '1 minute',updated_at=clock_timestamp()
	 WHERE id=? AND state='cleaning' AND cleanup_token=?`, state, item.Version, item.CleanupToken)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return apperrors.ErrConflict
	}
	return nil
}

var _ assets.AssetRepository = (*PersonalAssetRepository)(nil)

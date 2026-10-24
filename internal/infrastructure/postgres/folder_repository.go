// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/folders"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type FolderRepository struct{ db *gorm.DB }

// NewFolderRepository создаёт репозиторий папок с зависимостями и ограничениями, необходимыми
// для работы с папками.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный репозиторий папок.
func NewFolderRepository(db *gorm.DB) *FolderRepository {
	return &FolderRepository{db: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})}
}

// Account проверяет, является ли действующий аккаунт зарегистрированным и не является ли он
// гостевым.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) Account(ctx context.Context, actor string) error {
	return NewPersonalRepository(r.db).Account(ctx, actor)
}

// These канонические допустимые наборы для подсчётов, элементов и выбора.  Неактивные
// сопоставления сохраняются, но никогда не становятся объектами авторизации.
const folderConversationsSQL = `SELECT c.id, COALESCE(lm.created_at,c.created_at) AS activity_at,
 CASE WHEN c.type='group' THEN c.name ELSE COALESCE(NULLIF(peer.display_name,''),'Пользователь') END AS label
 FROM conversation_members m JOIN conversations c ON c.id=m.conversation_id
 JOIN users actor ON actor.id=m.user_id AND actor.guest_conference_id IS NULL AND actor.password_hash<>''
 LEFT JOIN users peer ON c.type='direct' AND peer.id=CASE WHEN c.user_low_id=m.user_id THEN c.user_high_id ELSE c.user_low_id END
 LEFT JOIN conversation_messages lm ON lm.id=c.last_message_id AND lm.sequence>m.history_cleared_through
 WHERE m.user_id=? AND m.left_at IS NULL AND m.hidden_at IS NULL AND c.deleted_at IS NULL AND c.type IN('direct','group')`
const folderConferencesSQL = `SELECT c.id,COALESCE(c.finished_at,c.scheduled_at,c.created_at) AS activity_at,c.title AS label
 FROM conference_participants p JOIN conferences c ON c.id=p.conference_id
 JOIN users actor ON actor.id=p.user_id AND actor.guest_conference_id IS NULL AND actor.password_hash<>''
 WHERE c.kind='meeting' AND p.user_id=? AND p.admission_state IN('admitted','waiting') AND p.status NOT IN('kicked','rejected')
 AND NOT EXISTS(SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=c.id AND cp.user_id=p.user_id AND cp.left_at IS NOT NULL)`
const folderEligibleSQL = `WITH eligible_conversations AS (` + folderConversationsSQL + `), eligible_conferences AS (` + folderConferencesSQL + `) `
const folderCountProjection = `SELECT f.id,f.name,f.position,f.created_at,f.updated_at,
 (SELECT count(*) FROM folder_conversations fc JOIN eligible_conversations ec ON ec.id=fc.conversation_id WHERE fc.folder_id=f.id) AS conversation_count,
 (SELECT count(*) FROM folder_conferences fc JOIN eligible_conferences ec ON ec.id=fc.conference_id WHERE fc.folder_id=f.id) AS conference_count`

type folderRow struct {
	folders.Folder `gorm:"embedded"`
	ItemContains   bool
}

// folderRows формирует запрос к папке с областью видимости владельца, используемый при работе с
// папками.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - itemKind: Проверенный тип ресурса, связанного с папкой.
//   - itemID: Идентификатор ресурса, связанного с папкой.
//
// @returns:
//   - result1: Собранные значения для папки.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func folderRows(db *gorm.DB, actor, id, itemKind, itemID string) ([]folders.Folder, error) {
	query := folderEligibleSQL + folderCountProjection
	args := []any{actor, actor}
	if itemKind != "" {
		query += `,CASE WHEN ?='conversation' THEN EXISTS(SELECT 1 FROM folder_conversations fc JOIN eligible_conversations ec ON ec.id=fc.conversation_id WHERE fc.folder_id=f.id AND fc.conversation_id=?) ELSE EXISTS(SELECT 1 FROM folder_conferences fc JOIN eligible_conferences ec ON ec.id=fc.conference_id WHERE fc.folder_id=f.id AND fc.conference_id=?) END AS item_contains`
		args = append(args, itemKind, itemID, itemID)
	}
	query += ` FROM folders f WHERE f.user_id=?`
	args = append(args, actor)
	if id != "" {
		query += ` AND f.id=?`
		args = append(args, id)
	}
	query += ` ORDER BY f.position,f.id LIMIT 100`
	rows := []folderRow{}
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]folders.Folder, 0, len(rows))
	for _, row := range rows {
		row.ItemCount = row.ConversationCount + row.ConferenceCount
		if itemKind != "" {
			value := row.ItemContains
			row.Contains = &value
		}
		items = append(items, row.Folder)
	}
	return items, nil
}

// folderView формирует ответ для папки с текущим количеством элементов.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученная папка.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func folderView(db *gorm.DB, actor, id string) (folders.Folder, error) {
	items, err := folderRows(db, actor, id, "", "")
	if err != nil {
		return folders.Folder{}, err
	}
	if len(items) != 1 {
		return folders.Folder{}, apperrors.ErrNotFound
	}
	return items[0], nil
}

// lockFolderActor блокирует и проверяет учетную запись, в которой изменяется коллекция папок.
//
// NO KEY UPDATE сериализует локальные изменения в учетной записи, не вступая в конфликт с
// блокировками на основе общего ключа, которые удерживаются при записи сообщений и участников
// на основе FK.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func lockFolderActor(tx *gorm.DB, actor string) error {
	var id string
	result := tx.Raw(`SELECT id FROM users WHERE id=? AND guest_conference_id IS NULL AND password_hash<>'' FOR NO KEY UPDATE`, actor).Scan(&id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return apperrors.ErrForbidden
	}
	return nil
}

// lockOwnedFolder блокирует папку только в том случае, если она принадлежит действующей учетной
// записи.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func lockOwnedFolder(tx *gorm.DB, actor, id string) error {
	var found string
	result := tx.Raw(`SELECT id FROM folders WHERE id=? AND user_id=? FOR UPDATE`, id, actor).Scan(&found)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return apperrors.ErrNotFound
	}
	return nil
}

// folderConflict сопоставляет нарушение уникальности папки с ошибкой публичного конфликта.
//
// @Parameters:
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func folderConflict(err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" && pgError.ConstraintName == "folders_owner_name_key" {
		return apperrors.New(apperrors.ErrConflict, "a folder with this name already exists")
	}
	return err
}

// folderID проверяет идентификатор папки перед сохранением доступа.
//
// @Parameters:
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func folderID(id string) error { _, err := chat.UUID(id); return err }

// List возвращает ограниченную страницу записей о папках с использованием заданных параметров
// разбивки на страницы и фильтров.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - itemKind: Проверенный тип ресурса, связанного с папкой.
//   - itemID: Идентификатор ресурса, связанного с папкой.
//
// @returns:
//   - result1: Собранные значения для папки.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) List(ctx context.Context, actor, itemKind, itemID string) ([]folders.Folder, error) {
	if itemKind != "" || itemID != "" {
		if !folders.ValidKind(itemKind) || folderID(itemID) != nil {
			return nil, apperrors.ErrInvalidInput
		}
	}
	if err := r.Account(ctx, actor); err != nil {
		return nil, err
	}
	return folderRows(r.db.WithContext(ctx), actor, "", itemKind, itemID)
}

// Get загружает запрошенную папку в рамках полномочий вызывающей стороны.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Полученная папка.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) Get(ctx context.Context, actor, id string) (folders.Folder, error) {
	if err := folderID(id); err != nil {
		return folders.Folder{}, err
	}
	if err := r.Account(ctx, actor); err != nil {
		return folders.Folder{}, err
	}
	return folderView(r.db.WithContext(ctx), actor, id)
}

// Create создаёт и сохраняет новую папку для действующей учётной записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - name: Имя параметра, файла cookie, ресурса или провайдера, к которому осуществляется
//     обращение.
//
// @returns:
//   - result1: Полученная папка.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) Create(ctx context.Context, actor, name string) (folders.Folder, error) {
	name, key, err := folders.NormalizeName(name)
	if err != nil {
		return folders.Folder{}, err
	}
	id := uuid.NewString()
	var item folders.Folder
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockFolderActor(tx, actor); err != nil {
			return err
		}
		var count int64
		if err := tx.Table("folders").Where("user_id=?", actor).Count(&count).Error; err != nil {
			return err
		}
		if count >= folders.MaxFolders {
			return apperrors.New(apperrors.ErrConflict, "at most 100 folders are supported")
		}
		if err := tx.Exec(`INSERT INTO folders(id,user_id,name,name_key,position) VALUES(?,?,?,?,?)`, id, actor, name, key, count).Error; err != nil {
			return err
		}
		var e error
		item, e = folderView(tx, actor, id)
		return e
	})
	return item, folderConflict(err)
}

// Rename нормализует имя новой папки и обновляет папку, принадлежащую действующей учётной
// записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - name: Имя параметра, файла cookie, ресурса или провайдера, к которому осуществляется
//     обращение.
//
// @returns:
//   - result1: Полученная папка.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) Rename(ctx context.Context, actor, id, name string) (folders.Folder, error) {
	if err := folderID(id); err != nil {
		return folders.Folder{}, err
	}
	name, key, err := folders.NormalizeName(name)
	if err != nil {
		return folders.Folder{}, err
	}
	var item folders.Folder
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockFolderActor(tx, actor); err != nil {
			return err
		}
		if err := lockOwnedFolder(tx, actor, id); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE folders SET name=?,name_key=?,updated_at=clock_timestamp() WHERE id=?`, name, key, id).Error; err != nil {
			return err
		}
		var e error
		item, e = folderView(tx, actor, id)
		return e
	})
	return item, folderConflict(err)
}

// Delete удаляет запрошенную папку, используя область видимости вызывающей стороны и правила
// жизненного цикла ресурса.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) Delete(ctx context.Context, actor, id string) error {
	if err := folderID(id); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockFolderActor(tx, actor); err != nil {
			return err
		}
		if err := lockOwnedFolder(tx, actor, id); err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM folders WHERE id=?`, id).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE folders f SET position=ranked.position FROM (SELECT id,(row_number() OVER(ORDER BY position,id)-1)::integer AS position FROM folders WHERE user_id=?) ranked WHERE f.id=ranked.id`, actor).Error
	})
}

// В Order хранится валидированный порядок расположения личных папок действующего аккаунта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - ids: Идентификаторы ресурсов для обработки в рамках ограниченного пакета.
//
// @returns:
//   - result1: Собранные значения для папки.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) Order(ctx context.Context, actor string, ids []string) ([]folders.Folder, error) {
	if len(ids) > folders.MaxFolders {
		return nil, apperrors.ErrInvalidInput
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if folderID(id) != nil || seen[id] {
			return nil, apperrors.ErrInvalidInput
		}
		seen[id] = true
	}
	var items []folders.Folder
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockFolderActor(tx, actor); err != nil {
			return err
		}
		var existing []string
		if err := tx.Table("folders").Where("user_id=?", actor).Pluck("id", &existing).Error; err != nil {
			return err
		}
		if len(existing) != len(ids) {
			return apperrors.New(apperrors.ErrConflict, "folder order changed; reload folders")
		}
		for _, id := range existing {
			if !seen[id] {
				return apperrors.New(apperrors.ErrConflict, "folder order does not match the current account")
			}
		}
		// Не более 100 папок; одно обновление VALUES позволяет избежать запроса для каждой
		// позиции.
		if len(ids) > 0 {
			values := make([]string, 0, len(ids))
			args := make([]any, 0, len(ids)*2)
			for position, id := range ids {
				values = append(values, "(?::uuid,?::integer)")
				args = append(args, id, position)
			}
			if err := tx.Exec(`UPDATE folders f SET position=v.position,updated_at=clock_timestamp() FROM (VALUES `+strings.Join(values, ",")+`) AS v(id,position) WHERE f.id=v.id`, args...).Error; err != nil {
				return err
			}
		}
		var e error
		items, e = folderRows(tx, actor, "", "", "")
		return e
	})
	return items, err
}

// lockFolderTarget блокирует и проверяет ресурс, добавляемый в личную папку или удаляемый из
// неё.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func lockFolderTarget(tx *gorm.DB, actor, kind, id string) error {
	table := "conversations"
	eligible := folderConversationsSQL
	if kind == "conference" {
		table = "conferences"
		eligible = folderConferencesSQL
	}
	var found string
	result := tx.Raw(`SELECT id FROM `+table+` WHERE id=? FOR SHARE`, id).Scan(&found)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return apperrors.ErrForbidden
	}
	// После получения общей блокировки целевого объекта этот оператор делает новый снимок. При
	// параллельном отзыве прав доступа нельзя использовать более ранний предикат
	// принадлежности.
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM (`+eligible+`) eligible WHERE id=?`, actor, id).Scan(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return apperrors.ErrForbidden
	}
	return nil
}

// SetItem обновляет элемент для папки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - folder: Идентификатор личной папки действующего аккаунта.
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//   - add: Указывает, нужно ли добавить целевой элемент, а не удалить его.
//
// @returns:
//   - result1: Полученная папка.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) SetItem(ctx context.Context, actor, folder, kind, id string, add bool) (folders.Folder, error) {
	if !folders.ValidKind(kind) || folderID(folder) != nil || folderID(id) != nil {
		return folders.Folder{}, apperrors.ErrInvalidInput
	}
	var item folders.Folder
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := NewPersonalRepository(tx).Account(ctx, actor); err != nil {
			return err
		}
		// Отклоняет запрос при несовпадении владельца и области видимости, прежде чем
		// обращаться к целевому объекту или блокировать его.
		var owned int64
		if err := tx.Table("folders").Where("id=? AND user_id=?", folder, actor).Count(&owned).Error; err != nil {
			return err
		}
		if owned != 1 {
			return apperrors.ErrNotFound
		}
		if add {
			if err := lockFolderTarget(tx, actor, kind, id); err != nil {
				return err
			}
		}
		if err := lockFolderActor(tx, actor); err != nil {
			return err
		}
		if err := lockOwnedFolder(tx, actor, folder); err != nil {
			return err
		}
		table, column := "folder_conversations", "conversation_id"
		if kind == "conference" {
			table, column = "folder_conferences", "conference_id"
		}
		query := `DELETE FROM ` + table + ` WHERE folder_id=? AND ` + column + `=?`
		if add {
			query = `INSERT INTO ` + table + `(folder_id,` + column + `) VALUES(?,?) ON CONFLICT DO NOTHING`
		}
		result := tx.Exec(query, folder, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			if err := tx.Exec(`UPDATE folders SET updated_at=clock_timestamp() WHERE id=?`, folder).Error; err != nil {
				return err
			}
		}
		var e error
		item, e = folderView(tx, actor, folder)
		return e
	})
	return item, err
}

type folderCursor struct {
	Actor, Scope, Folder, Kind, ID string
	Filter                         folders.Filter
	At                             time.Time
}
type folderItemRow struct {
	ID, Kind   string
	ActivityAt time.Time
	Payload    []byte
	InFolder   bool
}

// Полные метаданные чата отображаются только для выбранной страницы.  Предикат принадлежности
// по-прежнему используется в этом же операторе, до LIMIT.
const folderPersonalJSON = `jsonb_build_object('id',p.id,'type',p.type,'peer',CASE WHEN p.type='direct' THEN jsonb_build_object('id',p.peer_id,'displayName',p.peer_name) ELSE NULL END,
 'name',p.name,'description',p.description,'createdBy',p.created_by,'updatedAt',p.updated_at,'memberCount',p.member_count,'myRole',CASE WHEN p.type='group' THEN p.my_role ELSE '' END,
 'avatarVersion',p.avatar_version,'lastSender',CASE WHEN p.last_sender_id IS NULL THEN NULL ELSE jsonb_build_object('id',p.last_sender_id,'displayName',p.last_sender_name) END,
 'createdAt',p.created_at,'lastMessageAt',p.last_message_at,'lastMessageId',p.last_message_id,'preview',p.preview,'unreadCount',p.unread_count,
 'notificationsEnabled',p.notifications_enabled,'historyClearedThrough',p.history_cleared_through)`
const folderConferenceJSON = `jsonb_build_object('id',c.id,'ownerId',c.owner_id,'title',c.title,'inviteCode','','inviteUrl','','status',c.status,
 'createdAt',c.created_at,'updatedAt',c.updated_at,'startedAt',c.started_at,'finishedAt',c.finished_at,'waitingRoomEnabled',c.waiting_room_enabled,
 'scheduledAt',c.scheduled_at,'plannedDurationMin',c.planned_duration_min,'participantCount',CASE WHEN p.admission_state='admitted' AND p.status IN('joined','left') THEN
 (SELECT count(*) FROM conference_participants cp WHERE cp.conference_id=c.id AND cp.admission_state='admitted' AND cp.status IN('joined','left')) ELSE NULL END)`

// Items возвращает отфильтрованную страницу с ограниченным количеством ресурсов, связанных с
// принадлежащей учетной записи папкой.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - folder: Идентификатор личной папки действующего аккаунта.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//   - filter: Проверенные фильтры, ограничивающие набор результатов.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) Items(ctx context.Context, actor, folder, cursor string, limit int, filter folders.Filter) (folders.Page, error) {
	if folderID(folder) != nil {
		return folders.Page{}, apperrors.ErrInvalidInput
	}
	return r.itemPage(ctx, actor, folder, "items", cursor, limit, filter)
}

// Candidates возвращает доступные ресурсы, которые можно связать с папкой действующей учетной
// записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - folder: Идентификатор личной папки действующего аккаунта.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//   - filter: Проверенные фильтры, ограничивающие набор результатов.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) Candidates(ctx context.Context, actor, folder, cursor string, limit int, filter folders.Filter) (folders.Page, error) {
	if folder != "" && folderID(folder) != nil {
		return folders.Page{}, apperrors.ErrInvalidInput
	}
	return r.itemPage(ctx, actor, folder, "candidates", cursor, limit, filter)
}

// itemPage формирует авторизованную страницу с элементами папки или потенциальными ресурсами.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - actor: Идентификатор учетной записи, выполняющей операцию.
//   - folder: Идентификатор личной папки действующего аккаунта.
//   - scope: Авторизованный диалог или область доступа к ресурсу.
//   - cursor: Непрозрачная граница продолжения с предыдущей страницы.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//   - filter: Проверенные фильтры, ограничивающие набор результатов.
//
// @returns:
//   - result1: Полученная страница.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *FolderRepository) itemPage(ctx context.Context, actor, folder, scope, cursor string, limit int, filter folders.Filter) (folders.Page, error) {
	page := folders.Page{Items: []folders.Item{}}
	if limit < 1 || limit > 100 {
		return page, apperrors.ErrInvalidInput
	}
	var err error
	filter, err = folders.NormalizeFilter(filter)
	if err != nil {
		return page, err
	}
	if err = r.Account(ctx, actor); err != nil {
		return page, err
	}
	if folder != "" {
		if _, err = r.Get(ctx, actor, folder); err != nil {
			return page, err
		}
	}
	query := folderEligibleSQL + `, eligible AS (SELECT id,'conversation'::text AS kind,activity_at,label FROM eligible_conversations UNION ALL SELECT id,'conference'::text AS kind,activity_at,label FROM eligible_conferences), selected AS (SELECT e.* FROM eligible e WHERE true`
	args := []any{actor, actor}
	if folder != "" {
		// Область доступа владельца повторяется в проекции, поэтому при одновременном удалении
		// не может быть повторно использована успешная предварительная проверка детализации.
		query += ` AND EXISTS(SELECT 1 FROM folders f WHERE f.id=? AND f.user_id=?)`
		args = append(args, folder, actor)
	}
	if scope == "items" {
		query += ` AND (e.kind='conversation' AND EXISTS(SELECT 1 FROM folder_conversations fc WHERE fc.folder_id=? AND fc.conversation_id=e.id) OR e.kind='conference' AND EXISTS(SELECT 1 FROM folder_conferences fc WHERE fc.folder_id=? AND fc.conference_id=e.id))`
		args = append(args, folder, folder)
	}
	if filter.Type != "all" {
		query += ` AND e.kind=?`
		args = append(args, filter.Type)
	}
	if filter.Search != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(filter.Search)
		query += ` AND e.label ILIKE ?`
		args = append(args, "%"+escaped+"%")
	}
	if cursor != "" {
		if len(cursor) > 2048 {
			return page, apperrors.ErrInvalidInput
		}
		var c folderCursor
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil || json.Unmarshal(raw, &c) != nil || c.Actor != actor || c.Scope != scope || c.Folder != folder || c.Filter != filter || c.At.IsZero() || !folders.ValidKind(c.Kind) || folderID(c.ID) != nil {
			return page, apperrors.ErrInvalidInput
		}
		query += ` AND (e.activity_at,e.kind,e.id)<(?,?::text,?::uuid)`
		args = append(args, c.At, c.Kind, c.ID)
	}
	query += ` ORDER BY e.activity_at DESC,e.kind DESC,e.id DESC LIMIT ?)
 SELECT s.id,s.kind,s.activity_at,CASE WHEN s.kind='conversation' THEN ` + folderPersonalJSON + ` ELSE (SELECT ` + folderConferenceJSON + ` FROM conferences c JOIN conference_participants p ON p.conference_id=c.id AND p.user_id=? WHERE c.id=s.id) END AS payload`
	args = append(args, limit+1, actor)
	if scope == "candidates" && folder != "" {
		query += `,CASE WHEN s.kind='conversation' THEN EXISTS(SELECT 1 FROM folder_conversations fc WHERE fc.folder_id=? AND fc.conversation_id=s.id) ELSE EXISTS(SELECT 1 FROM folder_conferences fc WHERE fc.folder_id=? AND fc.conference_id=s.id) END AS in_folder`
		args = append(args, folder, folder)
	}
	query += ` FROM selected s LEFT JOIN LATERAL (` + personalProjection + ` AND c.id=s.id AND s.kind='conversation') p ON true ORDER BY s.activity_at DESC,s.kind DESC,s.id DESC`
	args = append(args, actor)
	var rows []folderItemRow
	if err = r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return page, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		raw, _ := json.Marshal(folderCursor{Actor: actor, Scope: scope, Folder: folder, Kind: last.Kind, ID: last.ID, Filter: filter, At: last.ActivityAt})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	for _, row := range rows {
		item := folders.Item{Type: row.Kind}
		if row.Kind == "conversation" {
			var conversation personal.Conversation
			if err = json.Unmarshal(row.Payload, &conversation); err != nil {
				return page, err
			}
			item.Item = conversation
		} else {
			var conference folders.ConferenceView
			if err = json.Unmarshal(row.Payload, &conference); err != nil {
				return page, err
			}
			item.Item = conference
		}
		if scope == "candidates" && folder != "" {
			value := row.InFolder
			item.InFolder = &value
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

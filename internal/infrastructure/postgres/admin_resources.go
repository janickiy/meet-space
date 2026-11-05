package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *AdminRepository) Conferences(ctx context.Context, q admin.Query) (admin.Page[admin.Conference], error) {
	q = q.Normalized()
	out := admin.NewPage[admin.Conference](q)
	query := r.db.WithContext(ctx).Table("conferences c").Where("c.kind='meeting'")
	if q.Q != "" {
		like := "%" + escapeAdminLike(q.Q) + "%"
		query = query.Joins("JOIN users u ON u.id=c.owner_id").Where("c.title ILIKE ? OR u.display_name ILIKE ? OR u.email ILIKE ?", like, like, like)
	}
	if q.Status != "" && q.Status != "all" {
		query = query.Where("c.status=?", q.Status)
	}
	if err := query.Count(&out.Total).Error; err != nil {
		return out, err
	}
	// owner_id is a non-null foreign key. Unfiltered counts need no owner join;
	// only the bounded page needs names. Search still joins owners for both queries.
	if q.Q == "" {
		query = query.Joins("JOIN users u ON u.id=c.owner_id")
	}
	query = query.Select(`c.id,c.title,COALESCE(NULLIF(u.display_name,''),u.email) AS owner_name,
		c.status,c.created_at,c.started_at,c.finished_at AS ended_at,
		(SELECT COUNT(*) FROM conference_participants p WHERE p.conference_id=c.id
		 AND CASE WHEN c.status='active' THEN p.status='joined' ELSE p.joined_at IS NOT NULL END) AS participant_count,
		(SELECT status FROM record r WHERE r.platform_conference_id=c.id AND r.deleted_at IS NULL
		 AND r.mode IN ('composite','audio_only','individual_tracks','screen_focus')
		 ORDER BY r.created_at DESC,r.id DESC LIMIT 1) AS recording_status`)
	order := adminOrder(q, map[string]string{"title": "c.title", "status": "c.status", "createdAt": "c.created_at", "startedAt": "c.started_at"}, "c.created_at")
	err := query.Order(order).Offset(q.Offset()).Limit(q.PageSize).Scan(&out.Items).Error
	return out, err
}

func (r *AdminRepository) Recordings(ctx context.Context, q admin.Query) (admin.Page[admin.Recording], error) {
	q = q.Normalized()
	out := admin.NewPage[admin.Recording](q)
	query := r.db.WithContext(ctx).Table("record r").Joins("LEFT JOIN conferences c ON c.id=r.platform_conference_id").Where("r.deleted_at IS NULL")
	if q.Q != "" {
		like := "%" + escapeAdminLike(q.Q) + "%"
		query = query.Where("c.title ILIKE ? OR r.uuid::text ILIKE ?", like, like)
	}
	if q.Status != "" && q.Status != "all" {
		query = query.Where("r.status=?", q.Status)
	}
	if err := query.Count(&out.Total).Error; err != nil {
		return out, err
	}
	query = query.Select(`r.uuid AS id,COALESCE(c.title,'Запись '||left(r.uuid::text,8)) AS title,
	 r.platform_conference_id AS conference_id,c.title AS conference_title,r.status,
	 r.duration_sec AS duration_seconds,r.size_bytes,r.created_at,
	 (r.status IN ('ready','partial_ready','failed','cancelled') AND
	 (r.recorder_lease_until IS NULL OR r.recorder_lease_until<=clock_timestamp())) AS can_delete`)
	order := adminOrder(q, map[string]string{"title": "c.title", "status": "r.status", "createdAt": "r.created_at", "sizeBytes": "r.size_bytes", "durationSeconds": "r.duration_sec"}, "r.created_at")
	err := query.Order(order).Offset(q.Offset()).Limit(q.PageSize).Scan(&out.Items).Error
	return out, err
}

func (r *AdminRepository) ChatSummary(ctx context.Context) (admin.Chats, error) {
	var out admin.Chats
	err := r.db.WithContext(ctx).Raw(`SELECT
	 (SELECT count(*) FROM conversations WHERE type='direct' AND deleted_at IS NULL) AS direct_chats,
	 (SELECT count(*) FROM conversations WHERE type='group' AND deleted_at IS NULL) AS group_chats,
	 (SELECT count(DISTINCT conference_id) FROM chat_messages WHERE deleted_at IS NULL) AS conference_chats,
	 ((SELECT count(*) FROM chat_messages WHERE deleted_at IS NULL)+(SELECT count(*) FROM conversation_messages WHERE deleted_at IS NULL)) AS messages,
	 ((SELECT count(*) FROM chat_attachments WHERE cleaned_at IS NULL AND status='attached')+
	  (SELECT count(*) FROM conversation_attachments WHERE cleaned_at IS NULL AND status='attached')) AS attachments`).Scan(&out).Error
	return out, err
}

// FileSummary reads persisted object metadata only; it never lists a bucket or
// walks the filesystem. Capacity and bytes of objects with unknown sizes are null.
func (r *AdminRepository) FileSummary(ctx context.Context) (admin.Files, error) {
	out := admin.Files{Categories: []admin.FileCategory{}, Scope: "Учитываются файлы с метаданными в базе; временные объекты и исторические версии хранилища не включены."}
	err := r.db.WithContext(ctx).Raw(`SELECT name,count(*) AS count,
	 CASE WHEN count(*) FILTER(WHERE size IS NULL)>0 THEN NULL ELSE COALESCE(sum(size),0) END AS bytes
	 FROM (
	 SELECT 'Записи' AS name,f.size_bytes AS size FROM record_file f JOIN record r ON r.id=f.record_id WHERE r.deleted_at IS NULL AND f.object_key<>''
	 UNION ALL SELECT 'Записи',s.size_bytes FROM record_segment s JOIN record r ON r.id=s.record_id
	 WHERE r.deleted_at IS NULL AND s.object_key<>''
	 AND NOT EXISTS (SELECT 1 FROM record_file f WHERE f.record_id=s.record_id AND f.object_key=s.object_key)
	 UNION ALL SELECT 'Вложения',size FROM chat_attachments WHERE cleaned_at IS NULL AND status IN ('ready','attached') AND object_key<>''
	 UNION ALL SELECT 'Вложения',size FROM conversation_attachments WHERE cleaned_at IS NULL AND status IN ('ready','attached') AND object_key<>''
	 UNION ALL SELECT 'Аватары групп',size FROM conversation_avatar_objects WHERE state IN ('active','retired','cleaning')
	 ) objects GROUP BY name ORDER BY name`).Scan(&out.Categories).Error
	if err != nil {
		return out, err
	}
	var total int64
	known := true
	for _, category := range out.Categories {
		out.Count += category.Count
		if category.Bytes == nil {
			known = false
		} else {
			total += *category.Bytes
		}
	}
	if known {
		out.Bytes = &total
	}
	return out, nil
}

func (r *AdminRepository) Dashboard(ctx context.Context) (admin.Dashboard, error) {
	out := admin.Dashboard{Activity: []admin.ActivityPoint{}, Services: []admin.ServiceHealth{}, CheckedAt: time.Now().UTC()}
	if err := r.db.WithContext(ctx).Raw(`SELECT count(*) AS total,
	 count(*) FILTER(WHERE NOT is_blocked) AS active,count(*) FILTER(WHERE is_blocked) AS blocked,
	 count(*) FILTER(WHERE created_at>=now()-interval '7 days') AS new
	 FROM users WHERE guest_conference_id IS NULL AND password_hash<>''`).Scan(&out.Users).Error; err != nil {
		return out, err
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT count(*) AS total,count(*) FILTER(WHERE status='active') AS active,
	 count(*) FILTER(WHERE status='finished') AS completed,
	 (SELECT count(*) FROM conference_participants p JOIN conferences c ON c.id=p.conference_id WHERE c.kind='meeting' AND c.status='active' AND p.status='joined') AS participants
	 FROM conferences WHERE kind='meeting'`).Scan(&out.Conferences).Error; err != nil {
		return out, err
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT count(*) AS total,
	 count(*) FILTER(WHERE status NOT IN ('ready','partial_ready','failed','cancelled')) AS active
	 FROM record WHERE deleted_at IS NULL`).Scan(&out.Recordings).Error; err != nil {
		return out, err
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT to_char(d.day,'YYYY-MM-DD') AS date,count(c.id) AS count
	 FROM generate_series(date_trunc('day',now() AT TIME ZONE 'UTC')-interval '6 days',date_trunc('day',now() AT TIME ZONE 'UTC'),interval '1 day') d(day)
	 LEFT JOIN conferences c ON c.kind='meeting' AND c.created_at>=d.day AT TIME ZONE 'UTC' AND c.created_at<(d.day+interval '1 day') AT TIME ZONE 'UTC'
	 GROUP BY d.day ORDER BY d.day`).Scan(&out.Activity).Error; err != nil {
		return out, err
	}
	var err error
	out.Files, err = r.FileSummary(ctx)
	if err != nil {
		return out, err
	}
	var emptyBytes int64
	out.Recordings.Bytes = &emptyBytes
	for _, category := range out.Files.Categories {
		if category.Name == "Записи" {
			out.Recordings.Bytes = category.Bytes
			break
		}
	}
	return out, err
}

// DeleteAdminRecording shares the existing object cleanup/tombstone operation.
// Row locking excludes the worker and retention process, and the audit entry is
// committed with the tombstone. Storage deletion is idempotent on retry.
func (r *AdminRepository) DeleteAdminRecording(ctx context.Context, id string, audit admin.AuditInput, remove func(context.Context, records.Record) error) (records.Record, error) {
	var record records.Record
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return record, apperrors.ErrInvalidInput
	}
	if remove == nil {
		return record, apperrors.ErrUnavailable
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminMutationGuard(tx); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid=?", id).Take(&record).Error; err != nil {
			return mapNotFound(err)
		}
		var state struct {
			Deleted bool
			Leased  bool
		}
		if err := tx.Model(&records.Record{}).Select("deleted_at IS NOT NULL AS deleted,COALESCE(recorder_lease_until>clock_timestamp(),false) AS leased").Where("id=?", record.ID).Scan(&state).Error; err != nil {
			return err
		}
		if state.Deleted {
			// A retry is successful but must not emit another deletion event.
			record = records.Record{}
			return nil
		}
		if !records.IsTerminalStatus(record.Status) || state.Leased {
			return apperrors.New(apperrors.ErrConflict, "Запись ещё обрабатывается")
		}
		audit.Action = "recording.delete"
		audit.ObjectType = "recording"
		audit.ObjectID = id
		audit.Metadata = map[string]any{"status": record.Status}
		if err := auditInsert(tx, audit); err != nil {
			return err
		}
		return removeRecordingArtifacts(ctx, tx, record, remove)
	})
	return record, err
}

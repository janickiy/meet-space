package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/errorlogs"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ErrorLogsRepository struct{ db *gorm.DB }

func NewErrorLogsRepository(db *gorm.DB) *ErrorLogsRepository {
	return &ErrorLogsRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

type errorLogRow struct {
	errorlogs.Event `gorm:"embedded"`
	StackJSON       json.RawMessage `gorm:"column:stack;type:jsonb"`
}

func (errorLogRow) TableName() string { return "error_events" }
func (r errorLogRow) event() errorlogs.Event {
	e := r.Event
	_ = json.Unmarshal(r.StackJSON, &e.Stack)
	return e
}

// WriteBatch serializes only this small journal's mutation. Receipt insertion,
// aggregation and pruning commit together, so a retry never counts twice.
func (r *ErrorLogsRepository) WriteBatch(ctx context.Context, batch errorlogs.Batch) error {
	if len(batch.Events) > errorlogs.MaxBatchSize {
		return apperrors.ErrInvalidInput
	}
	if len(batch.Events) > 0 {
		if id, e := uuid.Parse(batch.ID); e != nil {
			return apperrors.ErrInvalidInput
		} else {
			batch.ID = id.String()
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL lock_timeout = '1s'").Error; err != nil {
			return err
		}
		if err := tx.Exec("SET LOCAL statement_timeout = '2500ms'").Error; err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(748239154)").Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM error_events WHERE first_seen < clock_timestamp()-interval '14 days'").Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM error_event_batches WHERE created_at < clock_timestamp()-interval '2 hours'").Error; err != nil {
			return err
		}
		if len(batch.Events) > 0 {
			res := tx.Exec("INSERT INTO error_event_batches(id) VALUES(?::uuid) ON CONFLICT DO NOTHING", batch.ID)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return nil
			}
			grouped := map[string]errorlogs.Event{}
			cutoff := time.Now().UTC().Add(-errorlogs.RetentionDays * 24 * time.Hour)
			for _, raw := range batch.Events {
				e := errorlogs.Sanitize(raw)
				if e.FirstSeen.Before(cutoff) {
					continue
				}
				if old, ok := grouped[e.Fingerprint]; ok {
					old.Count++
					if e.FirstSeen.Before(old.FirstSeen) {
						old.FirstSeen = e.FirstSeen
					}
					if e.LastSeen.After(old.LastSeen) {
						old.LastSeen = e.LastSeen
						old.Instance = e.Instance
						old.RequestID = e.RequestID
						old.Stack = e.Stack
					}
					grouped[e.Fingerprint] = old
				} else {
					grouped[e.Fingerprint] = e
				}
			}
			// Sorting is unnecessary because the journal-wide lock already establishes
			// one lock order and bounds this batch to 256 distinct fingerprints.
			for _, e := range grouped {
				stack, _ := json.Marshal(e.Stack)
				if len(stack) == 0 || string(stack) == "null" {
					stack = []byte("[]")
				}
				err := tx.Exec(`INSERT INTO error_events(id,fingerprint,service,instance,level,code,message,route,request_id,release,first_seen,last_seen,occurrences,browser,stack)
VALUES(?::uuid,?,?,?,?,?,?,?,?,?,?,?,?,?,?::jsonb)
ON CONFLICT(fingerprint) DO UPDATE SET
 instance=CASE WHEN excluded.last_seen>=error_events.last_seen THEN excluded.instance ELSE error_events.instance END,
 request_id=CASE WHEN excluded.last_seen>=error_events.last_seen THEN excluded.request_id ELSE error_events.request_id END,
 stack=CASE WHEN excluded.last_seen>=error_events.last_seen THEN excluded.stack ELSE error_events.stack END,
 first_seen=LEAST(error_events.first_seen,excluded.first_seen),last_seen=GREATEST(error_events.last_seen,excluded.last_seen),
 occurrences=CASE WHEN error_events.occurrences>1000000000000000-excluded.occurrences THEN 1000000000000000 ELSE error_events.occurrences+excluded.occurrences END`, e.ID, e.Fingerprint, e.Service, e.Instance, e.Level, e.Code, e.Message, e.Route, e.RequestID, e.Release, e.FirstSeen, e.LastSeen, e.Count, e.Browser, string(stack)).Error
				if err != nil {
					return err
				}
			}
		}
		// The oldest groups are evicted after insertion, including concurrent writers.
		// Index ordering keeps both scans bounded by the fixed 20k journal capacity.
		if err := tx.Exec(`DELETE FROM error_events WHERE id IN (SELECT id FROM error_events ORDER BY last_seen DESC,id DESC OFFSET 20000)`).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM error_event_batches WHERE id IN (SELECT id FROM error_event_batches ORDER BY created_at DESC,id DESC OFFSET 20000)`).Error
	})
}
func errorLogQuery(db *gorm.DB, q errorlogs.Query) *gorm.DB {
	db = db.Where("first_seen >= clock_timestamp()-interval '14 days'")
	if q.Service != "" {
		db = db.Where("service=?", q.Service)
	}
	if q.Level != "" {
		db = db.Where("level=?", q.Level)
	}
	if q.Code != "" {
		db = db.Where("code=?", q.Code)
	}
	if q.RequestID != "" {
		db = db.Where("request_id=?", q.RequestID)
	}
	if q.From != "" {
		t, _ := time.Parse(time.RFC3339Nano, q.From)
		db = db.Where("last_seen>=?", t)
	}
	if q.To != "" {
		t, _ := time.Parse(time.RFC3339Nano, q.To)
		db = db.Where("last_seen<=?", t)
	}
	return db
}
func (r *ErrorLogsRepository) List(ctx context.Context, q errorlogs.Query) (errorlogs.Page, error) {
	out := errorlogs.Page{Items: []errorlogs.Event{}, RetentionDays: errorlogs.RetentionDays, MaxRecords: errorlogs.MaxRecords}
	q, err := errorlogs.ValidateQuery(q)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	db := errorLogQuery(r.db.WithContext(ctx).Model(&errorLogRow{}), q)
	if err := db.Select(`count(*) AS total,count(*) FILTER(WHERE last_seen>=clock_timestamp()-interval '1 hour') AS last_hour,count(*) FILTER(WHERE last_seen>=clock_timestamp()-interval '1 day') AS last_day`).Scan(&out.Summary).Error; err != nil {
		return out, err
	}
	// Do not reuse the Select/Scan statement, whose projection is summary-only.
	db = errorLogQuery(r.db.WithContext(ctx).Model(&errorLogRow{}), q)
	if q.Before != "" {
		c, _ := errorlogs.DecodeCursor(q.Before)
		db = db.Where("(last_seen,id) < (?,?::uuid)", c.LastSeen, c.ID)
	}
	var rows []errorLogRow
	if err := db.Order("last_seen DESC,id DESC").Limit(q.PageSize + 1).Find(&rows).Error; err != nil {
		return out, err
	}
	if len(rows) > q.PageSize {
		rows = rows[:q.PageSize]
		out.NextCursor = errorlogs.EncodeCursor(rows[len(rows)-1].Event)
	}
	for _, row := range rows {
		out.Items = append(out.Items, row.event())
	}
	return out, nil
}
func (r *ErrorLogsRepository) Get(ctx context.Context, id string) (errorlogs.Event, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return errorlogs.Event{}, apperrors.ErrInvalidInput
	}
	id = parsed.String()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var row errorLogRow
	err = r.db.WithContext(ctx).Where("id=?::uuid AND first_seen>=clock_timestamp()-interval '14 days'", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorlogs.Event{}, apperrors.ErrNotFound
	}
	return row.event(), err
}

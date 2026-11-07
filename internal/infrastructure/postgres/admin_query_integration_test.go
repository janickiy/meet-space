package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/admin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The measured regression was one full record scan per returned conference.
// Capture the real repository query so the planner check follows its SQL rather
// than a separately maintained test copy of the subquery.
type adminQueryTrace struct {
	logger.Interface
	statements []string
}

func (l *adminQueryTrace) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	sql, _ := query()
	l.statements = append(l.statements, sql)
}

func TestAuditAdminConferencePageUsesRecordingIndex(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		if err := db.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	owner := uuid.NewString()
	exec("INSERT INTO users(id,email,password_hash,display_name) VALUES(?,?,'synthetic','Owner')", owner, owner+"@example.invalid")
	exec(`INSERT INTO conferences(id,owner_id,title,invite_code,created_at)
	 SELECT md5('conference-'||g)::uuid,?,'Meeting '||g,md5('invite-'||g),now()-g*interval '1 second'
	 FROM generate_series(1,120) g`, owner)
	exec(`INSERT INTO record(conference_id,platform_conference_id,mode,source_type,transport_type,status,quality_mode,created_at)
	 SELECT md5('conference-'||((g-1)%120+1))::uuid,md5('conference-'||((g-1)%120+1))::uuid,
	 CASE g%4 WHEN 0 THEN 'composite' WHEN 1 THEN 'audio_only' WHEN 2 THEN 'individual_tracks' ELSE 'screen_focus' END,
	 'conference','webrtc',CASE WHEN g<=120 THEN 'failed' ELSE 'ready' END,'auto',now()-g*interval '1 second'
	 FROM generate_series(1,600) g`)
	exec("ANALYZE conferences")
	exec("ANALYZE record")
	trace := &adminQueryTrace{Interface: logger.Discard}
	repo := NewAdminRepository(db)
	repo.db = db.Session(&gorm.Session{Logger: trace})
	page, err := repo.Conferences(ctx, admin.Query{PageSize: 100})
	if err != nil || page.Total != 120 || len(page.Items) != 100 {
		t.Fatalf("conference page: total=%d items=%d err=%v", page.Total, len(page.Items), err)
	}
	for _, row := range page.Items {
		if row.RecordingStatus == nil || *row.RecordingStatus != "failed" {
			t.Fatalf("latest recording across supported modes: %+v", row)
		}
	}
	if len(trace.statements) != 2 {
		t.Fatalf("page must use count + one bounded query, got %d statements", len(trace.statements))
	}
	if strings.Contains(trace.statements[0], "JOIN users") {
		t.Fatal("unfiltered conference count unnecessarily joins all owners")
	}
	var plan string
	if err := db.Raw("EXPLAIN (FORMAT JSON) " + trace.statements[1]).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, `"Index Name": "idx_record_platform_page"`) {
		t.Fatal("latest-record lookup does not use the existing platform page index: " + plan)
	}
	filtered, err := repo.Conferences(ctx, admin.Query{Q: "Owner", PageSize: 20, Page: 2})
	if err != nil || filtered.Total != 120 || len(filtered.Items) != 20 {
		t.Fatalf("owner search still needs its join for count and page: total=%d items=%d err=%v", filtered.Total, len(filtered.Items), err)
	}
}

func TestAuditAdminSearchIndexMigrationUpgradeAndRollback(t *testing.T) {
	db := auditDatabase(t, 39)
	directory := filepath.Join("..", "..", "..", "database", "migrations")
	for attempt := 0; attempt < 2; attempt++ {
		if err := RunMigrations(db, directory); err != nil {
			t.Fatalf("upgrade/rerun %d: %v", attempt, err)
		}
	}
	countIndexes := func(want int64) {
		t.Helper()
		var count int64
		if err := db.Raw(`SELECT count(*) FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid
		 WHERE c.relnamespace=current_schema()::regnamespace AND i.indisvalid
		 AND c.relname IN ('admin_users_search_trgm','admin_audit_search_trgm')`).Scan(&count).Error; err != nil || count != want {
			t.Fatalf("valid search indexes: got %d want %d err=%v", count, want, err)
		}
	}
	countIndexes(2)
	for _, suffix := range []string{"down", "up"} {
		content, err := os.ReadFile(filepath.Join(directory, "legacy", "000040_administrator_search_indexes."+suffix+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(string(content)).Error; err != nil {
			t.Fatalf("%s migration: %v", suffix, err)
		}
		if suffix == "down" {
			countIndexes(0)
		} else {
			countIndexes(2)
		}
	}
}

func TestAuditAdminIndexedSearchPreservesFieldAndLiteralSemantics(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	ids := []string{"abcdefab-cdef-4abc-8def-abcdefabcdef", uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for i, row := range []struct{ name, email string }{
		{"Ada", "ada@example.invalid"},
		{"Percent%_Person", "literal@example.invalid"},
		{"PercentXPerson", "ordinary@example.invalid"},
		{`Проект\команда`, "unicode@example.invalid"},
	} {
		if err := db.Exec("INSERT INTO users(id,email,password_hash,display_name) VALUES(?,?,'synthetic',?)", ids[i], row.email, row.name).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		query string
		total int64
	}{
		{"Ada ada@", 0}, // Combined search text must not add cross-field matches.
		{"Percent%_", 1},
		{`Проект\`, 1},
		{"ad", 1},
		{"%' OR true --", 0},
		{strings.ToUpper(ids[0]), 1},
	} {
		page, err := repo.Users(ctx, admin.Query{Q: test.query})
		if err != nil || page.Total != test.total || int64(len(page.Items)) != test.total {
			t.Errorf("query %q: total=%d items=%d err=%v", test.query, page.Total, len(page.Items), err)
		}
	}
	for _, object := range []string{`object_%\key`, "objectXYkey"} {
		if err := repo.Audit(ctx, admin.AuditInput{Username: "operator", Action: "user.block", ObjectType: "user", ObjectID: object, IP: "192.0.2.1"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		query string
		total int64
	}{{"operator user", 0}, {`object_%\`, 1}, {"user.block", 2}, {"%' OR true --", 0}} {
		page, err := repo.AuditLog(ctx, admin.Query{Q: test.query})
		if err != nil || page.Total != test.total || int64(len(page.Items)) != test.total {
			t.Errorf("audit query %q: total=%d items=%d err=%v", test.query, page.Total, len(page.Items), err)
		}
	}
}

func TestAuditAdminQueryCancellationReleasesPoolWaiters(t *testing.T) {
	db := auditDatabase(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	repo := NewAdminRepository(db)
	queries := []func(context.Context) error{
		func(ctx context.Context) error { _, err := repo.Users(ctx, admin.Query{}); return err },
		func(ctx context.Context) error { _, err := repo.AuditLog(ctx, admin.Query{}); return err },
		func(ctx context.Context) error { _, err := repo.Conferences(ctx, admin.Query{}); return err },
		func(ctx context.Context) error { _, err := repo.Recordings(ctx, admin.Query{}); return err },
		func(ctx context.Context) error { _, err := repo.FileSummary(ctx); return err },
		func(ctx context.Context) error { _, err := repo.Dashboard(ctx); return err },
	}
	for _, query := range queries {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		err := query(ctx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("pool waiter ignored deadline: %v", err)
		}
	}
	if stats := sqlDB.Stats(); stats.InUse != 1 || stats.OpenConnections != 1 || stats.WaitCount < int64(len(queries)) {
		t.Fatalf("unexpected pool state with one held transaction: %+v", stats)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stats := sqlDB.Stats(); stats.InUse != 0 || stats.OpenConnections > 1 {
		t.Fatalf("cancelled queries retained pool resources: %+v", stats)
	}
}

func TestAuditAdminPaginationStableForEqualSortValues(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	repo := NewAdminRepository(db)
	var ascending []string
	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		ascending = append(ascending, id)
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{"INSERT INTO users(id,email,password_hash,display_name,created_at) VALUES(?,?,'synthetic','Same','2025-01-01T00:00:00Z')", []any{id, id + "@example.invalid"}},
			{"INSERT INTO administrators(id,username,name,password_hash,status,created_at) VALUES(?,?,'Same','synthetic','active','2025-01-01T00:00:00Z')", []any{id, fmt.Sprintf("operator%d", i)}},
			{"INSERT INTO administrator_audit_log(id,username,action,object_type,object_id,created_at) VALUES(?,'operator','user.block','user',?,'2025-01-01T00:00:00Z')", []any{id, id}},
			{"INSERT INTO conferences(id,owner_id,title,invite_code,created_at) VALUES(?,?,'Same',?,'2025-01-01T00:00:00Z')", []any{id, id, strings.ReplaceAll(id, "-", "")}},
			{`INSERT INTO record(uuid,conference_id,platform_conference_id,mode,source_type,transport_type,status,quality_mode,size_bytes,created_at)
			 VALUES(?,?,?,'composite','conference','webrtc','ready','auto',1024,'2025-01-01T00:00:00Z')`, []any{id, id, id}},
		} {
			if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	queries := []struct {
		name, sort string
		query      func(admin.Query) ([]string, error)
	}{
		{"users", "name", func(q admin.Query) ([]string, error) {
			p, err := repo.Users(ctx, q)
			var ids []string
			for _, item := range p.Items {
				ids = append(ids, item.ID)
			}
			return ids, err
		}},
		{"administrators", "name", func(q admin.Query) ([]string, error) {
			p, err := repo.List(ctx, q)
			var ids []string
			for _, item := range p.Items {
				ids = append(ids, item.ID)
			}
			return ids, err
		}},
		{"conferences", "title", func(q admin.Query) ([]string, error) {
			p, err := repo.Conferences(ctx, q)
			var ids []string
			for _, item := range p.Items {
				ids = append(ids, item.ID)
			}
			return ids, err
		}},
		{"recordings", "title", func(q admin.Query) ([]string, error) {
			p, err := repo.Recordings(ctx, q)
			var ids []string
			for _, item := range p.Items {
				ids = append(ids, item.ID)
			}
			return ids, err
		}},
		{"audit", "action", func(q admin.Query) ([]string, error) {
			p, err := repo.AuditLog(ctx, q)
			var ids []string
			for _, item := range p.Items {
				ids = append(ids, item.ID)
			}
			return ids, err
		}},
	}
	for _, test := range queries {
		for _, sort := range []string{test.sort, "createdAt", "status"} {
			for _, order := range []string{"asc", "desc"} {
				var got []string
				for page := 1; page <= 3; page++ {
					ids, err := test.query(admin.Query{Page: page, PageSize: 2, Sort: sort, Order: order})
					if err != nil {
						t.Fatalf("%s/%s/%s: %v", test.name, sort, order, err)
					}
					got = append(got, ids...)
				}
				if len(got) != len(ascending) {
					t.Fatalf("%s/%s/%s: got %d rows", test.name, sort, order, len(got))
				}
				for i, id := range got {
					want := i
					if order == "desc" {
						want = len(ascending) - 1 - i
					}
					if id != ascending[want] {
						t.Fatalf("%s/%s/%s: unstable pagination: %v", test.name, sort, order, got)
					}
				}
			}
		}
	}
}

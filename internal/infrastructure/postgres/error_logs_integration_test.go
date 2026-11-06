package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/errorlogs"
	"github.com/janickiy/meet-space/internal/operations"
)

func errorFixtureEvent(code string) errorlogs.Event {
	return errorlogs.Event{ID: uuid.NewString(), Service: "api", Instance: "synthetic-instance", Level: "ERROR", Code: code, Route: "/settings", Release: "synthetic-release", FirstSeen: time.Now().UTC(), LastSeen: time.Now().UTC()}
}
func TestAuditErrorLogIdempotencyPrivacyFiltersAndCursor(t *testing.T) {
	db := auditDatabase(t)
	repo := NewErrorLogsRepository(db)
	ctx := context.Background()
	first := errorFixtureEvent("http.5xx")
	first.Message = "private_password_and_SQL"
	first.RequestID = uuid.NewString()
	second := first
	second.ID = uuid.NewString()
	second.LastSeen = second.LastSeen.Add(time.Millisecond)
	batch := errorlogs.Batch{ID: uuid.NewString(), Events: []errorlogs.Event{first, second}}
	if err := repo.WriteBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if err := repo.WriteBatch(ctx, batch); err != nil {
		t.Fatal("retry", err)
	}
	page, err := repo.List(ctx, errorlogs.Query{RequestID: first.RequestID})
	if err != nil || len(page.Items) != 1 || page.Items[0].Count != 2 {
		t.Fatalf("idempotent aggregate: %+v %v", page, err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), first.Message) {
		t.Fatal("private raw diagnostic reached persistent response")
	}
	got, err := repo.Get(ctx, page.Items[0].ID)
	if err != nil || got.Code != "http.5xx" {
		t.Fatal("detail", err)
	}
	for _, code := range []string{"media.failed", "recording.failed", "smtp.failed"} {
		if err := repo.WriteBatch(ctx, errorlogs.Batch{ID: uuid.NewString(), Events: []errorlogs.Event{errorFixtureEvent(code)}}); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		p, err := repo.List(ctx, errorlogs.Query{PageSize: 1, Before: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if p.Summary.Total != 4 || p.RetentionDays != 14 || p.MaxRecords != 20000 {
			t.Fatalf("summary/limits: %+v", p)
		}
		for _, e := range p.Items {
			if seen[e.ID] {
				t.Fatal("cursor repeated an event")
			}
			seen[e.ID] = true
		}
		if p.NextCursor == "" {
			break
		}
		cursor = p.NextCursor
	}
	if len(seen) != 4 {
		t.Fatal("pagination omitted event")
	}
	filtered, err := repo.List(ctx, errorlogs.Query{Code: "smtp.failed", Service: "api", Level: "ERROR"})
	if err != nil || len(filtered.Items) != 1 {
		t.Fatal("filters", err)
	}
	if _, err := repo.List(ctx, errorlogs.Query{Before: "invalid"}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("invalid cursor accepted")
	}
	if _, err := repo.Get(ctx, uuid.NewString()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("missing detail", err)
	}
}
func TestAuditErrorLogStrictFourteenDayExpiryAndReset(t *testing.T) {
	db := auditDatabase(t)
	repo := NewErrorLogsRepository(db)
	ctx := context.Background()
	e := errorFixtureEvent("http.5xx")
	if err := repo.WriteBatch(ctx, errorlogs.Batch{ID: uuid.NewString(), Events: []errorlogs.Event{e}}); err != nil {
		t.Fatal(err)
	}
	page, _ := repo.List(ctx, errorlogs.Query{})
	old := page.Items[0]
	if err := db.Exec("UPDATE error_events SET first_seen=clock_timestamp()-interval '15 days',last_seen=clock_timestamp() WHERE id=?", old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, old.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("expired group visible despite new lastSeen")
	}
	p, err := repo.List(ctx, errorlogs.Query{})
	if err != nil || len(p.Items) != 0 {
		t.Fatal("expired list", err)
	}
	e = errorFixtureEvent("http.5xx")
	if err := repo.WriteBatch(ctx, errorlogs.Batch{ID: uuid.NewString(), Events: []errorlogs.Event{e}}); err != nil {
		t.Fatal(err)
	}
	p, err = repo.List(ctx, errorlogs.Query{})
	if err != nil || len(p.Items) != 1 || p.Items[0].Count != 1 || p.Items[0].ID == old.ID {
		t.Fatal("expired repeating group did not reset", err)
	}
	if err := db.Exec("UPDATE error_events SET first_seen=clock_timestamp()-interval '15 days',last_seen=clock_timestamp()-interval '15 days'").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO error_event_batches(id,created_at) VALUES(?::uuid,clock_timestamp()-interval '3 hours')", uuid.NewString()).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.WriteBatch(ctx, errorlogs.Batch{}); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Table("error_events").Count(&count)
	if count != 0 {
		t.Fatal("idle cleanup did not prune errors")
	}
	db.Table("error_event_batches").Where("created_at<clock_timestamp()-interval '2 hours'").Count(&count)
	if count != 0 {
		t.Fatal("receipt cleanup did not prune")
	}
}
func TestAuditErrorLogHardCapAndConcurrentWriters(t *testing.T) {
	db := auditDatabase(t)
	repo := NewErrorLogsRepository(db)
	ctx := context.Background()
	if err := db.Exec(`INSERT INTO error_events(id,fingerprint,service,instance,level,code,message,route,request_id,release,first_seen,last_seen,occurrences,browser,stack)
 SELECT gen_random_uuid(),repeat('0',32)||md5(n::text),'api','seed','ERROR','component.error','Ошибка компонента','/settings','','synthetic-release',clock_timestamp()-interval '2 days',clock_timestamp()-interval '2 days'+n*interval '1 microsecond',1,'','[]'::jsonb FROM generate_series(1,20005)n`).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e := errorFixtureEvent(fmt.Sprintf("go.error.%012x", i))
			failures <- repo.WriteBatch(ctx, errorlogs.Batch{ID: uuid.NewString(), Events: []errorlogs.Event{e}})
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal("concurrent write", err)
		}
	}
	var count int64
	if err := db.Table("error_events").Count(&count).Error; err != nil || count != 20000 {
		t.Fatalf("journal cap: %d %v", count, err)
	}
	p, err := repo.List(ctx, errorlogs.Query{PageSize: 100})
	if err != nil || len(p.Items) != 100 || p.Summary.Total != 20000 {
		t.Fatal("cap list", err)
	}
	for i := range 8 {
		p, err := repo.List(ctx, errorlogs.Query{Code: fmt.Sprintf("go.error.%012x", i)})
		if err != nil || len(p.Items) != 1 {
			t.Fatalf("new group was evicted instead of oldest: %d %v", i, err)
		}
	}
}

func TestAuditErrorLogGoPipelineFlushAndPrivacy(t *testing.T) {
	db := auditDatabase(t)
	repo := NewErrorLogsRepository(db)
	recorder := operations.NewErrorRecorder(repo)
	reset := operations.SetErrorRecorder(recorder)
	defer reset()
	defer recorder.Close()
	var stdout bytes.Buffer
	logger := slog.New(operations.NewErrorHandler(slog.NewJSONHandler(&stdout, nil))).With("service", "api", "instance_id", "pipeline-test", "release", "synthetic-release")
	requestID := uuid.NewString()
	logger.Error("private_database_password_and_SQL_payload", "event_type", "http.5xx", "route", "/api/v1/conferences/:id", "request_id", requestID, "error", "postgres://private_credential")
	recorder.Close()
	page, err := repo.List(context.Background(), errorlogs.Query{RequestID: requestID})
	if err != nil || len(page.Items) != 1 || page.Items[0].Code != "http.5xx" || page.Items[0].Count != 1 {
		t.Fatalf("pipeline failed: %+v %v", page, err)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "private_") || !strings.Contains(stdout.String(), "private_database") {
		t.Fatal("safe projection or original stdout broken")
	}
	if recorder.Stats().Written != 1 {
		t.Fatal("pipeline did not flush its event")
	}
}

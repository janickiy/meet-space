package postgres

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	postgresdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestRecordingRecoveryOnlySelectsSFURecords checks generated PostgreSQL queries
// without a database connection. Unsupported historical rows must neither occupy
// the recovery batch nor receive a processing lease.
func TestRecordingRecoveryOnlySelectsSFURecords(t *testing.T) {
	var sql bytes.Buffer
	db, err := gorm.Open(postgresdriver.Open("host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable"), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true,
		Logger: logger.New(log.New(&sql, "", 0), logger.Config{LogLevel: logger.Info}),
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRecordRepository(db)
	for name, query := range map[string]func() error{
		"recovery batch": func() error {
			_, err := repository.ListActiveComposite(context.Background())
			return err
		},
		"lease claim": func() error {
			_, err := repository.ClaimComposite(context.Background(), "a2401653-24bc-447e-a663-8b525d799088", "test-token", "record-worker-test", time.Minute)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			sql.Reset()
			if err := query(); err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{"source_type = 'conference'", "transport_type = 'sfu'", "platform_conference_id = conference_id", "mode IN ('composite','audio_only','individual_tracks','screen_focus')"} {
				if !strings.Contains(sql.String(), required) {
					t.Fatalf("missing SFU predicate %q in %s", required, sql.String())
				}
			}
		})
	}
}

package postgres

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func tableMigrationDirectory() string {
	return filepath.Join("..", "..", "..", "database", "migrations")
}

// Copy only the deployable table files. All integration cases therefore run
// without a historical SQL directory, even while old files exist in a checkout.
func copyTableMigrations(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files, err := filepath.Glob(filepath.Join(tableMigrationDirectory(), "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(file)), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Build the frozen initial schema through its three phases without invoking
// RunMigrations or the adoption code. A catalog golden, captured from the old
// SQL chain, independently checks that these declarations still describe v49.
func bootstrapHistoricalDatabase(t *testing.T, db *gorm.DB) {
	t.Helper()
	plan, err := loadMigrationPlan(copyTableMigrations(t))
	if err != nil {
		t.Fatal(err)
	}
	for phase := 0; phase < 3; phase++ {
		for _, table := range plan.tables {
			if strings.TrimSpace(table.phases[phase]) == "" {
				continue
			}
			if err := db.Exec(table.phases[phase]).Error; err != nil {
				t.Fatalf("historical fixture %s phase %d: %v", table.name, phase, err)
			}
		}
	}
	if err := db.Exec(`CREATE TABLE release_schema_migrations (
	 name TEXT PRIMARY KEY,checksum CHAR(64) NOT NULL,
	 applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),duration_ms BIGINT NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(historicalMigrationChecksums))
	for name := range historicalMigrationChecksums {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) != 49 {
		t.Fatalf("historical manifest has %d entries", len(names))
	}
	for _, name := range names {
		if err := db.Exec(`INSERT INTO release_schema_migrations(name,checksum,applied_at,duration_ms)
		 VALUES(?,?,'2026-10-01T12:00:00Z',7)`, name, historicalMigrationChecksums[name][0]).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func writeNumberedMigration(t *testing.T, dir string, version int, slug, sql string) string {
	t.Helper()
	name := fmt.Sprintf("%06d_%s.up.sql", version, slug)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(sql), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func migrationLedgerSnapshot(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var rows []string
	if err := db.Raw("SELECT row_to_json(m)::text FROM release_schema_migrations m ORDER BY name").Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

// Include all application rows and sequence values: checking only one user
// would miss accidental reseeding, ciphertext changes, lost chat/file metadata,
// deleted jobs or sequence resets during adoption.
func migrationDataSnapshot(t *testing.T, db *gorm.DB) map[string][]string {
	t.Helper()
	plan, err := loadMigrationPlan(tableMigrationDirectory())
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string][]string)
	for _, table := range plan.tables {
		name := table.table
		var rows []string
		if err := db.Raw(`SELECT row_to_json(r)::text FROM "` + name + `" r ORDER BY row_to_json(r)::text`).Scan(&rows).Error; err != nil {
			t.Fatal(err)
		}
		result[name] = rows
	}
	var sequences []string
	if err := db.Raw("SELECT sequencename FROM pg_sequences WHERE schemaname=current_schema() ORDER BY sequencename").Scan(&sequences).Error; err != nil {
		t.Fatal(err)
	}
	for _, name := range sequences {
		var state []string
		if err := db.Raw(`SELECT last_value::text||':'||is_called::text FROM "` + name + `"`).Scan(&state).Error; err != nil {
			t.Fatal(err)
		}
		result["sequence:"+name] = state
	}
	return result
}

func seedMigrationData(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if err := db.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	user, meeting, recording := uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec("INSERT INTO users(id,email,password_hash,display_name) VALUES(?,?,'synthetic-password-hash','Preserved user')", user, user+"@example.invalid")
	exec("INSERT INTO administrators(id,username,password_hash,name) VALUES(?,'migration_fixture','synthetic-admin-hash','Preserved administrator')", uuid.NewString())
	exec(`UPDATE platform_settings SET values='{"smtpEnabled":true,"maxMediaRooms":123}'::jsonb,
	 revision=7,smtp_password_ciphertext='synthetic-aes-gcm-ciphertext',ldap_bind_password_ciphertext='synthetic-directory-ciphertext',updated_at='2026-10-01T12:00:00Z'`)
	exec("INSERT INTO conferences(id,owner_id,title,invite_code) VALUES(?,?,'Preserved conference',?)", meeting, user, uuid.NewString()[:32])
	exec("INSERT INTO chat_messages(id,conference_id,sender_user_id,client_request_id,request_fingerprint,text) VALUES(?,?,?,?,?,'Preserved message')", uuid.NewString(), meeting, user, uuid.NewString(), strings.Repeat("a", 64))
	exec("INSERT INTO chat_attachments(id,conference_id,owner_user_id,client_request_id,filename,mime_type,size,object_key,expires_at) VALUES(?,?,?,?,'fixture.txt','text/plain',104857600,'synthetic/object',now()+interval '1 day')", uuid.NewString(), meeting, user, uuid.NewString())
	exec("INSERT INTO record(uuid,conference_id,platform_conference_id,mode,source_type,transport_type,status,quality_mode) VALUES(?,?,?,'composite','conference','webrtc','ready','auto')", recording, meeting, meeting)
	var recordID int64
	if err := db.Raw("SELECT id FROM record WHERE uuid=?", recording).Scan(&recordID).Error; err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO record_file(record_id,file_type,bucket,object_key,mime_type,size_bytes,checksum_sha256) VALUES(?,'final_mp4','fixture-bucket','fixture-recording.mp4','video/mp4',123456,?)", recordID, strings.Repeat("b", 64))
	exec("INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,dedup_key,payload) VALUES('integrations.calendar',?,?,?,'migration-fixture-job','{\"preserve\":true}')", recording, meeting, user)
}

func TestAuditTableBaselineMatchesHistoricalSchema(t *testing.T) {
	db := auditDatabase(t, 0)
	dir := copyTableMigrations(t)
	// This test compares the frozen baseline only; later numbered upgrades
	// intentionally change its catalog and have separate preservation checks.
	plan, err := loadMigrationPlan(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, upgrade := range plan.subsequent {
		if err := os.Remove(filepath.Join(dir, upgrade.name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := RunMigrations(db, dir); err != nil {
		t.Fatal(err)
	}
	var golden struct {
		TableNames  []string `json:"tableNames"`
		Definitions []string `json:"definitions"`
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "migration_schema_v49.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	got, err := historicalSchemaSnapshot(db, golden.TableNames)
	if err != nil || !reflect.DeepEqual(got, golden.Definitions) {
		t.Fatalf("new installation differs from independently captured v49 catalog: definitions=%d expected=%d err=%v", len(got), len(golden.Definitions), err)
	}
	for _, table := range []string{"platform_settings", "ldap_directory_state", "ldap_sync_runs"} {
		var count int64
		if err := db.Table(table).Where("singleton").Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("singleton %s count=%d: %v", table, count, err)
		}
	}
	t.Setenv("AUTO_MIGRATE", "false")
	before := migrationDataSnapshot(t, db)
	for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations, RunMigrations} {
		if err := run(db, dir); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) {
		t.Fatal("idempotent migration altered fresh application data")
	}
	if rows := migrationLedgerSnapshot(t, db); len(rows) != 54 {
		t.Fatalf("fresh baseline should have sentinel plus 53 table entries, got %d", len(rows))
	}
	t.Logf("identical historical PostgreSQL definitions: %d", len(got))
}

func TestAuditTableBaselineUpgradePreservesDataAndHistory(t *testing.T) {
	for _, aliases := range []bool{false, true} {
		t.Run(fmt.Sprintf("hostChecksumVariants=%v", aliases), func(t *testing.T) {
			db := auditDatabase(t, 49)
			if aliases {
				changed := 0
				for name, sums := range historicalMigrationChecksums {
					if len(sums) < 2 {
						continue
					}
					if err := db.Exec("UPDATE release_schema_migrations SET checksum=? WHERE name=?", sums[1], name).Error; err != nil {
						t.Fatal(err)
					}
					changed++
				}
				if changed == 0 {
					t.Fatal("historical host checksum aliases are missing")
				}
			}
			seedMigrationData(t, db)
			before, history := migrationDataSnapshot(t, db), migrationLedgerSnapshot(t, db)
			dir := copyTableMigrations(t)
			t.Setenv("AUTO_MIGRATE", "false")
			if err := RunStartupMigrations(db, dir); err == nil || !strings.Contains(err.Error(), "pending") {
				t.Fatalf("read-only startup accepted a pending adoption: %v", err)
			}
			for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations, RunMigrations} {
				if err := run(db, dir); err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) {
				t.Fatal("v49 adoption changed user data, ciphertext, settings, files, jobs or sequence values")
			}
			rows := migrationLedgerSnapshot(t, db)
			if len(rows) < len(history) || !reflect.DeepEqual(history, rows[:len(history)]) {
				t.Fatal("historical names, checksums, applied_at or duration changed")
			}
			if len(rows) != 103+migrationUpgradeCount(t, dir) {
				t.Fatalf("adoption should append sentinel plus 53 table entries, got %d", len(rows))
			}
			// An already adopted old installation must retain all checksum guards.
			for name := range historicalMigrationChecksums {
				if err := db.Exec("UPDATE release_schema_migrations SET checksum=? WHERE name=?", strings.Repeat("0", 64), name).Error; err != nil {
					t.Fatal(err)
				}
				break
			}
			for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations} {
				if err := run(db, dir); err == nil {
					t.Fatal("adopted historical checksum corruption was ignored")
				}
			}
		})
	}
}

func TestAuditTableBaselineRejectsUntrustedStateWithoutDataLoss(t *testing.T) {
	for _, state := range []string{"checksum", "incomplete-history", "unknown-history", "untracked-data", "schema-drift"} {
		t.Run(state, func(t *testing.T) {
			db := auditDatabase(t, 49)
			seedMigrationData(t, db)
			var err error
			switch state {
			case "checksum":
				err = db.Exec("UPDATE release_schema_migrations SET checksum=? WHERE name=(SELECT min(name) FROM release_schema_migrations)", strings.Repeat("0", 64)).Error
			case "incomplete-history":
				err = db.Exec("DELETE FROM release_schema_migrations WHERE name LIKE '000049_%'").Error
			case "unknown-history":
				err = db.Exec("INSERT INTO release_schema_migrations(name,checksum,duration_ms) VALUES('000051_unknown.up.sql',?,0)", strings.Repeat("f", 64)).Error
			case "untracked-data":
				err = db.Exec("TRUNCATE release_schema_migrations").Error
			case "schema-drift":
				err = db.Exec("DROP INDEX admin_users_search_trgm").Error
			}
			if err != nil {
				t.Fatal(err)
			}
			before, ledger := migrationDataSnapshot(t, db), migrationLedgerSnapshot(t, db)
			if err := RunMigrations(db, copyTableMigrations(t)); err == nil {
				t.Fatalf("unsafe %s adoption was accepted", state)
			}
			if !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) || !reflect.DeepEqual(ledger, migrationLedgerSnapshot(t, db)) {
				t.Fatal("rejected migration changed data, ciphertext, sequence values or the ledger")
			}
		})
	}
}

func TestAuditTableBaselineOld50RegistersTableFilesWithoutDDL(t *testing.T) {
	for _, withHistory := range []bool{false, true} {
		for _, drift := range []bool{false, true} {
			t.Run(fmt.Sprintf("history=%v/drift=%v", withHistory, drift), func(t *testing.T) {
				db := auditDatabase(t, 49)
				seedMigrationData(t, db)
				if !withHistory {
					if err := db.Exec("TRUNCATE release_schema_migrations").Error; err != nil {
						t.Fatal(err)
					}
				}
				dir := copyTableMigrations(t)
				plan, err := loadMigrationPlan(dir)
				if err != nil {
					t.Fatal(err)
				}
				if err := db.Exec(`INSERT INTO release_schema_migrations(name,checksum,applied_at,duration_ms)
				 VALUES(?,?,'2026-10-02T12:00:00Z',23)`, tableBaselineName, plan.baselineChecksum).Error; err != nil {
					t.Fatal(err)
				}
				if drift {
					if err := db.Exec("DROP INDEX admin_users_search_trgm").Error; err != nil {
						t.Fatal(err)
					}
				}
				before, ledger := migrationDataSnapshot(t, db), migrationLedgerSnapshot(t, db)
				t.Setenv("AUTO_MIGRATE", "false")
				if err := RunStartupMigrations(db, dir); err == nil {
					t.Fatal("read-only startup accepted missing numbered table registrations")
				}
				if drift {
					if err := RunMigrations(db, dir); err == nil {
						t.Fatal("old baseline50 with schema drift was silently registered")
					}
					if !reflect.DeepEqual(ledger, migrationLedgerSnapshot(t, db)) || !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) {
						t.Fatal("rejected old50 adoption changed metadata or user data")
					}
					return
				}
				for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations, RunMigrations} {
					if err := run(db, dir); err != nil {
						t.Fatal(err)
					}
				}
				after := migrationLedgerSnapshot(t, db)
				if len(after) != len(ledger)+53+migrationUpgradeCount(t, dir) || !reflect.DeepEqual(ledger, after[:len(ledger)]) {
					t.Fatal("old50 registration changed existing rows or did not append exactly 53 table entries")
				}
				if !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) {
					t.Fatal("old50 registration changed UUIDs, ciphertext, settings, messages, files, jobs or sequences")
				}
			})
		}
	}
}

func TestAuditTableBaselineRejectsPartialTableRegistration(t *testing.T) {
	db := auditDatabase(t)
	seedMigrationData(t, db)
	var name string
	if err := db.Raw("SELECT name FROM release_schema_migrations WHERE name<>? ORDER BY name LIMIT 1", tableBaselineName).Scan(&name).Error; err != nil || name == "" {
		t.Fatal("numbered table registration missing", err)
	}
	if err := db.Exec("DELETE FROM release_schema_migrations WHERE name=?", name).Error; err != nil {
		t.Fatal(err)
	}
	before, ledger := migrationDataSnapshot(t, db), migrationLedgerSnapshot(t, db)
	t.Setenv("AUTO_MIGRATE", "false")
	for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations} {
		if err := run(db, copyTableMigrations(t)); err == nil {
			t.Fatal("partial numbered table registration was silently repaired")
		}
	}
	if !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) || !reflect.DeepEqual(ledger, migrationLedgerSnapshot(t, db)) {
		t.Fatal("rejected partial registration changed data or ledger")
	}
}

func TestTableBaselineFilesAndPhases(t *testing.T) {
	dir := copyTableMigrations(t)
	plan, err := loadMigrationPlan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.tables) != 53 {
		t.Fatalf("unexpected table inventory: %d", len(plan.tables))
	}
	for index, table := range plan.tables {
		if table.name != fmt.Sprintf("%06d_%s.up.sql", index+51, table.table) {
			t.Errorf("unexpected numbered table filename: %s", table.name)
		}
		if strings.Count(table.phases[0], "CREATE TABLE ") != 1 || !strings.Contains(table.phases[0], "CREATE TABLE "+table.table+" (") {
			t.Errorf("%s must define exactly its named table", table.name)
		}
	}
	writeNumberedMigration(t, dir, 104+len(plan.subsequent), "добавить_поле", "ALTER TABLE users ADD COLUMN migration_probe TEXT;")
	changed, err := loadMigrationPlan(dir)
	if err != nil || changed.baselineChecksum != plan.baselineChecksum || len(changed.subsequent) != len(plan.subsequent)+1 {
		t.Fatalf("adding an upgrade changed the existing baseline50 checksum or was ignored: %v", err)
	}
	path := filepath.Join(dir, plan.tables[0].name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, []byte("-- changed frozen declaration\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err = loadMigrationPlan(dir)
	if err == nil && changed.baselineChecksum == plan.baselineChecksum {
		t.Fatal("frozen table bytes are not covered by the baseline checksum")
	}
}

func TestAuditTableBaselineFutureMigrationsAndGuards(t *testing.T) {
	db := auditDatabase(t)
	seedMigrationData(t, db)
	dir := copyTableMigrations(t)
	t.Setenv("AUTO_MIGRATE", "false")
	plan, err := loadMigrationPlan(dir)
	if err != nil {
		t.Fatal(err)
	}
	var table string
	for _, item := range plan.tables {
		if item.table == "users" {
			table = filepath.Join(dir, item.name)
		}
	}
	if table == "" {
		t.Fatal("users table file missing")
	}
	original, err := os.ReadFile(table)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(table, append(original, []byte("-- modified frozen definition\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations} {
		if err := run(db, dir); err == nil {
			t.Fatal("modified frozen baseline accepted")
		}
	}
	if err := os.WriteFile(table, original, 0600); err != nil {
		t.Fatal(err)
	}
	appliedName := writeNumberedMigration(t, dir, 104+len(plan.subsequent), "cross_table_probe", `ALTER TABLE users ADD COLUMN migration_parent UUID UNIQUE;
ALTER TABLE administrators ADD COLUMN migration_user UUID REFERENCES users(migration_parent);
CREATE FUNCTION migration_increment(input_value integer) RETURNS integer
LANGUAGE plpgsql AS $migration$
BEGIN
 RETURN input_value + 1;
END;
$migration$;`)
	if err := RunStartupMigrations(db, dir); err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("read-only startup accepted a pending numbered upgrade: %v", err)
	}
	for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations, RunMigrations} {
		if err := run(db, dir); err != nil {
			t.Fatal(err)
		}
	}
	var references bool
	if err := db.Raw("SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='administrators'::regclass AND contype='f' AND confrelid='users'::regclass AND convalidated)").Scan(&references).Error; err != nil || !references {
		t.Fatalf("cross-table upgrade failed: %v", err)
	}
	var incremented int
	if err := db.Raw("SELECT migration_increment(41)").Scan(&incremented).Error; err != nil || incremented != 42 {
		t.Fatalf("transaction-safe PL/pgSQL body was rejected or not applied: %d %v", incremented, err)
	}
	var count int64
	if err := db.Raw("SELECT count(*) FROM release_schema_migrations WHERE name=?", appliedName).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("numbered upgrade must create exactly one version entry: %d %v", count, err)
	}
	before, ledger := migrationDataSnapshot(t, db), migrationLedgerSnapshot(t, db)
	writeNumberedMigration(t, dir, 105+len(plan.subsequent), "failure", "ALTER TABLE users ADD COLUMN migration_rollback_probe INTEGER; CREATE TABLE migration_rollback_probe(id INTEGER); SELECT 1/0;")
	if err := RunMigrations(db, dir); err == nil {
		t.Fatal("invalid numbered upgrade accepted")
	}
	var exists bool
	if err := db.Raw("SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='users' AND column_name='migration_rollback_probe') OR to_regclass('migration_rollback_probe') IS NOT NULL").Scan(&exists).Error; err != nil || exists {
		t.Fatalf("failed upgrade was not rolled back: %v", err)
	}
	if !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) || !reflect.DeepEqual(ledger, migrationLedgerSnapshot(t, db)) {
		t.Fatal("failed upgrade changed application data or ledger")
	}
	// An already applied numbered file is immutable even if another is pending.
	path := filepath.Join(dir, appliedName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, []byte("-- tampered applied upgrade\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations} {
		if err := run(db, dir); err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Fatalf("applied upgrade checksum corruption ignored: %v", err)
		}
	}
}

func TestAuditTableBaselineRejectsTransactionEscapeWithoutDataLoss(t *testing.T) {
	db := auditDatabase(t)
	seedMigrationData(t, db)
	dir := copyTableMigrations(t)
	writeNumberedMigration(t, dir, 104+migrationUpgradeCount(t, dir), "transaction_escape", `ALTER TABLE users ADD COLUMN migration_escape_probe TEXT;
/* A migration must not commit the first ALTER before the following failure. */
CoMmIt;
SELECT 1/0;`)
	before, ledger := migrationDataSnapshot(t, db), migrationLedgerSnapshot(t, db)
	if err := RunMigrations(db, dir); err == nil {
		t.Fatal("migration with top-level transaction control was accepted")
	}
	var escaped bool
	if err := db.Raw("SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='users' AND column_name='migration_escape_probe')").Scan(&escaped).Error; err != nil || escaped {
		t.Fatalf("DDL escaped the migrator transaction: column=%v err=%v", escaped, err)
	}
	if !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) || !reflect.DeepEqual(ledger, migrationLedgerSnapshot(t, db)) {
		t.Fatal("rejected transaction escape changed user data, sequence values or migration ledger")
	}
}

func TestAuditTableBaselineConcurrentMigrationsPreserveData(t *testing.T) {
	for _, historical := range []bool{false, true} {
		t.Run(fmt.Sprintf("historical=%v", historical), func(t *testing.T) {
			version, wantLedger := 0, 54
			if historical {
				version, wantLedger = 49, 103
			}
			db := auditDatabase(t, version)
			var before map[string][]string
			if historical {
				seedMigrationData(t, db)
				before = migrationDataSnapshot(t, db)
			}
			dir := copyTableMigrations(t)
			start, results := make(chan struct{}), make(chan error, 2)
			for attempt := 0; attempt < 2; attempt++ {
				go func() {
					<-start
					results <- RunMigrations(db, dir)
				}()
			}
			close(start)
			for attempt := 0; attempt < 2; attempt++ {
				if err := <-results; err != nil {
					t.Fatalf("parallel migration failed: %v", err)
				}
			}
			wantLedger += migrationUpgradeCount(t, dir)
			if rows := migrationLedgerSnapshot(t, db); len(rows) != wantLedger {
				t.Fatalf("parallel migration duplicated or lost registrations: %d expected %d", len(rows), wantLedger)
			}
			if historical && !reflect.DeepEqual(before, migrationDataSnapshot(t, db)) {
				t.Fatal("parallel adoption changed data, ciphertext, UUIDs or sequence values")
			}
		})
	}
}

func migrationUpgradeCount(t *testing.T, dir string) int {
	t.Helper()
	plan, err := loadMigrationPlan(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(plan.subsequent)
}

package postgres

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func tableMigrationDirectory() string {
	return filepath.Join("..", "..", "..", "database", "migrations")
}

// Compare PostgreSQL's definitions, rather than SQL text: this catches omitted
// columns, indexes, defaults, FKs, trigger bodies and sequence ownership.
func migrationSchemaSnapshot(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var result []string
	query := `SELECT object FROM (
	 SELECT 'column:'||c.relname||':'||row_number() OVER(PARTITION BY c.oid ORDER BY a.attnum)||':'||a.attname||':'||format_type(a.atttypid,a.atttypmod)||':'||a.attnotnull||':'||a.attidentity::text||':'||a.attgenerated::text||':'||COALESCE(pg_get_expr(d.adbin,d.adrelid),'') AS object
	 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped
	 LEFT JOIN pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum
	 WHERE c.relnamespace=current_schema()::regnamespace AND c.relkind='r' AND c.relname<>'release_schema_migrations'
	 UNION ALL SELECT 'constraint:'||conrelid::regclass||':'||conname||':'||pg_get_constraintdef(oid)||':'||convalidated
	 FROM pg_constraint WHERE connamespace=current_schema()::regnamespace AND conrelid<>'release_schema_migrations'::regclass
	 UNION ALL SELECT 'index:'||pg_get_indexdef(i.indexrelid) FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid
	 WHERE c.relnamespace=current_schema()::regnamespace AND c.relname<>'release_schema_migrations'
	 UNION ALL SELECT 'trigger:'||pg_get_triggerdef(t.oid) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid
	 WHERE c.relnamespace=current_schema()::regnamespace AND NOT t.tgisinternal
	 UNION ALL SELECT 'function:'||pg_get_functiondef(p.oid) FROM pg_proc p
	 WHERE p.pronamespace=current_schema()::regnamespace AND NOT EXISTS(SELECT 1 FROM pg_depend d WHERE d.classid='pg_proc'::regclass AND d.objid=p.oid AND d.deptype='e')
	 UNION ALL SELECT 'sequence:'||s.relname||':'||q.seqtypid||':'||q.seqstart||':'||q.seqincrement||':'||q.seqmax||':'||q.seqmin||':'||q.seqcache||':'||q.seqcycle||':'||d.refobjid::regclass||':'||d.refobjsubid
	 FROM pg_sequence q JOIN pg_class s ON s.oid=q.seqrelid JOIN pg_depend d ON d.objid=s.oid AND d.classid='pg_class'::regclass AND d.deptype='a'
	 WHERE s.relnamespace=current_schema()::regnamespace
	) definitions ORDER BY object`
	if err := db.Raw(query).Scan(&result).Error; err != nil {
		t.Fatal(err)
	}
	return result
}

func TestAuditTableBaselineMatchesHistoricalSchema(t *testing.T) {
	db := auditDatabase(t, 49)
	want := migrationSchemaSnapshot(t, db)
	var schema string
	if err := db.Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	// auditDatabase owns this unique, disposable schema. No application database
	// or other schema is dropped, and the normal cleanup still removes this one.
	if !strings.HasPrefix(schema, "audit_") || strings.Trim(schema[6:], "0123456789abcdef") != "" {
		t.Fatal("unexpected disposable schema")
	}
	if err := db.Exec("DROP SCHEMA " + schema + " CASCADE; CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db, tableMigrationDirectory()); err != nil {
		t.Fatal(err)
	}
	got := migrationSchemaSnapshot(t, db)
	if !reflect.DeepEqual(want, got) {
		for _, definition := range want {
			found := false
			for _, actual := range got {
				found = found || actual == definition
			}
			if !found {
				t.Errorf("historical definition differs/missing: %s", definition)
			}
		}
		t.Fatalf("schema definitions differ: historical=%d baseline=%d", len(want), len(got))
	}
	for _, table := range []string{"platform_settings", "ldap_directory_state", "ldap_sync_runs"} {
		var count int64
		if err := db.Table(table).Where("singleton").Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("singleton %s count=%d: %v", table, count, err)
		}
	}
	t.Setenv("AUTO_MIGRATE", "false")
	if err := RunStartupMigrations(db, tableMigrationDirectory()); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db, tableMigrationDirectory()); err != nil {
		t.Fatal(err)
	}
	var ledgerRows int64
	if err := db.Table("release_schema_migrations").Count(&ledgerRows).Error; err != nil || ledgerRows != 1 {
		t.Fatalf("fresh baseline should have one ledger entry: %d %v", ledgerRows, err)
	}
	t.Logf("identical PostgreSQL definitions: %d", len(got))
}

func TestAuditTableBaselineUpgradePreservesDataAndHistory(t *testing.T) {
	db := auditDatabase(t, 49)
	id := uuid.NewString()
	if err := db.Exec("INSERT INTO users(id,email,password_hash) VALUES(?,?,'synthetic-hash')", id, id+"@example.invalid").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE platform_settings SET values='{"smtpEnabled":false,"maxMediaRooms":123}'::jsonb,revision=7`).Error; err != nil {
		t.Fatal(err)
	}
	var before []struct{ Name, Checksum, AppliedAt string }
	if err := db.Raw("SELECT name,checksum,applied_at::text FROM release_schema_migrations ORDER BY name").Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_MIGRATE", "false")
	if err := RunStartupMigrations(db, tableMigrationDirectory()); err == nil || !strings.Contains(err.Error(), "baseline is pending") {
		t.Fatalf("read-only startup accepted a pending baseline: %v", err)
	}
	if err := RunMigrations(db, tableMigrationDirectory()); err != nil {
		t.Fatal(err)
	}
	if err := RunStartupMigrations(db, tableMigrationDirectory()); err != nil {
		t.Fatal(err)
	}
	var after []struct{ Name, Checksum, AppliedAt string }
	if err := db.Raw("SELECT name,checksum,applied_at::text FROM release_schema_migrations WHERE name<>? ORDER BY name", tableBaselineName).Scan(&after).Error; err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("historical ledger changed: %v", err)
	}
	var count int64
	if err := db.Raw("SELECT count(*) FROM users WHERE id=? AND password_hash='synthetic-hash'", id).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("existing user changed: %v", err)
	}
	if err := db.Raw(`SELECT count(*) FROM platform_settings WHERE revision=7 AND values->>'maxMediaRooms'='123'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("settings overwritten by baseline seed: %v", err)
	}
	if err := db.Exec("UPDATE release_schema_migrations SET checksum=? WHERE name=?", strings.Repeat("0", 64), before[0].Name).Error; err != nil {
		t.Fatal(err)
	}
	for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations} {
		if err := run(db, tableMigrationDirectory()); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("historical checksum mismatch ignored after adoption: %v", err)
		}
	}
}

func TestTableBaselineFilesAndPhases(t *testing.T) {
	plan, err := loadMigrationPlan(tableMigrationDirectory())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.tables) != 53 || len(plan.history) != 49 {
		t.Fatalf("unexpected table/history inventory: %d/%d", len(plan.tables), len(plan.history))
	}
	for _, table := range plan.tables {
		name := strings.TrimSuffix(table.name, ".sql")
		if strings.Count(table.phases[0], "CREATE TABLE ") != 1 || !strings.Contains(table.phases[0], "CREATE TABLE "+name+" (") {
			t.Errorf("%s must define exactly its named table", table.name)
		}
	}
	for _, sql := range []string{"SELECT 1", "-- migrate:references\nSELECT 1;\n-- migrate:constraints\n-- migrate:structure\n", "-- migrate:structure\n-- migrate:constraints\n-- migrate:references\n"} {
		if _, err := parseTableMigration("invalid.sql", sql); err == nil {
			t.Errorf("accepted invalid phase layout: %q", sql)
		}
	}
	// Changes to a single table must change the logical baseline checksum.
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(tableMigrationDirectory())); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, plan.tables[0].name)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString("\n-- changed table definition\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	changed, err := loadMigrationPlan(dir)
	if err != nil || changed.baselineChecksum == plan.baselineChecksum {
		t.Fatalf("table bytes are not covered by baseline checksum: %v", err)
	}
}

func TestAuditTableBaselineFutureMigrationsAndGuards(t *testing.T) {
	db := auditDatabase(t)
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(tableMigrationDirectory())); err != nil {
		t.Fatal(err)
	}
	table := filepath.Join(dir, "users.sql")
	original, err := os.ReadFile(table)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(table, append(original, []byte("\n-- modified baseline\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTO_MIGRATE", "false")
	for _, run := range []func(*gorm.DB, string) error{RunMigrations, RunStartupMigrations} {
		if err := run(db, dir); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("modified table baseline accepted: %v", err)
		}
	}
	if err := os.WriteFile(table, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "000051_test_column.up.sql"), []byte("ALTER TABLE users ADD COLUMN migration_test TEXT;"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RunStartupMigrations(db, dir); err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("read-only startup accepted a future pending migration: %v", err)
	}
	if err := RunMigrations(db, dir); err != nil {
		t.Fatal(err)
	}
	if err := RunStartupMigrations(db, dir); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := db.Raw("SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='users' AND column_name='migration_test')").Scan(&exists).Error; err != nil || !exists {
		t.Fatalf("subsequent migration was not applied: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "000052_failure.up.sql"), []byte("CREATE TABLE migration_rollback_probe(id INTEGER); SELECT 1/0;"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db, dir); err == nil {
		t.Fatal("invalid future migration accepted")
	}
	if err := db.Raw("SELECT to_regclass('migration_rollback_probe') IS NOT NULL OR EXISTS(SELECT 1 FROM release_schema_migrations WHERE name='000052_failure.up.sql')").Scan(&exists).Error; err != nil || exists {
		t.Fatalf("failed migration was not rolled back atomically: %v", err)
	}
}

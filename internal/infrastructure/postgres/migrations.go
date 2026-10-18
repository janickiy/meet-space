package postgres

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// migration contains an immutable migration file and the checksum of its exact bytes.
type migration struct {
	name, checksum, sql string
}

// migrationFiles loads ordered migration definitions and their checksums.
//
// Parameters:
//   - dir: Validated local directory.
//
// Returns:
//   - result 1: The collected migration values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func migrationFiles(dir string) ([]migration, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil || len(paths) == 0 {
		return nil, fmt.Errorf("migration files unavailable")
	}
	sort.Strings(paths)
	files := make([]migration, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", filepath.Base(path), err)
		}
		if strings.TrimSpace(string(content)) == "" {
			return nil, fmt.Errorf("empty migration %s", filepath.Base(path))
		}
		files = append(files, migration{filepath.Base(path), fmt.Sprintf("%x", sha256.Sum256(content)), string(content)})
	}
	return files, nil
}

// RunStartupMigrations starts startup migrations with the configured dependencies and propagates
// startup or shutdown failures.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//   - dir: Validated local directory.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func RunStartupMigrations(db *gorm.DB, dir string) error {
	automatic := true
	if value := os.Getenv("AUTO_MIGRATE"); value != "" {
		var err error
		automatic, err = strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("AUTO_MIGRATE must be a boolean")
		}
	}
	if automatic {
		return RunMigrations(db, dir)
	}
	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return verifyMigrations(db.WithContext(ctx), files)
}

// verifyMigrations compares the applied migration ledger with the checked-in migration files.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//   - files: Ordered migration or artifact files included in this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func verifyMigrations(db *gorm.DB, files []migration) error {
	var rows []struct{ Name, Checksum string }
	if err := db.Table("release_schema_migrations").Select("name, checksum").Find(&rows).Error; err != nil {
		return fmt.Errorf("migration ledger unavailable; run explicit migration job")
	}
	applied := make(map[string]string, len(rows))
	for _, row := range rows {
		applied[row.Name] = row.Checksum
	}
	for _, file := range files {
		checksum, ok := applied[file.name]
		if !ok {
			return fmt.Errorf("migration %s is pending; run explicit migration job", file.name)
		}
		if checksum != file.checksum {
			return fmt.Errorf("migration %s checksum mismatch", file.name)
		}
	}
	return nil
}

// RunMigrations starts migrations with the configured dependencies and propagates startup or
// shutdown failures.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//   - dir: Validated local directory.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func RunMigrations(db *gorm.DB, dir string) error {
	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL lock_timeout = '15s'").Error; err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(748239105)").Error; err != nil {
			return fmt.Errorf("migration lock unavailable")
		}
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS release_schema_migrations (
			name TEXT PRIMARY KEY, checksum CHAR(64) NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now(), duration_ms BIGINT NOT NULL
		)`).Error; err != nil {
			return fmt.Errorf("create migration ledger: %w", err)
		}
		for _, file := range files {
			var checksum string
			result := tx.Raw("SELECT checksum FROM release_schema_migrations WHERE name = ?", file.name).Scan(&checksum)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				if checksum != file.checksum {
					return fmt.Errorf("migration %s checksum mismatch", file.name)
				}
				continue
			}
			started := time.Now()
			if err := tx.Exec(file.sql).Error; err != nil {
				return fmt.Errorf("apply migration %s: %w", file.name, err)
			}
			if err := tx.Exec("INSERT INTO release_schema_migrations(name,checksum,duration_ms) VALUES (?,?,?)", file.name, file.checksum, time.Since(started).Milliseconds()).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

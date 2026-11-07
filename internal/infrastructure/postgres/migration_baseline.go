package postgres

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// The baseline is immutable, just like an applied numbered migration. Its SQL
// lives in one file per table; subsequent releases still use numbered up files.
const tableBaselineName = "000050_table_baseline.up.sql"

type tableMigration struct {
	name   string
	phases [3]string
}

type migrationPlan struct {
	history, subsequent []migration
	tables              []tableMigration
	baselineChecksum    string
}

func loadMigrationPlan(dir string) (migrationPlan, error) {
	var plan migrationPlan
	paths, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return plan, err
	}
	hash := sha256.New()
	for _, path := range paths {
		name := filepath.Base(path)
		if strings.HasSuffix(name, ".up.sql") || strings.HasSuffix(name, ".down.sql") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return plan, fmt.Errorf("read table migration %s: %w", name, err)
		}
		file, err := parseTableMigration(name, string(raw))
		if err != nil {
			return plan, err
		}
		plan.tables = append(plan.tables, file)
		_, _ = hash.Write([]byte(name + "\x00"))
		_, _ = hash.Write(raw)
	}
	if len(plan.tables) == 0 {
		plan.subsequent, err = migrationFiles(dir)
		return plan, err
	}
	plan.baselineChecksum = fmt.Sprintf("%x", hash.Sum(nil))
	plan.history, err = migrationFiles(filepath.Join(dir, "legacy"))
	if err != nil {
		return plan, fmt.Errorf("historical migrations unavailable: %w", err)
	}
	if len(plan.history) != 49 {
		return plan, fmt.Errorf("table baseline needs the complete historical chain 1 through 49")
	}
	for index, file := range plan.history {
		if !strings.HasPrefix(file.name, fmt.Sprintf("%06d_", index+1)) {
			return plan, fmt.Errorf("historical migration %d is missing or duplicated", index+1)
		}
	}
	paths, err = filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return plan, err
	}
	if len(paths) > 0 {
		plan.subsequent, err = migrationFiles(dir)
		if err != nil {
			return plan, err
		}
		for _, file := range plan.subsequent {
			version, err := strconv.Atoi(strings.SplitN(file.name, "_", 2)[0])
			if err != nil || version <= 50 {
				return plan, fmt.Errorf("migration %s must follow table baseline version 50", file.name)
			}
		}
	}
	return plan, nil
}

func parseTableMigration(name, sql string) (tableMigration, error) {
	file := tableMigration{name: name}
	previous := 0
	for index, phase := range []string{"structure", "constraints", "references"} {
		marker := "-- migrate:" + phase + "\n"
		if strings.Count(sql, marker) != 1 {
			return file, fmt.Errorf("table migration %s needs exactly one %s phase", name, phase)
		}
		position := strings.Index(sql, marker)
		if position < previous {
			return file, fmt.Errorf("table migration %s has unordered phases", name)
		}
		if index > 0 {
			file.phases[index-1] = sql[previous:position]
		}
		previous = position + len(marker)
	}
	file.phases[2] = sql[previous:]
	if strings.TrimSpace(file.phases[0]) == "" {
		return file, fmt.Errorf("table migration %s has no structure", name)
	}
	return file, nil
}

func appliedMigrationChecksums(db *gorm.DB) (map[string]string, error) {
	var rows []struct{ Name, Checksum string }
	if err := db.Table("release_schema_migrations").Select("name, checksum").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("migration ledger unavailable; run explicit migration job")
	}
	applied := make(map[string]string, len(rows))
	for _, row := range rows {
		applied[row.Name] = row.Checksum
	}
	return applied, nil
}

func validateHistoricalChecksums(applied map[string]string, history []migration) error {
	for _, file := range history {
		if checksum, exists := applied[file.name]; exists && checksum != file.checksum {
			return fmt.Errorf("migration %s checksum mismatch", file.name)
		}
	}
	return nil
}

func verifyMigrationPlan(db *gorm.DB, plan migrationPlan) error {
	if len(plan.tables) == 0 {
		return verifyMigrations(db, plan.subsequent)
	}
	applied, err := appliedMigrationChecksums(db)
	if err != nil {
		return err
	}
	if err := validateHistoricalChecksums(applied, plan.history); err != nil {
		return err
	}
	if checksum, exists := applied[tableBaselineName]; !exists {
		return fmt.Errorf("table baseline is pending; run explicit migration job")
	} else if checksum != plan.baselineChecksum {
		return fmt.Errorf("migration %s checksum mismatch", tableBaselineName)
	}
	if len(plan.subsequent) > 0 {
		return verifyMigrations(db, plan.subsequent)
	}
	return nil
}

func applyMigrationPlan(tx *gorm.DB, plan migrationPlan) error {
	if len(plan.tables) == 0 {
		return applyMigrationFiles(tx, plan.subsequent)
	}
	applied, err := appliedMigrationChecksums(tx)
	if err != nil {
		return err
	}
	if err := validateHistoricalChecksums(applied, plan.history); err != nil {
		return err
	}
	if checksum, exists := applied[tableBaselineName]; exists {
		if checksum != plan.baselineChecksum {
			return fmt.Errorf("migration %s checksum mismatch", tableBaselineName)
		}
	} else {
		started := time.Now()
		if len(applied) > 0 {
			// Existing installations finish the immutable historical chain first.
			// Their tables are never recreated or populated from the baseline.
			if err := applyMigrationFiles(tx, plan.history); err != nil {
				return err
			}
		} else {
			// All tables/sequence defaults precede keys/indexes, then foreign keys
			// and triggers. This also supports cycles between chats and messages.
			for phase := 0; phase < 3; phase++ {
				for _, table := range plan.tables {
					if strings.TrimSpace(table.phases[phase]) == "" {
						continue
					}
					if err := tx.Exec(table.phases[phase]).Error; err != nil {
						return fmt.Errorf("apply table migration %s phase %d: %w", table.name, phase+1, err)
					}
				}
			}
		}
		if err := tx.Exec("INSERT INTO release_schema_migrations(name,checksum,duration_ms) VALUES (?,?,?)", tableBaselineName, plan.baselineChecksum, time.Since(started).Milliseconds()).Error; err != nil {
			return err
		}
	}
	return applyMigrationFiles(tx, plan.subsequent)
}

package postgres

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Version 50 remains an immutable compatibility identity for existing databases.
// The 53 numbered table files occupy versions 51–103; later files run normally.
const tableBaselineName = "000050_table_baseline.up.sql"
const firstTableVersion = 51

type tableMigration struct {
	name, table, checksum string
	phases                [3]string
}

type migrationPlan struct {
	tables           []tableMigration
	subsequent       []migration
	baselineChecksum string
}

var numberedMigrationName = regexp.MustCompile(`^([0-9]{6})_(.+)\.up\.sql$`)
var migrationPhases = []string{"structure", "constraints", "references"}

func loadMigrationPlan(dir string) (migrationPlan, error) {
	var plan migrationPlan
	paths, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return plan, err
	}
	hash := sha256.New()
	versions := make(map[int]bool)
	lastTableVersion := firstTableVersion + len(historicalTableNames) - 1
	for _, path := range paths {
		name := filepath.Base(path)
		if strings.HasSuffix(name, ".down.sql") {
			continue
		}
		match := numberedMigrationName.FindStringSubmatch(name)
		if match == nil {
			return plan, fmt.Errorf("migration %s must use NNNNNN_description.up.sql naming", name)
		}
		version, _ := strconv.Atoi(match[1])
		if version < firstTableVersion || versions[version] {
			return plan, fmt.Errorf("migration %s has a reserved or duplicate version", name)
		}
		versions[version] = true
		raw, err := os.ReadFile(path)
		if err != nil {
			return plan, fmt.Errorf("read migration %s: %w", name, err)
		}
		if strings.TrimSpace(string(raw)) == "" {
			return plan, fmt.Errorf("migration %s is empty", name)
		}
		if err := validateMigrationSQL(name, string(raw)); err != nil {
			return plan, err
		}
		if strings.Contains(string(raw), "-- migrate:upgrade") {
			return plan, fmt.Errorf("migration %s: future upgrades belong in separate numbered files", name)
		}
		if version <= lastTableVersion {
			expected := historicalTableNames[version-firstTableVersion]
			if match[2] != expected {
				return plan, fmt.Errorf("migration %s must define table %s at version %06d", name, expected, version)
			}
			file, err := parseTableMigration(name, string(raw))
			if err != nil {
				return plan, err
			}
			file.table = expected
			sum := sha256.Sum256(raw)
			file.checksum = fmt.Sprintf("%x", sum[:])
			plan.tables = append(plan.tables, file)
			// Keep the exact old aggregate despite renaming the physical files.
			_, _ = hash.Write([]byte(expected + ".sql\x00"))
			_, _ = hash.Write(raw)
		} else {
			sum := sha256.Sum256(raw)
			plan.subsequent = append(plan.subsequent, migration{name, fmt.Sprintf("%x", sum[:]), string(raw)})
		}
	}
	if len(plan.tables) != len(historicalTableNames) {
		return plan, fmt.Errorf("table baseline needs all %d numbered table files (%06d–%06d)", len(historicalTableNames), firstTableVersion, lastTableVersion)
	}
	plan.baselineChecksum = fmt.Sprintf("%x", hash.Sum(nil))
	return plan, nil
}

func parseTableMigration(name, sql string) (tableMigration, error) {
	file := tableMigration{name: name}
	if match := numberedMigrationName.FindStringSubmatch(name); match != nil {
		file.table = match[2]
	}
	previous := 0
	for index, phase := range migrationPhases {
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

func registeredTableCount(applied map[string]string, plan migrationPlan) int {
	count := 0
	for _, file := range plan.tables {
		if _, ok := applied[file.name]; ok {
			count++
		}
	}
	return count
}

func validateAppliedPlan(applied map[string]string, plan migrationPlan) error {
	if plan.baselineChecksum != historicalTableBaselineChecksum {
		return fmt.Errorf("migration %s checksum mismatch; preserve applied table files and add a new numbered migration", tableBaselineName)
	}
	known := map[string]string{tableBaselineName: plan.baselineChecksum}
	for _, file := range plan.tables {
		known[file.name] = file.checksum
	}
	for _, file := range plan.subsequent {
		known[file.name] = file.checksum
	}
	historical := 0
	for name, checksum := range applied {
		if _, ok := historicalMigrationChecksums[name]; ok {
			historical++
			continue
		}
		want, ok := known[name]
		if !ok {
			return fmt.Errorf("unknown migration %s; refusing to upgrade or run older code", name)
		}
		if checksum != want {
			return fmt.Errorf("migration %s checksum mismatch", name)
		}
	}
	_, baseline := applied[tableBaselineName]
	if err := validateHistoricalLedger(applied, historical > 0 || (!baseline && len(applied) > 0)); err != nil {
		return err
	}
	registered := registeredTableCount(applied, plan)
	if registered != 0 && registered != len(plan.tables) {
		return fmt.Errorf("incomplete table baseline registration; refusing to alter the database")
	}
	if registered > 0 && !baseline {
		return fmt.Errorf("table baseline marker is missing")
	}
	missing := registered != len(plan.tables)
	for _, file := range plan.subsequent {
		_, exists := applied[file.name]
		if exists && missing {
			return fmt.Errorf("migration %s is applied after a missing earlier version", file.name)
		}
		missing = missing || !exists
	}
	return nil
}

func verifyMigrationPlan(db *gorm.DB, plan migrationPlan) error {
	applied, err := appliedMigrationChecksums(db)
	if err != nil {
		return err
	}
	if err := validateAppliedPlan(applied, plan); err != nil {
		return err
	}
	if _, exists := applied[tableBaselineName]; !exists {
		return fmt.Errorf("table baseline is pending; run explicit migration job")
	}
	if registeredTableCount(applied, plan) != len(plan.tables) {
		return fmt.Errorf("table baseline registration is pending; run explicit migration job")
	}
	return verifyMigrations(db, plan.subsequent)
}

func applyMigrationPlan(tx *gorm.DB, plan migrationPlan) error {
	applied, err := appliedMigrationChecksums(tx)
	if err != nil {
		return err
	}
	if err := validateAppliedPlan(applied, plan); err != nil {
		return err
	}
	if registeredTableCount(applied, plan) == 0 {
		started := time.Now()
		if len(applied) > 0 {
			// Adopt only verified complete schemas; no historical DDL or seeds replay.
			names := make([]string, 0, len(plan.tables))
			for _, table := range plan.tables {
				names = append(names, table.table)
			}
			if err := validateHistoricalSchema(tx, names); err != nil {
				return err
			}
		} else {
			for _, table := range plan.tables {
				if tx.Migrator().HasTable(table.table) {
					return fmt.Errorf("unversioned existing table %s; refusing to recreate data", table.table)
				}
			}
			// Bootstrap needs phases across all files because some FKs are cyclic.
			for phase := 0; phase < 3; phase++ {
				for _, table := range plan.tables {
					if strings.TrimSpace(table.phases[phase]) == "" {
						continue
					}
					if err := tx.Exec(table.phases[phase]).Error; err != nil {
						return fmt.Errorf("apply table migration %s phase %s: %w", table.name, migrationPhases[phase], err)
					}
				}
			}
		}
		if _, exists := applied[tableBaselineName]; !exists {
			if err := tx.Exec("INSERT INTO release_schema_migrations(name,checksum,duration_ms) VALUES (?,?,?)", tableBaselineName, plan.baselineChecksum, time.Since(started).Milliseconds()).Error; err != nil {
				return err
			}
		}
		for _, table := range plan.tables {
			if err := tx.Exec("INSERT INTO release_schema_migrations(name,checksum,duration_ms) VALUES (?,?,0)", table.name, table.checksum).Error; err != nil {
				return err
			}
		}
	}
	return applyMigrationFiles(tx, plan.subsequent)
}

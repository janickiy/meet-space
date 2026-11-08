package postgres

import (
	"strings"
	"testing"
)

func TestMigrationSQLRejectsTransactionEscape(t *testing.T) {
	for _, sql := range []string{
		"BEGIN; SELECT 1; COMMIT;",
		"START TRANSACTION READ WRITE;",
		"CREATE TABLE probe(id integer); COMMIT; SELECT 1/0;",
		"SELECT 1; END WORK AND CHAIN;",
		"\vCOMMIT;",
		"ROLLBACK;",
		"ABORT WORK;",
		"SAVEPOINT internal;",
		"RELEASE SAVEPOINT internal;",
		"PREPARE TRANSACTION 'migration';",
		"COMMIT PREPARED 'migration';",
		"ROLLBACK PREPARED 'migration';",
		"; -- leading comment\n /* a /* nested */ comment */ bEgIn;",
		"PREPARE /* nested /* one */ two */ TRANSACTION 'migration';",
		"SELECT 'it''s; BEGIN'; /* not a transaction */ cOmMiT;",
		`SELECT E'it\'s; COMMIT'; ROLLBACK;`,
		`SELECT E'escaped\\'; END;`,
		`SELECT "BEGIN; ""COMMIT"""; ABORT;`,
		`SELECT $body$ COMMIT; '"; $body$; SAVEPOINT x;`,
		`SELECT $1; COMMIT;`,
		`SELECT identifier$tag$; COMMIT;`,
		`SET standard_conforming_strings=off;`,
		`SET LOCAL standard_conforming_strings TO ON;`,
		`SET SESSION "standard_conforming_strings" TO 'off';`,
		`SET U&"\0073tandard_conforming_strings" TO 'off';`,
		`RESET U&"\0073tandard_conforming_strings";`,
		`RESET standard_conforming_strings;`,
		`RESET "standard_conforming_strings";`,
		`RESET ALL;`,
	} {
		t.Run(sql, func(t *testing.T) {
			err := validateMigrationSQL("000104_probe.up.sql", sql)
			if err == nil || !strings.Contains(err.Error(), "000104_probe.up.sql") {
				t.Fatalf("unsafe transaction script accepted or filename omitted: %v", err)
			}
		})
	}
}

func TestMigrationSQLAllowsQuotedBodiesAndNormalStatements(t *testing.T) {
	for _, sql := range []string{
		"",
		"-- COMMIT\r\nSELECT 1; /* BEGIN; /* ROLLBACK */ END; */ SELECT 2;",
		"ALTER TABLE users ADD COLUMN migration_probe integer;",
		`CREATE FUNCTION migration_probe() RETURNS integer LANGUAGE plpgsql AS $$BEGIN RETURN 42; END;$$;`,
		`DO $block_12$ BEGIN PERFORM 'COMMIT;'; END; $block_12$;`,
		`DO $outer$ BEGIN PERFORM '$inner$ ROLLBACK; $inner$'; END; $outer$;`,
		`SELECT 'it''s; BEGIN; COMMIT';`,
		`SELECT E'it\'s; COMMIT;';`,
		`SELECT e'backslash\\; ROLLBACK';`,
		`SELECT "COMMIT; ""ROLLBACK""";`,
		`SELECT U&"\0043OMMIT";`,
		`SELECT U&'COMMIT;';`,
		`SELECT $1, $2;`,
		`SELECT identifier$tag$, another$$identifier;`,
		`SELECT $тело$ COMMIT; $тело$;`,
		`PREPARE migration_probe(integer) AS SELECT $1;`,
		`SET LOCAL lock_timeout = '15s';`,
		`SELECT 'standard_conforming_strings'; RESET application_name;`,
	} {
		t.Run(sql, func(t *testing.T) {
			if err := validateMigrationSQL("000104_probe.up.sql", sql); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMigrationSQLRejectsUnterminatedQuotesAndComments(t *testing.T) {
	for _, sql := range []string{"SELECT 'open", `SELECT E'open\`, `SELECT "open`, "SELECT $tag$open", "/* nested /* closed */ open"} {
		if err := validateMigrationSQL("000104_probe.up.sql", sql); err == nil {
			t.Errorf("accepted unterminated SQL syntax: %q", sql)
		}
	}
}

package postgres

import (
	"fmt"
	"strings"
)

// Validate trusted migration scripts before sending any statement to PostgreSQL.
// The caller pins standard_conforming_strings=on; this is not a SQL sandbox.
func validateMigrationSQL(name, sql string) error {
	var first []string
	collect := true
	for i := 0; i < len(sql); {
		c := sql[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v':
			i++
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			for i < len(sql) && sql[i] != '\n' && sql[i] != '\r' {
				i++
			}
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			i += 2
			depth := 1
			for i < len(sql) && depth > 0 {
				if i+1 < len(sql) && sql[i:i+2] == "/*" {
					depth++
					i += 2
				} else if i+1 < len(sql) && sql[i:i+2] == "*/" {
					depth--
					i += 2
				} else {
					i++
				}
			}
			if depth != 0 {
				return fmt.Errorf("migration %s has an unterminated block comment", name)
			}
		case (c == 'U' || c == 'u') && i+2 < len(sql) && sql[i+1:i+3] == `&"` && collect && len(first) > 0 && (first[0] == "SET" || first[0] == "RESET"):
			return fmt.Errorf("migration %s uses an unsupported Unicode-escaped setting name; use a plain setting identifier", name)
		case c == '\'' || ((c == 'E' || c == 'e') && i+1 < len(sql) && sql[i+1] == '\''):
			escape := c != '\''
			if escape {
				i++
			}
			end, err := migrationQuotedEnd(sql, i, '\'', escape)
			if err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
			i, collect = end, false
		case c == '"':
			end, err := migrationQuotedEnd(sql, i, '"', false)
			if err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
			if collect && len(first) > 0 && (first[0] == "SET" || first[0] == "RESET") {
				first = append(first, strings.ToUpper(strings.ReplaceAll(sql[i+1:end-1], `""`, `"`)))
				if err := migrationStatementGuard(name, first); err != nil {
					return err
				}
			} else {
				collect = false
			}
			i = end
		case c == '$':
			delimiter := migrationDollarDelimiter(sql[i:])
			if delimiter == "" {
				i++ // A positional parameter such as $1 is not a dollar quote.
			} else {
				end := strings.Index(sql[i+len(delimiter):], delimiter)
				if end < 0 {
					return fmt.Errorf("migration %s has an unterminated dollar quote", name)
				}
				i += len(delimiter)*2 + end
			}
			collect = false
		case c == ';':
			first, collect = nil, true
			i++
		case migrationIdentifierStart(c):
			start := i
			for i < len(sql) && (migrationIdentifierStart(sql[i]) || sql[i] >= '0' && sql[i] <= '9' || sql[i] == '$') {
				i++
			}
			if collect && len(first) < 3 {
				first = append(first, strings.ToUpper(sql[start:i]))
				if err := migrationStatementGuard(name, first); err != nil {
					return err
				}
			}
		default:
			collect = false
			i++
		}
	}
	return nil
}

func migrationStatementGuard(name string, words []string) error {
	if len(words) == 0 {
		return nil
	}
	unsafe := false
	switch words[0] {
	case "BEGIN", "COMMIT", "END", "ROLLBACK", "ABORT", "SAVEPOINT", "RELEASE":
		unsafe = true
	case "START", "PREPARE":
		unsafe = len(words) > 1 && words[1] == "TRANSACTION"
	case "SET":
		parameter := 1
		if len(words) > 1 && (words[1] == "LOCAL" || words[1] == "SESSION") {
			parameter++
		}
		unsafe = len(words) > parameter && words[parameter] == "STANDARD_CONFORMING_STRINGS"
	case "RESET":
		unsafe = len(words) > 1 && (words[1] == "ALL" || words[1] == "STANDARD_CONFORMING_STRINGS")
	}
	if unsafe {
		return fmt.Errorf("migration %s contains transaction control or changes SQL string parsing; use the migrator's transaction", name)
	}
	return nil
}

func migrationIdentifierStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 128
}

func migrationDollarDelimiter(sql string) string {
	if len(sql) > 1 && sql[1] == '$' {
		return "$$"
	}
	if len(sql) < 3 || !migrationIdentifierStart(sql[1]) {
		return ""
	}
	for i := 2; i < len(sql); i++ {
		if sql[i] == '$' {
			return sql[:i+1]
		}
		if !migrationIdentifierStart(sql[i]) && !(sql[i] >= '0' && sql[i] <= '9') {
			return ""
		}
	}
	return ""
}

func migrationQuotedEnd(sql string, start int, quote byte, escape bool) (int, error) {
	for i := start + 1; i < len(sql); i++ {
		if escape && sql[i] == '\\' {
			i++
		} else if sql[i] == quote {
			if i+1 < len(sql) && sql[i+1] == quote {
				i++
			} else {
				return i + 1, nil
			}
		}
	}
	return 0, fmt.Errorf("unterminated SQL quote")
}

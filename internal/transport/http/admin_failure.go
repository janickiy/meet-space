package httptransport

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"strings"
)

// adminDependencyUnavailable classifies only raw transport/availability failures.
// SQL syntax, integrity errors and application bugs must remain internal errors.
func adminDependencyUnavailable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, sql.ErrConnDone) || errors.Is(err, driver.ErrBadConn) {
		return true
	}
	var networkError *net.OpError
	if errors.As(err, &networkError) {
		return true
	}
	// pgconn resolves hosts before dialing, so DNS failures need not have a
	// net.OpError wrapper (for example when a stopped container loses its alias).
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return true
	}
	var databaseError interface{ SQLState() string }
	if errors.As(err, &databaseError) {
		state := databaseError.SQLState()
		if len(state) == 5 && strings.HasPrefix(state, "08") {
			return true
		}
		switch state {
		case "57P01", "57P02", "57P03", "53300":
			return true
		}
	}
	return false
}

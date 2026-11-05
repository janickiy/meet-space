package httptransport

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

func TestAdminFailureDependencyClassification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, err := pgconn.ParseConfig("host=127.0.0.1 port=1 user=audit dbname=audit sslmode=disable")
	if err != nil {
		t.Fatal("synthetic pgconn configuration invalid")
	}
	// Exercise the real pgconn wrapper without contacting any network address.
	cfg.DialFunc = func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	}
	_, connectionErr := pgconn.ConnectConfig(context.Background(), cfg)
	var realConnectError *pgconn.ConnectError
	if !errors.As(connectionErr, &realConnectError) {
		t.Fatal("fixture did not produce pgconn.ConnectError")
	}
	dnsConfig, err := pgconn.ParseConfig("host=postgres.invalid user=audit dbname=audit sslmode=disable")
	if err != nil {
		t.Fatal("synthetic DNS configuration invalid")
	}
	dnsConfig.LookupFunc = func(context.Context, string) ([]string, error) {
		return nil, &net.DNSError{Name: "postgres.invalid", Err: "no such host", IsNotFound: true}
	}
	_, dnsErr := pgconn.ConnectConfig(context.Background(), dnsConfig)
	var realDNSError *net.DNSError
	if !errors.As(dnsErr, &realDNSError) || !errors.As(dnsErr, &realConnectError) {
		t.Fatal("fixture did not produce pgconn DNS connection failure")
	}
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"real-pgconn-connect", connectionErr, 503},
		{"real-pgconn-dns", dnsErr, 503},
		{"wrapped-network", fmt.Errorf("private dependency address: %w", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}), 503},
		{"connection-closed", sql.ErrConnDone, 503},
		{"bad-connection", driver.ErrBadConn, 503},
		{"deadline", fmt.Errorf("private SQL query: %w", context.DeadlineExceeded), 503},
		{"sqlstate-connection", &pgconn.PgError{Code: "08006", Message: "private SQL detail"}, 503},
		{"sqlstate-establishment", &pgconn.PgError{Code: "08001", Message: "private SQL detail"}, 503},
		{"sqlstate-shutdown", &pgconn.PgError{Code: "57P01", Message: "private SQL detail"}, 503},
		{"sqlstate-crash", &pgconn.PgError{Code: "57P02", Message: "private SQL detail"}, 503},
		{"sqlstate-startup", &pgconn.PgError{Code: "57P03", Message: "private SQL detail"}, 503},
		{"sqlstate-capacity", &pgconn.PgError{Code: "53300", Message: "private SQL detail"}, 503},
		{"sqlstate-syntax", &pgconn.PgError{Code: "42601", Message: "private SQL detail"}, 500},
		{"sqlstate-constraint", &pgconn.PgError{Code: "23514", Message: "private SQL detail"}, 500},
		{"sqlstate-unique-unmapped", &pgconn.PgError{Code: "23505", Message: "private SQL detail"}, 500},
		{"transaction-closed", sql.ErrTxDone, 500},
		{"client-cancelled", context.Canceled, 500},
		{"unknown", errors.New("private implementation error"), 500},
		{"explicit-internal", apperrors.Wrap(apperrors.ErrInternal, context.DeadlineExceeded, "private implementation error"), 500},
		{"explicit-conflict", apperrors.Wrap(apperrors.ErrConflict, driver.ErrBadConn, "Конфликт"), 409},
		{"explicit-unauthorized", apperrors.ErrUnauthorized, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/failure", func(c *gin.Context) { adminFailure(c, tc.err) })
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("GET", "/failure", nil))
			if rec.Code != tc.status {
				t.Fatalf("status=%d expected=%d", rec.Code, tc.status)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tc.status == 503 && body["error"] != "unavailable" {
				t.Fatal("dependency failure did not use unavailable code")
			}
			if tc.status == 500 && body["error"] != "internal_error" {
				t.Fatal("internal failure category changed")
			}
			if strings.Contains(rec.Body.String(), "private") || strings.Contains(rec.Body.String(), "127.0.0.1") {
				t.Fatal("dependency details leaked")
			}
		})
	}
}

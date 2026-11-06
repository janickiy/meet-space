// meetspace-admin only accepts secrets on standard input. It is never an HTTP endpoint.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/admin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	postgresinfra "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	adminusecase "github.com/janickiy/meet-space/internal/usecase/admin"
)

func run(args []string, in io.Reader, out io.Writer) error {
	if len(args) != 1 || (args[0] != "status" && args[0] != "bootstrap" && args[0] != "reset-password") {
		return errors.New("usage: meetspace-admin status|bootstrap|reset-password; credentials are JSON on stdin")
	}
	cfg, err := config.Load()
	if err != nil {
		return errors.New("invalid application configuration")
	}
	db, err := postgresinfra.Connect(cfg.PostgresDSN)
	if err != nil {
		return errors.New("database unavailable")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return errors.New("database unavailable")
	}
	defer sqlDB.Close()
	if args[0] == "bootstrap" {
		if err = postgresinfra.RunMigrations(db, "database/migrations"); err != nil {
			return errors.New("administrative bootstrap migration failed")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repo := postgresinfra.NewAdminRepository(db)
	if args[0] == "status" {
		a, e := repo.Status(ctx)
		if e != nil {
			return errors.New("administrative schema unavailable; apply migrations first")
		}
		v := map[string]any{"exists": a != nil}
		if a != nil {
			v["username"] = a.Username
		}
		return json.NewEncoder(out).Encode(v)
	}
	var input domain.CreateRequest
	decoder := json.NewDecoder(io.LimitReader(in, 4097))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&input); err != nil {
		return errors.New("invalid credential input")
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("expected one credential JSON object")
	}
	service, err := adminusecase.NewService(repo, security.PasswordHasher{})
	if err != nil {
		return errors.New("password service unavailable")
	}
	if args[0] == "bootstrap" {
		a, e := service.Create(ctx, input, domain.AuditInput{}, true)
		input.Password = ""
		if e != nil {
			return safeError(e)
		}
		return json.NewEncoder(out).Encode(map[string]any{"exists": true, "username": a.Username})
	}
	username, err := domain.NormalizeUsername(input.Username)
	if err != nil {
		return safeError(err)
	}
	a, err := repo.ByUsername(ctx, username)
	if err != nil {
		return errors.New("administrator not found")
	}
	err = service.Password(ctx, a.ID, "", input.Password, domain.AuditInput{AdministratorID: a.ID, Username: a.Username}, true)
	input.Password = ""
	if err != nil {
		return safeError(err)
	}
	return json.NewEncoder(out).Encode(map[string]any{"username": a.Username, "reset": true})
}
func safeError(err error) error {
	var e *apperrors.Error
	if errors.As(err, &e) {
		return errors.New(e.Message)
	}
	return errors.New("administrative operation failed")
}
func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

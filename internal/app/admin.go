package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/janickiy/meet-space/internal/config"
	postgresinfra "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	httptransport "github.com/janickiy/meet-space/internal/transport/http"
	adminusecase "github.com/janickiy/meet-space/internal/usecase/admin"
	"gorm.io/gorm"
)

// NewAdminServer is intentionally independent of the public router. Its port must not be published.
func NewAdminServer(cfg config.AdminConfig, db *gorm.DB, resources httptransport.AdminResources, onUserBlocked func(context.Context, string) error) (*http.Server, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	service, err := adminusecase.NewService(postgresinfra.NewAdminRepository(db), security.PasswordHasher{})
	if err != nil {
		return nil, err
	}
	router, err := httptransport.NewAdminRouter(service, cfg, resources, onUserBlocked)
	if err != nil {
		return nil, err
	}
	return &http.Server{Addr: fmt.Sprintf(":%d", cfg.HTTPPort), Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}, nil
}

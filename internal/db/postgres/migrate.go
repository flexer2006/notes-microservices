package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	// Импортируем драйвер для работы с Postgres.
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	// Импортируем драйвер для чтения миграций из файлов.
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
)

func MigrateDSN(ctx context.Context, dsn string, migrationsPath string) error {
	log := logger.Log(ctx)
	migrator, err := migrate.New(migrationsPath, dsn)
	if err != nil {
		log.Error(ctx, domain.LogErrCreateMigrationInstance, zap.Error(err), zap.String("path", migrationsPath))
		return fmt.Errorf("%s: %w", domain.LogErrCreateMigrationInstance, err)
	}
	defer func() {
		_, closeErr := migrator.Close()
		if closeErr != nil {
			log.Error(ctx, domain.LogErrFailedCloseMigrationInstance, zap.Error(closeErr))
		}
	}()
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Error(ctx, domain.LogErrApplyMigrations, zap.Error(err))
		return fmt.Errorf("%s: %w", domain.LogErrApplyMigrations, err)
	}
	log.Info(ctx, domain.LogMigrationsApplied)
	return nil
}

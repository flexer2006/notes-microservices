package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Database struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string, minConn, maxConn int) (*Database, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogConnecting)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Error(ctx, domain.LogErrParseConfig, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.LogErrParseConfig, err)
	}
	if minConn > 0 && minConn <= math.MaxInt32 {
		cfg.MinConns = int32(minConn)
	}
	if maxConn > 0 && maxConn <= math.MaxInt32 {
		cfg.MaxConns = int32(maxConn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Error(ctx, domain.LogErrCreatePool, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.LogErrCreatePool, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		log.Error(ctx, domain.LogErrPingDatabase, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.LogErrPingDatabase, err)
	}
	log.Info(ctx, domain.LogConnected)
	return &Database{pool: pool}, nil
}

func (db *Database) Pool() *pgxpool.Pool { return db.pool }
func (db *Database) Close(ctx context.Context) {
	logger.Log(ctx).Info(ctx, domain.LogClosing)
	db.pool.Close()
}

func Migrate(ctx context.Context, dsn, migrationsPath string) error {
	log := logger.Log(ctx)
	m, err := migrate.New(migrationsPath, dsn)
	if err != nil {
		log.Error(ctx, domain.LogErrCreateMigrationInstance, zap.Error(err), zap.String("path", migrationsPath))
		return fmt.Errorf("%s: %w", domain.LogErrCreateMigrationInstance, err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Error(ctx, domain.LogErrApplyMigrations, zap.Error(err))
		return fmt.Errorf("%s: %w", domain.LogErrApplyMigrations, err)
	}
	log.Info(ctx, domain.LogMigrationsApplied)
	return nil
}

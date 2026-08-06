package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres driver for golang-migrate.
	_ "github.com/golang-migrate/migrate/v4/source/file"       // file:// migrations source for golang-migrate.
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/logger"
)

type Database struct {
	pool *pgxpool.Pool
}

func NewDatabase(ctx context.Context, dsn string, minConn, maxConn int) (*Database, error) {
	log := logger.Log(ctx)
	log.Info(ctx, "connecting to Postgres database")

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}

	if minConn > 0 && minConn <= math.MaxInt32 {
		cfg.MinConns = int32(minConn)
	}

	if maxConn > 0 && maxConn <= math.MaxInt32 {
		cfg.MaxConns = int32(maxConn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}

	pingErr := pool.Ping(ctx)
	if pingErr != nil {
		pool.Close()

		return nil, fmt.Errorf("postgres: ping: %w", pingErr)
	}

	log.Info(ctx, "successfully connected to Postgres")

	db := new(Database)
	db.pool = pool

	return db, nil
}

func (db *Database) Pool() *pgxpool.Pool { return db.pool }

func (db *Database) Close(ctx context.Context) {
	logger.Log(ctx).Info(ctx, "closing Postgres connection pool")
	db.pool.Close()
}

func Migrate(ctx context.Context, dsn, migrationsPath string) error {
	log := logger.Log(ctx)

	migrator, err := migrate.New(migrationsPath, dsn)
	if err != nil {
		return fmt.Errorf("postgres: init migrations from %q: %w", migrationsPath, err)
	}

	defer func() { _, _ = migrator.Close() }()

	upErr := migrator.Up()
	if upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		log.Error(ctx, "migration failed", zap.Error(upErr))

		return fmt.Errorf("postgres: apply migrations: %w", upErr)
	}

	log.Info(ctx, "database migrations successfully applied")

	return nil
}

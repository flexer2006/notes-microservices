package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"
)

type Database struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string, minConn, maxConn int) (*Database, error) {
	log := logger.Log(ctx)
	log.Info(ctx, domain.LogConnecting)
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Error(ctx, domain.LogErrParseConfig, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.LogErrParseConfig, err)
	}
	if minConn > 0 && minConn <= (1<<31-1) {
		poolCfg.MinConns = int32(minConn)
	} else {
		log.Warn(ctx, domain.LogWarnMinConnOutOfRange)
	}
	if maxConn > 0 && maxConn <= (1<<31-1) {
		poolCfg.MaxConns = int32(maxConn)
	} else {
		log.Warn(ctx, domain.LogWarnMaxConnOutOfRange)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
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

func (db *Database) Pool() *pgxpool.Pool {
	return db.pool
}

func (db *Database) Close(ctx context.Context) {
	logger.Log(ctx).Info(ctx, domain.LogClosing)
	db.pool.Close()
}

func (db *Database) Ping(ctx context.Context) error {
	if err := db.pool.Ping(ctx); err != nil {
		return fmt.Errorf("%s: %w", domain.LogErrPingDatabase, err)
	}
	return nil
}

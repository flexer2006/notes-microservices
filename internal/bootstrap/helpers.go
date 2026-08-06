package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"go.uber.org/zap"

	postgresadapter "github.com/flexer2006/notes-microservices/internal/adapters/postgres"
	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/logger"
)

const minJWTSecretLength = 32

var errInvalidConfig = errors.New("invalid configuration")

func Wait(
	ctx context.Context,
	timeout time.Duration,
	serveErr <-chan error,
	hooks ...func(context.Context) error,
) error {
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var runErr error

	select {
	case <-sigCtx.Done():
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil {
			runErr = fmt.Errorf("server terminated unexpectedly: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	var shutdownErr error

	for _, hook := range hooks {
		err := hook(shutdownCtx)
		if err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}

	return errors.Join(runErr, shutdownErr)
}

func Load(ctx context.Context, configPath string) (*config.Config, error) {
	log := logger.Log(ctx)
	cfg := new(config.Config)

	if configPath != "" {
		fileInfo, statErr := os.Stat(configPath)
		if statErr == nil && !fileInfo.IsDir() {
			readErr := cleanenv.ReadConfig(configPath, cfg)
			if readErr != nil {
				return nil, fmt.Errorf("load config %q: %w", configPath, readErr)
			}
		}
	}

	err := cleanenv.ReadEnv(cfg)
	if err != nil {
		return nil, fmt.Errorf("load config from env: %w", err)
	}

	log.Info(ctx, "configuration loaded")

	return cfg, nil
}

func initLogger(cfg *config.Config) (*logger.Logger, error) {
	if cfg.Logging == nil {
		return nil, fmt.Errorf("%w: logging section is missing", errInvalidConfig)
	}

	logEnv := logger.Development
	if strings.EqualFold(cfg.Logging.Mode, "production") {
		logEnv = logger.Production
	}

	log, err := logger.NewLogger(logEnv, cfg.Logging.Level)
	if err != nil {
		return nil, fmt.Errorf("init logger: %w", err)
	}

	return log, nil
}

func initService(ctx context.Context, configPath string) (*config.Config, error) {
	cfg, err := Load(ctx, configPath)
	if err != nil {
		return nil, err
	}

	log, err := initLogger(cfg)
	if err != nil {
		return nil, err
	}

	logger.SetGlobalLogger(log)

	return cfg, nil
}

func ensurePostgresAndGRPC(cfg *config.Config) error {
	if cfg.Postgres == nil {
		return fmt.Errorf("%w: postgres section is required", errInvalidConfig)
	}

	if cfg.GRPC == nil {
		return fmt.Errorf("%w: grpc section is required", errInvalidConfig)
	}

	if cfg.Shutdown == nil {
		return fmt.Errorf("%w: shutdown section is required", errInvalidConfig)
	}

	if strings.TrimSpace(cfg.Postgres.Password) == "" {
		return fmt.Errorf(
			"%w: postgres password is required (set POSTGRES_PASSWORD)",
			errInvalidConfig,
		)
	}

	return nil
}

func ensureAuthJWT(cfg *config.Config) error {
	if cfg.JWT == nil {
		return fmt.Errorf("%w: jwt section is required", errInvalidConfig)
	}

	access := cfg.JWT.ResolvedAccessSecret()
	refresh := cfg.JWT.ResolvedRefreshSecret()

	if access == "" {
		return fmt.Errorf(
			"%w: jwt access secret is required (set JWT_ACCESS_SECRET_KEY)",
			errInvalidConfig,
		)
	}

	if refresh == "" {
		return fmt.Errorf(
			"%w: jwt refresh secret is required (set JWT_REFRESH_SECRET_KEY)",
			errInvalidConfig,
		)
	}

	if len(access) < minJWTSecretLength || len(refresh) < minJWTSecretLength {
		return fmt.Errorf("%w: jwt secrets must be at least 32 bytes", errInvalidConfig)
	}

	if access == refresh {
		return fmt.Errorf(
			"%w: jwt access and refresh secrets must differ",
			errInvalidConfig,
		)
	}

	return nil
}

func ensureNotesJWT(cfg *config.Config) error {
	if cfg.JWT == nil {
		return fmt.Errorf("%w: jwt section is required", errInvalidConfig)
	}

	access := cfg.JWT.ResolvedAccessSecret()
	if access == "" {
		return fmt.Errorf(
			"%w: jwt access secret is required (set JWT_ACCESS_SECRET_KEY)",
			errInvalidConfig,
		)
	}

	if len(access) < minJWTSecretLength {
		return fmt.Errorf("%w: jwt access secret must be at least 32 bytes", errInvalidConfig)
	}

	return nil
}

func requireGatewayConfig(cfg *config.Config) error {
	if cfg.GRPCClient == nil {
		return fmt.Errorf("%w: grpc client section is required", errInvalidConfig)
	}

	if cfg.Redis == nil {
		return fmt.Errorf("%w: redis section is required", errInvalidConfig)
	}

	if cfg.HTTP == nil {
		return fmt.Errorf("%w: http section is required", errInvalidConfig)
	}

	if cfg.Shutdown == nil {
		return fmt.Errorf("%w: shutdown section is required", errInvalidConfig)
	}

	return nil
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	parsed, parseErr := time.ParseDuration(raw)
	if parseErr == nil {
		return parsed
	}

	return fallback
}

func migrationSourceURL(service string) string {
	return "file://migrations/" + service
}

func postgresDSN(cfg *config.Config) string {
	hostPort := net.JoinHostPort(cfg.Postgres.Host, strconv.Itoa(cfg.Postgres.Port))
	userInfo := url.UserPassword(cfg.Postgres.User, cfg.Postgres.Password)

	sslMode := cfg.Postgres.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	return fmt.Sprintf(
		"postgres://%s@%s/%s?sslmode=%s",
		userInfo.String(),
		hostPort,
		url.PathEscape(cfg.Postgres.Database),
		url.QueryEscape(sslMode),
	)
}

func setupPostgres(
	ctx context.Context,
	cfg *config.Config,
	serviceName string,
) (*postgresadapter.Database, error) {
	cfgErr := ensurePostgresAndGRPC(cfg)
	if cfgErr != nil {
		return nil, cfgErr
	}

	migrateErr := postgresadapter.Migrate(
		ctx,
		postgresDSN(cfg),
		migrationSourceURL(serviceName),
	)
	if migrateErr != nil {
		return nil, migrateErr
	}

	database, err := postgresadapter.NewDatabase(
		ctx,
		postgresDSN(cfg),
		cfg.Postgres.MinConn,
		cfg.Postgres.MaxConn,
	)
	if err != nil {
		return nil, err
	}

	return database, nil
}

func serviceStartupLog(ctx context.Context, cfg *config.Config, serviceName string) {
	logger.Log(ctx).Info(ctx, serviceName+" service started",
		zap.String("log_level", cfg.Logging.Level),
		zap.String("startup_time", time.Now().Format(time.RFC3339)))
}

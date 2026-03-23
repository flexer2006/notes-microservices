package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	notesv1 "github.com/flexer2006/notes-microservices/gen/notes/v1"
	postgresadapter "github.com/flexer2006/notes-microservices/internal/adapters/postgres"
	"github.com/flexer2006/notes-microservices/internal/config"
	"github.com/flexer2006/notes-microservices/internal/domain"
	"github.com/flexer2006/notes-microservices/internal/logger"

	"github.com/ilyakaznacheev/cleanenv"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var (
	emailRegex          = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	passwordLetterRegex = regexp.MustCompile(`[a-zA-Z]`)
	passwordDigitRegex  = regexp.MustCompile(`\d`)
)

func Wait(ctx context.Context, timeout time.Duration, hooks ...func(context.Context) error) error {
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-sigCtx.Done():
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	g, gctx := errgroup.WithContext(shutdownCtx)
	for _, hook := range hooks {
		g.Go(func() error {
			return hook(gctx)
		})
	}
	return g.Wait()
}

func Load(ctx context.Context, configPath string) (*config.Config, error) {
	log := logger.Log(ctx)
	log.Info(ctx, "loading notes service configuration")
	cfg := new(config.Config)
	if configPath != "" {
		if fi, err := os.Stat(configPath); err == nil && !fi.IsDir() {
			if err := cleanenv.ReadConfig(configPath, cfg); err != nil {
				log.Error(ctx, domain.ErrFailedLoadConfig.Error(), zap.Error(err), zap.String("path", configPath))
				return nil, fmt.Errorf("%s: %w", domain.ErrFailedLoadConfig.Error(), err)
			}
		}
	}
	if err := cleanenv.ReadEnv(cfg); err != nil {
		log.Error(ctx, domain.ErrFailedLoadConfig.Error(), zap.Error(err))
		return nil, fmt.Errorf("%s: %w", domain.ErrFailedLoadConfig.Error(), err)
	}
	if loggable, ok := any(cfg).(interface{ LogFields() []zap.Field }); ok {
		log.Info(ctx, "configuration loaded", loggable.LogFields()...)
	} else {
		log.Info(ctx, "configuration loaded")
	}
	return cfg, nil
}

func appLog(ctx context.Context, method string) *logger.Logger {
	return logger.Method(ctx, method)
}

func wrapErr(ctx context.Context, domainErr, err error) error {
	if err == nil {
		return nil
	}
	logger.Log(ctx).Error(ctx, domainErr.Error(), zap.Error(err))
	return fmt.Errorf("%s: %w", domainErr, err)
}

func validateEmail(email string) error {
	if email == "" {
		return domain.ErrInvalidEmail
	}
	if !emailRegex.MatchString(email) {
		return domain.ErrInvalidEmail
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return domain.ErrPasswordTooShort
	}
	if !passwordLetterRegex.MatchString(password) || !passwordDigitRegex.MatchString(password) {
		return domain.ErrPasswordTooWeak
	}
	return nil
}

func hashToken(token string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(token))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func convertNoteFromProto(protoNote *notesv1.Note) *domain.Note {
	if protoNote == nil {
		return nil
	}
	return new(domain.Note{
		ID:        protoNote.NoteId,
		UserID:    protoNote.UserId,
		Title:     protoNote.Title,
		Content:   protoNote.Content,
		CreatedAt: protoNote.CreatedAt.AsTime(),
		UpdatedAt: protoNote.UpdatedAt.AsTime(),
	})
}

func initLogger(cfg *config.Config) (*logger.Logger, error) {
	if cfg.Logging == nil {
		return nil, fmt.Errorf("logging config is missing")
	}
	logEnv := logger.Development
	if strings.ToLower(cfg.Logging.Mode) == "production" {
		logEnv = logger.Production
	}
	return logger.NewLogger(logEnv, cfg.Logging.Level)
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

func ensureCommonConfig(cfg *config.Config) error {
	if cfg.Postgres == nil {
		return fmt.Errorf("%s: %w", domain.ErrMissingPostgresConfig.Error(), domain.ErrInvalidConfig)
	}
	if cfg.JWT == nil {
		return fmt.Errorf("%s: %w", domain.ErrMissingJWTConfig.Error(), domain.ErrInvalidConfig)
	}
	if cfg.GRPC == nil {
		return fmt.Errorf("%s: %w", domain.ErrMissingGRPCConfig.Error(), domain.ErrInvalidConfig)
	}
	if cfg.Shutdown == nil {
		return fmt.Errorf("%s: %w", domain.ErrMissingShutdownConfig.Error(), domain.ErrInvalidConfig)
	}
	return nil
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	return fallback
}

func migrationSourceURL(service string) string {
	return fmt.Sprintf("file://migrations/%s", service)
}

func postgresDSN(cfg *config.Config) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.Database)
}

func postgresDBConfig(cfg *config.Config) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.Database)
}

func setupPostgres(ctx context.Context, cfg *config.Config, serviceName string) (*postgresadapter.Database, error) {
	if err := ensureCommonConfig(cfg); err != nil {
		return nil, err
	}
	if err := postgresadapter.Migrate(ctx, postgresDSN(cfg), migrationSourceURL(serviceName)); err != nil {
		return nil, err
	}
	database, err := postgresadapter.NewDatabase(ctx, postgresDBConfig(cfg), cfg.Postgres.MinConn, cfg.Postgres.MaxConn)
	if err != nil {
		return nil, err
	}
	return database, nil
}

func serviceStartupLog(ctx context.Context, cfg *config.Config, serviceName string) {
	logger.Log(ctx).Info(ctx, fmt.Sprintf("%s service started", serviceName),
		zap.String("log_level", cfg.Logging.Level),
		zap.String("startup_time", time.Now().Format(time.RFC3339)))
}

func requireGatewayConfig(cfg *config.Config) error {
	if cfg.GRPCClient == nil {
		return fmt.Errorf("%s: %w", domain.ErrMissingGatewayGRPCConfig.Error(), domain.ErrInvalidConfig)
	}
	if cfg.Redis == nil {
		return fmt.Errorf("%s: %w", domain.ErrMissingGatewayRedisConfig.Error(), domain.ErrInvalidConfig)
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("%s: %w", domain.ErrMissingGatewayHTTPConfig.Error(), domain.ErrInvalidConfig)
	}
	if cfg.Shutdown == nil {
		return fmt.Errorf("%s: %w", domain.ErrMissingShutdownConfig.Error(), domain.ErrInvalidConfig)
	}
	return nil
}

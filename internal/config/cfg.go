package config

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/flexer2006/notes-microservices/internal/logger"
	"github.com/ilyakaznacheev/cleanenv"
	"go.uber.org/zap"
)

const (
	LogLoadingConfig    = "Loading notes service configuration"
	LogConfigLoaded     = "Configuration loaded successfully"
	ErrFailedLoadConfig = "Failed to load configuration"
)

type Config struct {
	Postgres   PostgresConfig   `yaml:"postgres"`
	Logging    LoggingConfig    `yaml:"logging"`
	Shutdown   ShutdownConfig   `yaml:"shutdown"`
	GRPC       GRPCConfig       `yaml:"grpc"`
	GRPCClient GRPCClientConfig `yaml:"grpc_client"`
	JWT        JWTConfig        `yaml:"jwt"`
	HTTP       HTTPConfig       `yaml:"http"`
	Redis      RedisConfig      `yaml:"redis"`
}

func Load(ctx context.Context) (*Config, error) {
	log := logger.Log(ctx)
	log.Info(ctx, LogLoadingConfig)
	var cfg Config
	err := cleanenv.ReadEnv(&cfg)
	if err != nil {
		log.Error(ctx, ErrFailedLoadConfig, zap.Error(err))
		return nil, fmt.Errorf("%s: %w", ErrFailedLoadConfig, err)
	}
	log.Info(ctx, LogConfigLoaded,
		zap.String("postgres_host", cfg.Postgres.Host),
		zap.Int("postgres_port", cfg.Postgres.Port),
		zap.String("log_level", cfg.Logging.Level),
		zap.String("log_mode", cfg.Logging.Mode),
		zap.Int("shutdown_timeout_seconds", cfg.Shutdown.Timeout),
		zap.Int("postgres_min_conn", cfg.Postgres.MinConn),
		zap.Int("postgres_max_conn", cfg.Postgres.MaxConn))
	return &cfg, nil
}

type PostgresConfig struct {
	Host     string `yaml:"host" env:"NOTES_POSTGRES_HOST" env-default:"0.0.0.0"`
	Port     int    `yaml:"port" env:"NOTES_POSTGRES_PORT" env-default:"5432"`
	User     string `yaml:"user" env:"NOTES_POSTGRES_USER" env-default:"postgres"`
	Password string `yaml:"password" env:"NOTES_POSTGRES_PASSWORD" env-default:"postgres"`
	Database string `yaml:"database" env:"NOTES_POSTGRES_DB" env-default:"notes"`
	MinConn  int    `yaml:"min_conn" env:"NOTES_POSTGRES_MIN_CONN" env-default:"1"`
	MaxConn  int    `yaml:"max_conn" env:"NOTES_POSTGRES_MAX_CONN" env-default:"10"`
}

func (p *PostgresConfig) GetDSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		p.Host, p.Port, p.User, p.Password, p.Database)
}

func (p *PostgresConfig) GetConnectionURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		p.User, p.Password, p.Host, p.Port, p.Database)
}

type GRPCConfig struct {
	Host string `yaml:"host" env:"NOTES_GRPC_HOST" env-default:"0.0.0.0"`
	Port int    `yaml:"port" env:"NOTES_GRPC_PORT" env-default:"50053"`
}

func (g *GRPCConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", g.Host, g.Port)
}

type JWTConfig struct {
	SecretKey       string `yaml:"secret_key" env:"JWT_SECRET_KEY" env-default:"2hlsdwbzmv7yGxbQ4sIah/MuvvNoe889pbEzZql0SU8n3U1gYi29gZnFQKxiUdGH"`
	AccessTokenTTL  string `yaml:"access_token_ttl" env:"JWT_ACCESS_TOKEN_TTL" env-default:"15m"`
	RefreshTokenTTL string `yaml:"refresh_token_ttl" env:"JWT_REFRESH_TOKEN_TTL" env-default:"24h"`
	BCryptCost      int    `yaml:"bcrypt_cost" env:"JWT_BCRYPT_COST" env-default:"10"`
}

func (c *JWTConfig) GetAccessTokenTTL() time.Duration {
	duration, err := time.ParseDuration(c.AccessTokenTTL)
	if err != nil {
		return 15 * time.Minute
	}
	return duration
}

func (c *JWTConfig) GetRefreshTokenTTL() time.Duration {
	duration, err := time.ParseDuration(c.RefreshTokenTTL)
	if err != nil {
		return 24 * time.Hour
	}
	return duration
}

type LoggingConfig struct {
	Level string `yaml:"level" env:"NOTES_LOGGER_LEVEL" env-default:"info"`
	Mode  string `yaml:"mode" env:"NOTES_LOGGER_MODE" env-default:"development"`
}

func (l *LoggingConfig) GetEnvironment() logger.Environment {
	if l.Mode == "production" {
		return logger.Production
	}
	return logger.Development
}

type ShutdownConfig struct {
	Timeout int `yaml:"timeout" env:"NOTES_GRACEFUL_SHUTDOWN_TIMEOUT" env-default:"5"`
}

func (s *ShutdownConfig) GetTimeout() time.Duration {
	return time.Duration(s.Timeout) * time.Second
}

type GRPCClientConfig struct {
	AuthService    GRPCServiceConfig `yaml:"auth_service" env-prefix:"GATEWAY_GRPC_AUTH_"`
	NotesService   GRPCServiceConfig `yaml:"notes_service" env-prefix:"GATEWAY_GRPC_NOTES_"`
	RequestTimeout time.Duration     `yaml:"request_timeout" env:"GATEWAY_GRPC_REQUEST_TIMEOUT" env-default:"5s"`
}

type GRPCServiceConfig struct {
	Host           string        `yaml:"host" env:"HOST" env-default:"0.0.0.0"`
	Port           int           `yaml:"port" env:"PORT" env-default:"50052"`
	ConnectTimeout time.Duration `yaml:"connect_timeout" env:"GATEWAY_TO_AUTHCONNECT_TIMEOUT" env-default:"5s"`
}

func (c *GRPCServiceConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type HTTPConfig struct {
	Host            string        `yaml:"host" env:"GATEWAY_HTTP_HOST" env-default:"0.0.0.0"`
	Port            int           `yaml:"port" env:"GATEWAY_HTTP_PORT" env-default:"8080"`
	ReadTimeout     time.Duration `yaml:"read_timeout" env:"GATEWAY_HTTP_READ_TIMEOUT" env-default:"5s"`
	WriteTimeout    time.Duration `yaml:"write_timeout" env:"GATEWAY_HTTP_WRITE_TIMEOUT" env-default:"10s"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"GATEWAY_HTTP_SHUTDOWN_TIMEOUT" env-default:"5s"`
}

func (c *HTTPConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type RedisConfig struct {
	Host            string        `yaml:"host" env:"GATEWAY_REDIS_HOST" env-default:"localhost"`
	Port            int           `yaml:"port" env:"GATEWAY_REDIS_PORT" env-default:"6379"`
	Password        string        `yaml:"password" env:"GATEWAY_REDIS_PASSWORD" env-default:""`
	DB              int           `yaml:"db" env:"GATEWAY_REDIS_DB" env-default:"0"`
	ConnectTimeout  time.Duration `yaml:"connect_timeout" env:"GATEWAY_REDIS_CONNECT_TIMEOUT" env-default:"5s"`
	ReadTimeout     time.Duration `yaml:"read_timeout" env:"GATEWAY_REDIS_READ_TIMEOUT" env-default:"3s"`
	WriteTimeout    time.Duration `yaml:"write_timeout" env:"GATEWAY_REDIS_WRITE_TIMEOUT" env-default:"3s"`
	PoolSize        int           `yaml:"pool_size" env:"GATEWAY_REDIS_POOL_SIZE" env-default:"10"`
	MinIdle         int           `yaml:"min_idle" env:"GATEWAY_REDIS_MIN_IDLE" env-default:"2"`
	IdleTimeout     time.Duration `yaml:"idle_timeout" env:"GATEWAY_REDIS_IDLE_TIMEOUT" env-default:"5m"`
	MaxConnLifetime time.Duration `yaml:"max_conn_lifetime" env:"GATEWAY_REDIS_MAX_CONN_LIFETIME" env-default:"1h"`
	DefaultTTL      time.Duration `yaml:"default_ttl" env:"GATEWAY_REDIS_DEFAULT_TTL" env-default:"15m"`
}

func (c *RedisConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c *RedisConfig) GetAddressString() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

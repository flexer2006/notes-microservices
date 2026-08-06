package config

import (
	"strings"
	"time"
)

type PostgresConfig struct {
	Host     string `env:"POSTGRES_HOST"     env-default:"localhost" yaml:"host"`
	User     string `env:"POSTGRES_USER"     env-default:"postgres"  yaml:"user"`
	Password string `env:"POSTGRES_PASSWORD"                         yaml:"password"`
	Database string `env:"POSTGRES_DB"       env-default:"notes"     yaml:"database"`
	SSLMode  string `env:"POSTGRES_SSLMODE"  env-default:"disable"   yaml:"ssl_mode"`
	Port     int    `env:"POSTGRES_PORT"     env-default:"5432"      yaml:"port"`
	MinConn  int    `env:"POSTGRES_MIN_CONN" env-default:"1"         yaml:"min_conn"`
	MaxConn  int    `env:"POSTGRES_MAX_CONN" env-default:"10"        yaml:"max_conn"`
}

type LoggingConfig struct {
	Level string `env:"LOGGER_LEVEL" env-default:"info"        yaml:"level"`
	Mode  string `env:"LOGGER_MODE"  env-default:"development" yaml:"mode"`
}

type ShutdownConfig struct {
	Timeout int `env:"GRACEFUL_SHUTDOWN_TIMEOUT" env-default:"5" yaml:"timeout"`
}

type GRPCConfig struct {
	Host       string `env:"GRPC_HOST"       env-default:"0.0.0.0" yaml:"host"`
	Port       int    `env:"GRPC_PORT"       env-default:"50053"   yaml:"port"`
	Reflection bool   `env:"GRPC_REFLECTION" env-default:"false"   yaml:"reflection"`
}

type GRPCServiceConfig struct {
	Host           string        `env:"HOST"            env-default:"localhost" yaml:"host"`
	Port           int           `env:"PORT"            env-default:"50052"     yaml:"port"`
	ConnectTimeout time.Duration `env:"CONNECT_TIMEOUT" env-default:"5s"        yaml:"connect_timeout"`
}

type GRPCClientConfig struct {
	AuthService    GRPCServiceConfig `env-prefix:"GRPC_AUTH_"  yaml:"auth_service"`
	NotesService   GRPCServiceConfig `env-prefix:"GRPC_NOTES_" yaml:"notes_service"`
	RequestTimeout time.Duration     `                         yaml:"request_timeout" env:"GRPC_REQUEST_TIMEOUT" env-default:"5s"`
	Insecure       bool              `                         yaml:"insecure"        env:"GRPC_CLIENT_INSECURE" env-default:"false"`
}

type JWTConfig struct {
	AccessSecretKey  string `env:"JWT_ACCESS_SECRET_KEY"  yaml:"access_secret_key"`
	RefreshSecretKey string `env:"JWT_REFRESH_SECRET_KEY" yaml:"refresh_secret_key"`
	SecretKey        string `env:"JWT_SECRET_KEY"         yaml:"secret_key"`
	AccessTokenTTL   string `env:"JWT_ACCESS_TOKEN_TTL"   yaml:"access_token_ttl"   env-default:"15m"`
	RefreshTokenTTL  string `env:"JWT_REFRESH_TOKEN_TTL"  yaml:"refresh_token_ttl"  env-default:"24h"`
	BCryptCost       int    `env:"JWT_BCRYPT_COST"        yaml:"bcrypt_cost"        env-default:"10"`
}

func (j *JWTConfig) ResolvedAccessSecret() string {
	if j == nil {
		return ""
	}

	if access := strings.TrimSpace(j.AccessSecretKey); access != "" {
		return access
	}

	return strings.TrimSpace(j.SecretKey)
}

func (j *JWTConfig) ResolvedRefreshSecret() string {
	if j == nil {
		return ""
	}

	if refresh := strings.TrimSpace(j.RefreshSecretKey); refresh != "" {
		return refresh
	}

	return strings.TrimSpace(j.SecretKey)
}

type HTTPConfig struct {
	Host                string        `env:"HTTP_HOST"                  env-default:"0.0.0.0" yaml:"host"`
	ReadTimeout         time.Duration `env:"HTTP_READ_TIMEOUT"          env-default:"5s"      yaml:"read_timeout"`
	WriteTimeout        time.Duration `env:"HTTP_WRITE_TIMEOUT"         env-default:"10s"     yaml:"write_timeout"`
	ShutdownTimeout     time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT"      env-default:"5s"      yaml:"shutdown_timeout"`
	Port                int           `env:"HTTP_PORT"                  env-default:"8080"    yaml:"port"`
	TrustProxy          bool          `env:"HTTP_TRUST_PROXY"           env-default:"false"   yaml:"trust_proxy"`
	TrustPrivateProxies bool          `env:"HTTP_TRUST_PRIVATE_PROXIES" env-default:"false"   yaml:"trust_private_proxies"`
}

type RedisConfig struct {
	Host            string        `env:"REDIS_HOST"              env-default:"localhost" yaml:"host"`
	Password        string        `env:"REDIS_PASSWORD"          env-default:""          yaml:"password"`
	ConnectTimeout  time.Duration `env:"REDIS_CONNECT_TIMEOUT"   env-default:"5s"        yaml:"connect_timeout"`
	ReadTimeout     time.Duration `env:"REDIS_READ_TIMEOUT"      env-default:"3s"        yaml:"read_timeout"`
	WriteTimeout    time.Duration `env:"REDIS_WRITE_TIMEOUT"     env-default:"3s"        yaml:"write_timeout"`
	IdleTimeout     time.Duration `env:"REDIS_IDLE_TIMEOUT"      env-default:"5m"        yaml:"idle_timeout"`
	MaxConnLifetime time.Duration `env:"REDIS_MAX_CONN_LIFETIME" env-default:"1h"        yaml:"max_conn_lifetime"`
	DefaultTTL      time.Duration `env:"REDIS_DEFAULT_TTL"       env-default:"15m"       yaml:"default_ttl"`
	Port            int           `env:"REDIS_PORT"              env-default:"6379"      yaml:"port"`
	DB              int           `env:"REDIS_DB"                env-default:"0"         yaml:"db"`
	PoolSize        int           `env:"REDIS_POOL_SIZE"         env-default:"10"        yaml:"pool_size"`
	MinIdle         int           `env:"REDIS_MIN_IDLE"          env-default:"2"         yaml:"min_idle"`
}

type Config struct {
	Postgres   *PostgresConfig   `yaml:"postgres"`
	Logging    *LoggingConfig    `yaml:"logging"`
	Shutdown   *ShutdownConfig   `yaml:"shutdown"`
	GRPC       *GRPCConfig       `yaml:"grpc"`
	GRPCClient *GRPCClientConfig `yaml:"grpc_client"`
	JWT        *JWTConfig        `yaml:"jwt"`
	HTTP       *HTTPConfig       `yaml:"http"`
	Redis      *RedisConfig      `yaml:"redis"`
}

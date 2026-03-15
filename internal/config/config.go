package config

import "time"

type Config struct {
	Postgres *struct {
		Host     string `yaml:"host" env:"POSTGRES_HOST" env-default:"0.0.0.0"`
		User     string `yaml:"user" env:"POSTGRES_USER" env-default:"postgres"`
		Password string `yaml:"password" env:"POSTGRES_PASSWORD" env-default:"postgres"`
		Database string `yaml:"database" env:"POSTGRES_DB" env-default:"notes"`
		Port     int    `yaml:"port" env:"POSTGRES_PORT" env-default:"5432"`
		MinConn  int    `yaml:"min_conn" env:"POSTGRES_MIN_CONN" env-default:"1"`
		MaxConn  int    `yaml:"max_conn" env:"POSTGRES_MAX_CONN" env-default:"10"`
	} `yaml:"postgres"`
	Logging *struct {
		Level string `yaml:"level" env:"LOGGER_LEVEL" env-default:"info"`
		Mode  string `yaml:"mode" env:"LOGGER_MODE" env-default:"development"`
	} `yaml:"logging"`
	Shutdown *struct {
		Timeout int `yaml:"timeout" env:"GRACEFUL_SHUTDOWN_TIMEOUT" env-default:"5"`
	} `yaml:"shutdown"`
	GRPC *struct {
		Host string `yaml:"host" env:"GRPC_HOST" env-default:"0.0.0.0"`
		Port int    `yaml:"port" env:"GRPC_PORT" env-default:"50053"`
	} `yaml:"grpc"`
	GRPCClient *struct {
		AuthService struct {
			Host           string        `yaml:"host" env:"HOST" env-default:"0.0.0.0"`
			Port           int           `yaml:"port" env:"PORT" env-default:"50052"`
			ConnectTimeout time.Duration `yaml:"connect_timeout" env:"CONNECT_TIMEOUT" env-default:"5s"`
		} `yaml:"auth_service" env-prefix:"GRPC_AUTH_"`
		NotesService struct {
			Host           string        `yaml:"host" env:"HOST" env-default:"0.0.0.0"`
			Port           int           `yaml:"port" env:"PORT" env-default:"50052"`
			ConnectTimeout time.Duration `yaml:"connect_timeout" env:"CONNECT_TIMEOUT" env-default:"5s"`
		} `yaml:"notes_service" env-prefix:"GRPC_NOTES_"`
		RequestTimeout time.Duration `yaml:"request_timeout" env:"GRPC_REQUEST_TIMEOUT" env-default:"5s"`
	} `yaml:"grpc_client"`
	JWT *struct {
		SecretKey       string `yaml:"secret_key" env:"JWT_SECRET_KEY" env-default:"2hlsdwbzmv7yGxbQ4sIah/MuvvNoe889pbEzZql0SU8n3U1gYi29gZnFQKxiUdGH"`
		AccessTokenTTL  string `yaml:"access_token_ttl" env:"JWT_ACCESS_TOKEN_TTL" env-default:"15m"`
		RefreshTokenTTL string `yaml:"refresh_token_ttl" env:"JWT_REFRESH_TOKEN_TTL" env-default:"24h"`
		BCryptCost      int    `yaml:"bcrypt_cost" env:"JWT_BCRYPT_COST" env-default:"10"`
	} `yaml:"jwt"`
	HTTP *struct {
		Host            string        `yaml:"host" env:"HTTP_HOST" env-default:"0.0.0.0"`
		Port            int           `yaml:"port" env:"HTTP_PORT" env-default:"8080"`
		ReadTimeout     time.Duration `yaml:"read_timeout" env:"HTTP_READ_TIMEOUT" env-default:"5s"`
		WriteTimeout    time.Duration `yaml:"write_timeout" env:"HTTP_WRITE_TIMEOUT" env-default:"10s"`
		ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"HTTP_SHUTDOWN_TIMEOUT" env-default:"5s"`
	} `yaml:"http"`
	Redis *struct {
		Host            string        `yaml:"host" env:"REDIS_HOST" env-default:"localhost"`
		Password        string        `yaml:"password" env:"REDIS_PASSWORD" env-default:""`
		Port            int           `yaml:"port" env:"REDIS_PORT" env-default:"6379"`
		DB              int           `yaml:"db" env:"REDIS_DB" env-default:"0"`
		ConnectTimeout  time.Duration `yaml:"connect_timeout" env:"REDIS_CONNECT_TIMEOUT" env-default:"5s"`
		ReadTimeout     time.Duration `yaml:"read_timeout" env:"REDIS_READ_TIMEOUT" env-default:"3s"`
		WriteTimeout    time.Duration `yaml:"write_timeout" env:"REDIS_WRITE_TIMEOUT" env-default:"3s"`
		PoolSize        int           `yaml:"pool_size" env:"REDIS_POOL_SIZE" env-default:"10"`
		MinIdle         int           `yaml:"min_idle" env:"REDIS_MIN_IDLE" env-default:"2"`
		IdleTimeout     time.Duration `yaml:"idle_timeout" env:"REDIS_IDLE_TIMEOUT" env-default:"5m"`
		MaxConnLifetime time.Duration `yaml:"max_conn_lifetime" env:"REDIS_MAX_CONN_LIFETIME" env-default:"1h"`
		DefaultTTL      time.Duration `yaml:"default_ttl" env:"REDIS_DEFAULT_TTL" env-default:"15m"`
	} `yaml:"redis"`
}

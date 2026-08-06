//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

const (
	PostgresImage = "postgres:18.4"
	RedisImage    = "redis:8.10.0-alpine"

	TestDBUser = "nm_test"
	TestDBPass = "nm_test_pass"
	TestDBName = "nm_test"

	TestRedisPassword = "nm_redis_test"

	containerStartTimeout = 2 * time.Minute
	poolMinConns          = 1
	poolMaxConns          = 4
	redisDialTimeout      = 5 * time.Second
	redisIOTimeout        = 3 * time.Second
	redisPoolSize         = 4
	redisMinIdle          = 1
	redisDefaultTTL       = time.Minute
)

func ModuleRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	dir := filepath.Dir(file)
	for {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}

		dir = parent
	}
}

func MigrationsFileURI(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(ModuleRoot(t), "migrations", name)

	return "file://" + path
}

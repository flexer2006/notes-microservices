//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	pgadapter "github.com/flexer2006/notes-microservices/internal/adapters/postgres"
)

func StartPostgres(t *testing.T, migrationsName string) *pgadapter.Database {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), containerStartTimeout)
	t.Cleanup(cancel)

	ctr, err := postgres.Run(
		ctx,
		PostgresImage,
		postgres.WithDatabase(TestDBName),
		postgres.WithUsername(TestDBUser),
		postgres.WithPassword(TestDBPass),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}

	testcontainers.CleanupContainer(t, ctr)

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres dsn: %v", err)
	}

	migErr := pgadapter.Migrate(ctx, dsn, MigrationsFileURI(t, migrationsName))
	if migErr != nil {
		t.Fatalf("migrate %s: %v", migrationsName, migErr)
	}

	db, err := pgadapter.NewDatabase(ctx, dsn, poolMinConns, poolMaxConns)
	if err != nil {
		t.Fatalf("new database: %v", err)
	}

	t.Cleanup(func() { db.Close(context.Background()) })

	return db
}

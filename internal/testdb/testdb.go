// Package testdb поднимает чистую схему в тестовой базе для интеграционных тестов.
// Тесты пропускаются, если TEST_DATABASE_URL не задан.
package testdb

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"sportslot/internal/repository/postgres"
	"sportslot/internal/seed"
	"sportslot/migrations"
)

var mu sync.Mutex

// New пересоздаёт схему, применяет миграции и загружает демо-партнёров.
func New(t *testing.T) *postgres.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	mu.Lock()
	t.Cleanup(mu.Unlock)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatal(err)
	}
	store := postgres.NewStore(pool)
	if _, err := seed.SyncDemo(ctx, store, time.Now()); err != nil {
		t.Fatal(err)
	}
	return store
}

// Exec — прямой SQL для подготовки особых случаев в тестах.
func Exec(t *testing.T, sql string, args ...interface{}) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

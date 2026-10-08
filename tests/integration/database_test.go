package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/internal/database"
	"github.com/petarnenov/bot-space/internal/httpserver"
	"github.com/petarnenov/bot-space/migrations"
)

func isolated(t *testing.T) (context.Context, string, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run real PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal("test database connection failed")
	}
	name := fmt.Sprintf("bot_space_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := admin.Exec(cleanup, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		if err != nil {
			t.Error("test database cleanup failed")
		}
		_ = admin.Close(cleanup)
	})
	url = replaceDatabase(t, url, name)
	pool, err := database.Open(ctx, url, 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, url, pool
}

func bundle(t *testing.T) []migrations.Migration {
	t.Helper()
	ms, err := migrations.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func TestMigrateConcurrentAndRepeat(t *testing.T) {
	ctx, url, pool := isolated(t)
	ms := bundle(t)
	if database.Ready(ctx, pool, ms) == nil {
		t.Fatal("empty schema ready")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- database.Migrate(ctx, url, ms) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Migrate(ctx, url, ms); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM public.mailbox_schema_migrations").Scan(&count); err != nil || count != len(ms) {
		t.Fatalf("ledger count %d: %v", count, err)
	}
	if err := database.Ready(ctx, pool, ms); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationRollback(t *testing.T) {
	ctx, url, pool := isolated(t)
	ms, err := migrations.Load(fstest.MapFS{
		"0001_good.sql": &fstest.MapFile{Data: []byte("CREATE TABLE stable (id integer);")},
		"0002_bad.sql":  &fstest.MapFile{Data: []byte("CREATE TABLE rolled_back (id integer); SELECT missing_column;")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if database.Migrate(ctx, url, ms) == nil {
		t.Fatal("bad SQL accepted")
	}
	var count int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM public.mailbox_schema_migrations").Scan(&count)
	if count != 1 {
		t.Fatal("failed version recorded")
	}
	var present bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('rolled_back') IS NOT NULL").Scan(&present); err != nil || present {
		t.Fatal("partial migration survived")
	}
	if err := pool.QueryRow(ctx, "SELECT to_regclass('stable') IS NOT NULL").Scan(&present); err != nil || !present {
		t.Fatal("earlier migration lost")
	}
}

func TestMigrationIntegrity(t *testing.T) {
	for _, kind := range []string{"checksum", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			ctx, url, pool := isolated(t)
			ms := bundle(t)
			if err := database.Migrate(ctx, url, ms); err != nil {
				t.Fatal(err)
			}
			if kind == "checksum" {
				_, _ = pool.Exec(ctx, "UPDATE public.mailbox_schema_migrations SET checksum=$1 WHERE version=1", "invalid")
			} else {
				_, _ = pool.Exec(ctx, "INSERT INTO public.mailbox_schema_migrations (version,checksum) VALUES (999,'unknown')")
			}
			if !errors.Is(database.Ready(ctx, pool, ms), database.ErrSchema) {
				t.Fatal("incompatible schema ready")
			}
			if !errors.Is(database.Migrate(ctx, url, ms), database.ErrSchema) {
				t.Fatal("incompatible migration allowed")
			}
		})
	}
}

func TestMigrationLockDeadline(t *testing.T) {
	ctx, url, pool := isolated(t)
	lock, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if _, err = lock.Exec(ctx, "SELECT pg_advisory_lock($1)", database.MigrationLockID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = lock.Exec(ctx, "SELECT pg_advisory_unlock($1)", database.MigrationLockID) }()
	short, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if database.Migrate(short, url, bundle(t)) == nil {
		t.Fatal("lock wait ignored")
	}
	if time.Since(start) > time.Second {
		t.Fatal("migration wait unbounded")
	}
	var present bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.mailbox_schema_migrations') IS NOT NULL").Scan(&present); err != nil || present {
		t.Fatal("migration changed schema without lock")
	}
	if database.MigrationTimeout != 5*time.Minute {
		t.Fatal("production deadline changed")
	}
}

func TestReadinessHTTPAndPoolExhaustion(t *testing.T) {
	ctx, url, pool := isolated(t)
	ms := bundle(t)
	s := httpserver.New(func(ctx context.Context) error { return database.Ready(ctx, pool, ms) }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(s.HTTP.Handler)
	defer srv.Close()
	check := func(path string, want int) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s = %d", path, resp.StatusCode)
		}
	}
	check("/healthz", 200)
	check("/readyz", 503)
	if err := database.Migrate(ctx, url, ms); err != nil {
		t.Fatal(err)
	}
	check("/readyz", 200)
	one, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Release()
	two, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer two.Release()
	start := time.Now()
	check("/readyz", 503)
	if time.Since(start) > 3*time.Second {
		t.Fatal("pool wait unbounded")
	}
	check("/healthz", 200)
}

func TestUnavailableDatabaseHTTP(t *testing.T) {
	ctx, url, pool := isolated(t)
	ms := bundle(t)
	if err := database.Migrate(ctx, url, ms); err != nil {
		t.Fatal(err)
	}
	pool.Close()
	s := httpserver.New(func(ctx context.Context) error { return database.Ready(ctx, pool, ms) }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, tc := range []struct {
		path   string
		status int
	}{{"/healthz", 200}, {"/readyz", 503}} {
		w := httptest.NewRecorder()
		s.HTTP.Handler.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Fatal("wrong database outage behavior")
		}
	}
}

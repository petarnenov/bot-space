// Package database manages bounded PostgreSQL connections and migration integrity.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petarnenov/bot-space/migrations"
)

const MigrationTimeout = 5 * time.Minute
const MigrationLockID int64 = 784329104125

var ErrUnavailable = errors.New("database unavailable")
var ErrSchema = errors.New("database schema is incompatible")

func Open(ctx context.Context, databaseURL string, maxConns int32) (*pgxpool.Pool, error) {
	if maxConns < 1 || maxConns > 1000 {
		return nil, errors.New("invalid database pool limit")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	cfg.MaxConns = maxConns
	cfg.ConnConfig.ConnectTimeout = 2 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, ErrUnavailable
	}
	return pool, nil
}

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func inspect(ctx context.Context, q querier, expected []migrations.Migration, requireAll bool) (map[int64]string, error) {
	rows, err := q.Query(ctx, "SELECT version, checksum FROM public.mailbox_schema_migrations ORDER BY version")
	if err != nil {
		return nil, ErrSchema
	}
	defer rows.Close()
	applied := make(map[int64]string)
	for rows.Next() {
		var version int64
		var checksum string
		if rows.Scan(&version, &checksum) != nil {
			return nil, ErrSchema
		}
		if version < 1 || version > int64(len(expected)) || expected[version-1].Version != version || expected[version-1].Checksum != checksum {
			return nil, fmt.Errorf("%w: migration %d", ErrSchema, version)
		}
		if version != int64(len(applied)+1) {
			return nil, ErrSchema
		}
		applied[version] = checksum
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	if requireAll && len(applied) != len(expected) {
		return nil, ErrSchema
	}
	return applied, nil
}

func Ready(ctx context.Context, pool *pgxpool.Pool, expected []migrations.Migration) error {
	if err := pool.Ping(ctx); err != nil {
		return ErrUnavailable
	}
	_, err := inspect(ctx, pool, expected, true)
	return err
}

func Migrate(ctx context.Context, databaseURL string, expected []migrations.Migration) error {
	ctx, cancel := context.WithTimeout(ctx, MigrationTimeout)
	defer cancel()
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	cfg.ConnectTimeout = 2 * time.Second
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	// A dedicated connection holds the session lock until it closes.
	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock($1)", MigrationLockID); err != nil {
		return errors.New("migration lock could not be acquired within the deadline")
	}
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.mailbox_schema_migrations (
		version bigint PRIMARY KEY, checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return ErrSchema
	}
	applied, err := inspect(ctx, conn, expected, false)
	if err != nil {
		return err
	}
	for _, m := range expected {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		if err := apply(ctx, conn, m); err != nil {
			return err
		}
	}
	return nil
}

func apply(ctx context.Context, conn *pgx.Conn, m migrations.Migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return fmt.Errorf("migration %d failed", m.Version)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO public.mailbox_schema_migrations (version, checksum) VALUES ($1, $2)", m.Version, m.Checksum); err != nil {
		return fmt.Errorf("migration %d could not be recorded", m.Version)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migration %d could not be committed", m.Version)
	}
	return nil
}

// Package database owns PostgreSQL connections, schema upgrades and readiness.
package database

import (
	"context"
	"errors"
	"net"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/tern/v2/migrate"
	"github.com/techize/selloovy/db/migrations"
)

var (
	ErrConfiguration = errors.New("invalid database configuration")
	ErrUnavailable   = errors.New("database is unavailable")
	ErrSchema        = errors.New("database schema is not compatible with this application")
	ErrMigration     = errors.New("database migration failed")
)

// Open creates a bounded pool. Connectivity is checked by readiness, not startup.
func Open(ctx context.Context, connectionURL string) (*pgxpool.Pool, error) {
	if !validURL(connectionURL) {
		return nil, ErrConfiguration
	}
	cfg, err := pgxpool.ParseConfig(connectionURL)
	if err != nil || !validTLS(connectionURL, cfg.ConnConfig.Host) {
		return nil, ErrConfiguration
	}
	cfg.MaxConns = 5
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, ErrConfiguration
	}
	return pool, nil
}

// Ready checks connectivity and the exact schema version without changing data.
func Ready(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return ErrUnavailable
	}
	var version int32
	if err := pool.QueryRow(ctx, "SELECT version FROM public.selloovy_schema_version").Scan(&version); err != nil {
		return ErrUnavailable
	}
	if version != migrations.Version {
		return ErrSchema
	}
	return nil
}

// Migrate applies embedded forward migrations with Tern's locking/transactions.
// It is invoked explicitly by an operator, never automatically by HTTP startup.
func Migrate(ctx context.Context, connectionURL string) error {
	if !validURL(connectionURL) {
		return ErrConfiguration
	}
	cfg, err := pgx.ParseConfig(connectionURL)
	if err != nil || !validTLS(connectionURL, cfg.Host) {
		return ErrConfiguration
	}
	cfg.ConnectTimeout = 3 * time.Second
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	migrator, err := migrate.NewMigrator(ctx, conn, "public.selloovy_schema_version")
	if err != nil {
		return ErrMigration
	}
	if err := migrator.LoadMigrations(migrations.Files); err != nil {
		return ErrMigration
	}
	if int32(len(migrator.Migrations)) != migrations.Version {
		return ErrSchema
	}
	current, err := migrator.GetCurrentVersion(ctx)
	if err != nil {
		return ErrMigration
	}
	if current > migrations.Version {
		return ErrSchema
	}
	if err := migrator.Migrate(ctx); err != nil {
		return ErrMigration
	}
	return nil
}

func validURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") && u.Hostname() != "" && u.User != nil && u.User.Username() != "" && len(u.Path) > 1
}

func validTLS(value, host string) bool {
	u, err := url.Parse(value)
	if err != nil {
		return false
	}
	mode := u.Query().Get("sslmode")
	if mode == "verify-full" {
		return true
	}
	// Unencrypted development connections must target an explicit loopback IP.
	ip := net.ParseIP(host)
	return mode == "disable" && ip != nil && ip.IsLoopback()
}

package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/techize/selloovy/internal/web"
)

func TestPostgresMigrationsAndReadiness(t *testing.T) {
	base := os.Getenv("SELLOOVY_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set SELLOOVY_TEST_DATABASE_URL for isolated PostgreSQL integration tests")
	}
	u, err := url.Parse(base)
	if err != nil || u.Path != "/selloovy_test" || u.Query().Has("dbname") {
		t.Fatal("integration tests require the dedicated selloovy_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal("could not connect to integration database")
	}
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer closeCancel()
		_ = admin.Close(closeCtx)
	})
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("could not generate test database name")
	}
	name := "selloovy_test_" + hex.EncodeToString(random[:])
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal("could not create isolated test database")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		// FORCE applies only to the unique database created by this test.
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Error("could not remove isolated test database")
		}
	})
	u.Path = "/" + name
	connectionURL := u.String()
	pool, err := Open(ctx, connectionURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if Ready(ctx, pool) == nil {
		t.Fatal("unmigrated database was ready")
	}
	if err := Migrate(ctx, connectionURL); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, connectionURL); err != nil {
		t.Fatal("repeat migration did not succeed")
	}
	if err := Ready(ctx, pool); err != nil {
		t.Fatal("migrated database not ready")
	}
	var id int64
	if err := pool.QueryRow(ctx, "INSERT INTO public.shops (name) VALUES ($1) RETURNING id", "Example Maker").Scan(&id); err != nil {
		t.Fatal("could not save synthetic shop")
	}
	pool.Close()
	pool, err = Open(ctx, connectionURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var shopName, currency string
	if err := pool.QueryRow(ctx, "SELECT name, currency_code FROM public.shops WHERE id = $1", id).Scan(&shopName, &currency); err != nil {
		t.Fatal("saved shop did not survive pool restart")
	}
	if shopName != "Example Maker" || currency != "GBP" {
		t.Fatal("saved shop values changed")
	}
	if _, err := pool.Exec(ctx, "UPDATE public.selloovy_schema_version SET version = 2"); err != nil {
		t.Fatal("could not set future schema fixture")
	}
	if !errors.Is(Ready(ctx, pool), ErrSchema) {
		t.Fatal("future schema incorrectly ready")
	}
	if !errors.Is(Migrate(ctx, connectionURL), ErrSchema) {
		t.Fatal("migration command accepted future schema")
	}
	var version int32
	if err := pool.QueryRow(ctx, "SELECT version FROM public.selloovy_schema_version").Scan(&version); err != nil || version != 2 {
		t.Fatal("future schema was changed")
	}
	if _, err := pool.Exec(ctx, "UPDATE public.selloovy_schema_version SET version = 1"); err != nil {
		t.Fatal("could not restore fixture version")
	}
	handler, err := web.NewHandler(func(ctx context.Context) error { return Ready(ctx, pool) })
	if err != nil {
		t.Fatal(err)
	}
	// Terminate only this test database's connections and disable reconnects.
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+quoted+" ALLOW_CONNECTIONS false"); err != nil {
		t.Fatal("could not disable fixture database connections")
	}
	if _, err := admin.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1", name); err != nil {
		t.Fatal("could not terminate fixture connections")
	}
	for _, tc := range []struct {
		path string
		code int
	}{{"/health/live", 200}, {"/health/ready", 503}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if response.Code != tc.code {
			t.Errorf("dependency outage: %s = %d, want %d", tc.path, response.Code, tc.code)
		}
	}
}

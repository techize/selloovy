package database

import (
	"context"
	"errors"
	"testing"
)

func TestConfigurationRejectedWithoutDisclosure(t *testing.T) {
	for _, input := range []string{
		"", "private-invalid-value", "postgres://selloovy@example.com/shop?sslmode=disable",
		"postgres://selloovy@127.0.0.1/shop?sslmode=prefer",
		"postgres://selloovy@127.0.0.1/shop?sslmode=disable&host=example.com",
	} {
		pool, err := Open(context.Background(), input)
		if pool != nil {
			pool.Close()
			t.Fatal("invalid input created a pool")
		}
		if !errors.Is(err, ErrConfiguration) || err.Error() != "invalid database configuration" {
			t.Fatal("configuration was not rejected with a redacted error")
		}
	}
}

func TestReadinessWithoutPool(t *testing.T) {
	if !errors.Is(Ready(context.Background(), nil), ErrUnavailable) {
		t.Fatal("missing pool must not report ready")
	}
}

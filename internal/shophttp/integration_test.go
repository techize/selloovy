package shophttp_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/database"
	"github.com/techize/selloovy/internal/shop"
	"github.com/techize/selloovy/internal/shophttp"
)

func fixtureDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("SELLOOVY_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("dedicated test database required")
	}
	u, err := url.Parse(base)
	if err != nil || u.Path != "/selloovy_test" || u.Query().Has("dbname") {
		t.Fatal("dedicated selloovy_test database required")
	}
	admin, err := pgx.Connect(t.Context(), base)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	var suffix [8]byte
	_, _ = rand.Read(suffix[:])
	name := "selloovy_test_shop_" + hex.EncodeToString(suffix[:])
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(t.Context(), "CREATE DATABASE "+quoted); err != nil {
		t.Fatal("fixture creation failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Error("fixture cleanup failed")
		}
		_ = admin.Close(ctx)
	})
	u.Path = "/" + name
	if err = database.Migrate(t.Context(), u.String()); err != nil {
		t.Fatal(err)
	}
	pool, err := database.Open(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
func TestOwnedShopSettingsPersistenceAndIsolation(t *testing.T) {
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, err := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = owners.CreateFirstOwner(ctx, "First fixture", "first@example.com", "fixture8")
	if err != nil {
		t.Fatal(err)
	}
	var secondID int64
	if err = pool.QueryRow(ctx, "INSERT INTO public.shops(name) VALUES('Second fixture') RETURNING id").Scan(&secondID); err != nil {
		t.Fatal("second shop fixture failed")
	}
	if _, _, err = owners.CreateOwner(ctx, secondID, "second@example.com", "fixture8"); err != nil {
		t.Fatal(err)
	}
	a, err := owners.Login(ctx, "first@example.com", "fixture8")
	if err != nil {
		t.Fatal(err)
	}
	b, err := owners.Login(ctx, "second@example.com", "fixture8")
	if err != nil {
		t.Fatal(err)
	}
	service := shop.NewStore(pool)
	guard, _ := authhttp.New(owners, "https://shop.example.com")
	handler := guard.Protect(shophttp.New(service))
	send := func(method, body, token, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://shop.example.com/shop", strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Selloovy-Request", "owner-auth")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "__Host-selloovy_session", Value: token})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := send("GET", "", "", "https://shop.example.com"); w.Code != 401 {
		t.Fatal("anonymous settings access allowed")
	}
	read := func(token string) shop.Settings {
		t.Helper()
		w := send("GET", "", token, "")
		if w.Code != 200 {
			t.Fatal("read failed")
		}
		var data shop.Settings
		if json.Unmarshal(w.Body.Bytes(), &data) != nil {
			t.Fatal("settings response invalid")
		}
		return data
	}
	first := read(a.Token.Reveal())
	second := read(b.Token.Reveal())
	if first.Name != "First fixture" || second.Name != "Second fixture" || first.Revision != 1 {
		t.Fatal("shop ownership/defaults mismatch")
	}
	input := shop.Input{Name: "  Updated maker  ", Tagline: "Made with care", Description: "First line\nSecond line", ContactEmail: "Contact@Example.com", Revision: 1}
	payload, _ := json.Marshal(input)
	if w := send("PUT", string(payload), a.Token.Reveal(), "https://other.example.com"); w.Code != 403 {
		t.Fatal("cross-origin write accepted")
	}
	if w := send("PUT", `{"name":"Other","shopId":2,"revision":1}`, a.Token.Reveal(), "https://shop.example.com"); w.Code != 400 {
		t.Fatal("client shop selector accepted")
	}
	if w := send("PUT", `{"name":"","revision":1}`, a.Token.Reveal(), "https://shop.example.com"); w.Code != 422 {
		t.Fatal("empty name accepted")
	}
	if w := send("PUT", strings.Repeat("x", 17*1024), a.Token.Reveal(), "https://shop.example.com"); w.Code != 400 {
		t.Fatal("oversized body accepted")
	}
	w := send("PUT", string(payload), a.Token.Reveal(), "https://shop.example.com")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private save failed")
	}
	saved := read(a.Token.Reveal())
	if saved.Name != "Updated maker" || saved.ContactEmail != "contact@example.com" || saved.Description != input.Description || saved.Revision != 2 {
		t.Fatal("saved values not retained")
	}
	if read(b.Token.Reveal()).Name != "Second fixture" {
		t.Fatal("save changed another merchant")
	}
	if w := send("PUT", string(payload), a.Token.Reveal(), "https://shop.example.com"); w.Code != 409 {
		t.Fatal("stale save overwrote data")
	}
	// Independent clients racing from the same revision must have exactly one winner.
	parallel, err := owners.Login(ctx, "first@example.com", "fixture8")
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, token := range []string{a.Token.Reveal(), parallel.Token.Reveal()} {
		wg.Go(func() {
			draft := shop.Input{Name: []string{"Race one", "Race two"}[i], Revision: 2}
			_, err := service.Save(ctx, token, draft)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, shop.ErrConflict) {
			conflict++
		} else {
			t.Fatal("unexpected race outcome")
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("lost-update protection failed")
	}
	if err = owners.Logout(ctx, a.Token.Reveal()); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Save(ctx, a.Token.Reveal(), shop.Input{Name: "Denied", Revision: 3}); !errors.Is(err, auth.ErrCredential) {
		t.Fatal("logged-out write accepted")
	}
	pool.Close()
	if w := send("GET", "", b.Token.Reveal(), ""); w.Code != 503 {
		t.Fatal("outage did not fail closed")
	}
}

package cataloghttp_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/cataloghttp"
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
	name := "selloovy_test_catalog_" + hex.EncodeToString(suffix[:])
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

func TestPrivateDraftOwnershipPersistenceAndRetries(t *testing.T) {
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = owners.CreateFirstOwner(ctx, "First fixture", "first@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	var shopID int64
	if pool.QueryRow(ctx, "INSERT INTO shops(name) VALUES('Second fixture') RETURNING id").Scan(&shopID) != nil {
		t.Fatal("fixture failed")
	}
	if _, _, e = owners.CreateOwner(ctx, shopID, "second@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	a, e := owners.Login(ctx, "first@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	b, e := owners.Login(ctx, "second@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	service := catalog.NewStore(pool)
	guard, _ := authhttp.New(owners, "https://shop.example.com")
	routes := chi.NewRouter()
	routes.Mount("/", shophttp.New(shop.NewStore(pool)))
	routes.Mount("/products", cataloghttp.New(service))
	handler := guard.Protect(routes)
	send := func(method, path, body, token, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://shop.example.com"+path, strings.NewReader(body))
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
	token := a.Token.Reveal()
	other := b.Token.Reveal()
	origin := "https://shop.example.com"
	if send("GET", "/products/", "", "", "").Code != 401 {
		t.Fatal("anonymous access accepted")
	}
	if send("GET", "/shop", "", token, "").Code != 200 {
		t.Fatal("settings route broken")
	}
	in := catalog.Input{Name: "  Demo dragon  ", Description: "Line one\n<script>demo</script>", PricePence: 1999, CertificateName: "required", CreationKey: "00000000-0000-4000-8000-000000000001"}
	payload, _ := json.Marshal(in)
	if send("POST", "/products/", string(payload), token, "https://other.example.com").Code != 403 {
		t.Fatal("cross origin accepted")
	}
	if send("POST", "/products/", `{"shopId":2}`, token, origin).Code != 400 {
		t.Fatal("shop selector accepted")
	}
	if send("POST", "/products/", strings.Repeat("x", 33000), token, origin).Code != 400 {
		t.Fatal("oversize accepted")
	}
	invalid := in
	invalid.PricePence = 0
	bad, _ := json.Marshal(invalid)
	if send("POST", "/products/", string(bad), token, origin).Code != 422 {
		t.Fatal("invalid price accepted")
	}
	w := send("POST", "/products/", string(payload), token, origin)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("create failed")
	}
	var p catalog.Product
	if json.Unmarshal(w.Body.Bytes(), &p) != nil || p.ID < 1 || p.Name != "Demo dragon" || p.PricePence != 1999 {
		t.Fatal("create values wrong")
	}
	path := fmt.Sprintf("/products/%d", p.ID)
	if send("GET", path, "", other, "").Code != 404 {
		t.Fatal("other shop read allowed")
	}
	if send("PUT", path, `{"name":"Attack","pricePence":100,"certificateName":"none","revision":1}`, other, origin).Code != 404 {
		t.Fatal("other shop write allowed")
	}
	w = send("GET", "/products/", "", other, "")
	var page catalog.Page
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Products) != 0 {
		t.Fatal("list leaked another shop")
	}
	if send("GET", "/products/?shopId=1", "", token, "").Code != 400 {
		t.Fatal("list selector accepted")
	}
	if send("GET", "/products/?after=bad", "", token, "").Code != 400 {
		t.Fatal("invalid cursor accepted")
	}
	// A repeated create returns the same ID, even if independent callers race.
	var wg sync.WaitGroup
	ids := make(chan int64, 2)
	for range 2 {
		wg.Go(func() {
			v, e := service.Save(ctx, token, 0, in)
			if e != nil {
				ids <- 0
			} else {
				ids <- v.ID
			}
		})
	}
	wg.Wait()
	close(ids)
	for id := range ids {
		if id != p.ID {
			t.Fatal("create retry duplicated product")
		}
	}
	changed := in
	changed.PricePence = 2000
	if _, e = service.Save(ctx, token, 0, changed); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("reused create key changed product")
	}
	edit := catalog.Input{Name: "Updated dragon", Description: in.Description, PricePence: 2499, CertificateName: "optional", Revision: 1}
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, e := service.Save(ctx, token, p.ID, edit); results <- e })
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, catalog.ErrConflict) {
			conflict++
		} else {
			t.Fatal("unexpected edit failure")
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("stale edit protection failed")
	}
	saved, e := catalog.NewStore(pool).Read(ctx, token, p.ID)
	if e != nil || saved.Name != edit.Name || saved.PricePence != 2499 || saved.Revision != 2 || saved.Description != in.Description {
		t.Fatal("saved draft not retained")
	}
	// More than one bounded page must be traversable without overlapping IDs.
	for i := 2; i <= 53; i++ {
		draft := in
		draft.CreationKey = fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		if _, e = service.Save(ctx, token, 0, draft); e != nil {
			t.Fatal("pagination fixture failed")
		}
	}
	first, e := service.List(ctx, token, 0)
	if e != nil || len(first.Products) != 50 || first.NextAfter == 0 {
		t.Fatal("first page wrong")
	}
	last, e := service.List(ctx, token, first.NextAfter)
	if e != nil || len(last.Products) != 3 || last.NextAfter != 0 || last.Products[0].ID <= first.NextAfter {
		t.Fatal("next page wrong")
	}
	if e = owners.Logout(ctx, token); e != nil {
		t.Fatal(e)
	}
	if _, e = service.Save(ctx, token, p.ID, edit); !errors.Is(e, auth.ErrCredential) {
		t.Fatal("logged out edit accepted")
	}
	if _, e = service.List(ctx, token, 0); !errors.Is(e, auth.ErrCredential) {
		t.Fatal("logged out list accepted")
	}
	pool.Close()
	if send("GET", "/products/", "", other, "").Code != 503 {
		t.Fatal("outage failed open")
	}
}

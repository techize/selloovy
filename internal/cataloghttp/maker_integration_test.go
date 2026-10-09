package cataloghttp_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/cataloghttp"
)

func TestMakerStockIsolationConcurrencyAndRollback(t *testing.T) {
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = owners.CreateFirstOwner(ctx, "Demo maker", "first@example.com", "fixture8")
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
	token, other := a.Token.Reveal(), b.Token.Reveal()
	service := catalog.NewStore(pool)
	create := func(token, key, name string) catalog.Product {
		t.Helper()
		p, e := service.Save(ctx, token, 0, catalog.Input{Name: name, PricePence: 1999, CertificateName: "required", CreationKey: key})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	first := create(token, "00000000-0000-4000-8000-000000000001", "Demo dragon")
	second := create(other, "00000000-0000-4000-8000-000000000001", "Demo dinosaur")
	guard, _ := authhttp.New(owners, "https://shop.example.com")
	handler := guard.Protect(cataloghttp.New(service))
	path := fmt.Sprintf("/%d/maker", first.ID)
	origin := "https://shop.example.com"
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
	if send("GET", path, "", "", "").Code != 401 || send("GET", path, "", other, "").Code != 404 {
		t.Fatal("maker read isolation failed")
	}
	initial, e := service.ReadMaker(ctx, token, first.ID)
	if e != nil || initial.MadeToOrderFallback || len(initial.Variants) != 0 {
		t.Fatal("unsafe maker defaults")
	}
	premium := int64(2499)
	in := catalog.MakerInput{Revision: 1, MadeToOrderFallback: true, Variants: []catalog.VariantInput{{Label: "Amber / white", ColourPair: "Amber / white", SupplyMode: "stocked", StockQuantity: 7}, {Label: "Teal / copper", ColourPair: "Teal / copper", SupplyMode: "stocked", PricePence: &premium}, {Label: "Custom production", SizeLabel: "Standard", SupplyMode: "made_to_order"}}}
	body, _ := json.Marshal(in)
	if send("PUT", path, string(body), token, "https://other.example.com").Code != 403 {
		t.Fatal("cross origin stock write accepted")
	}
	if send("PUT", path, string(body), other, origin).Code != 404 {
		t.Fatal("foreign stock write accepted")
	}
	if send("PUT", path, `{"shopId":1}`, token, origin).Code != 400 {
		t.Fatal("shop selector accepted")
	}
	if send("PUT", path, strings.Repeat("x", 97000), token, origin).Code != 400 {
		t.Fatal("oversized maker request accepted")
	}
	w := send("PUT", path, string(body), token, origin)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("maker save failed")
	}
	var saved catalog.MakerSettings
	if json.Unmarshal(w.Body.Bytes(), &saved) != nil || saved.Revision != 2 || len(saved.Variants) != 3 {
		t.Fatal("maker response wrong")
	}
	if saved.Variants[0].StockQuantity != 7 || saved.Variants[0].Availability != "in_stock" || saved.Variants[0].EffectivePricePence != 1999 || saved.Variants[1].Availability != "made_to_order" || saved.Variants[1].EffectivePricePence != 2499 || saved.Variants[2].Availability != "made_to_order" {
		t.Fatal("variant price/stock/production rule mismatch")
	}
	if _, e = service.Save(ctx, token, first.ID, catalog.Input{Name: "Stale", PricePence: 100, CertificateName: "none", Revision: 1}); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("detail save ignored maker revision")
	}
	input := func(s catalog.MakerSettings) catalog.MakerInput {
		out := catalog.MakerInput{Revision: s.Revision, MadeToOrderFallback: s.MadeToOrderFallback}
		for _, v := range s.Variants {
			out.Variants = append(out.Variants, v.VariantInput)
		}
		return out
	}
	next := input(saved)
	next.MadeToOrderFallback = false
	next.Variants[0].StockQuantity = 6
	saved, e = service.SaveMaker(ctx, token, first.ID, next)
	if e != nil || saved.Variants[1].Availability != "unavailable" || saved.Variants[2].Availability != "made_to_order" {
		t.Fatal("fallback was not product controlled")
	}
	// Base-price changes update inherited prices while preserving explicit overrides.
	if _, e = service.Save(ctx, token, first.ID, catalog.Input{Name: "Demo dragon", PricePence: 2999, CertificateName: "required", Revision: saved.Revision}); e != nil {
		t.Fatal("base price update failed")
	}
	saved, e = service.ReadMaker(ctx, token, first.ID)
	if e != nil || saved.Variants[0].EffectivePricePence != 2999 || saved.Variants[1].EffectivePricePence != 2499 {
		t.Fatal("price inheritance lost explicit override")
	}
	// Missing saved variants cannot delete stock. Cross-product IDs cannot be injected.
	omitted := input(saved)
	omitted.Variants = omitted.Variants[:1]
	var validation *catalog.ValidationError
	if _, e = service.SaveMaker(ctx, token, first.ID, omitted); !errors.As(e, &validation) {
		t.Fatal("omitted stock was silently removed")
	}
	foreign, e := service.SaveMaker(ctx, other, second.ID, catalog.MakerInput{Revision: 1, Variants: []catalog.VariantInput{{Label: "Standard", SupplyMode: "stocked", StockQuantity: 1}}})
	if e != nil {
		t.Fatal(e)
	}
	injected := input(saved)
	injected.Variants[0].ID = foreign.Variants[0].ID
	if _, e = service.SaveMaker(ctx, token, first.ID, injected); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("foreign variant selector accepted")
	}
	// Rename swaps are atomic and must not hit an intermediate uniqueness failure.
	swapped := input(saved)
	swapped.Variants[0].Label, swapped.Variants[1].Label = swapped.Variants[1].Label, swapped.Variants[0].Label
	saved, e = service.SaveMaker(ctx, token, first.ID, swapped)
	if e != nil {
		t.Fatal("label swap failed")
	}
	var adjustments int
	if pool.QueryRow(ctx, "SELECT count(*) FROM stock_adjustments WHERE variant_id=$1", saved.Variants[0].ID).Scan(&adjustments) != nil || adjustments != 2 {
		t.Fatal("stock audit missing or duplicated")
	}
	// Force audit insertion failure: count, policy and parent revision must roll back together.
	if _, e = pool.Exec(ctx, `CREATE FUNCTION reject_fixture_adjustment() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture failure'; END $$; CREATE TRIGGER reject_fixture BEFORE INSERT ON stock_adjustments FOR EACH ROW EXECUTE FUNCTION reject_fixture_adjustment()`); e != nil {
		t.Fatal("failure fixture unavailable")
	}
	attempt := input(saved)
	attempt.MadeToOrderFallback = true
	attempt.Variants[0].StockQuantity = 10
	if _, e = service.SaveMaker(ctx, token, first.ID, attempt); !errors.Is(e, catalog.ErrStorage) {
		t.Fatal("induced adjustment failure not closed")
	}
	after, e := service.ReadMaker(ctx, token, first.ID)
	if e != nil || after.Revision != saved.Revision || after.MadeToOrderFallback || after.Variants[0].StockQuantity != 6 {
		t.Fatal("stock transaction partially committed")
	}
	if _, e = pool.Exec(ctx, "DROP TRIGGER reject_fixture ON stock_adjustments"); e != nil {
		t.Fatal("fixture trigger cleanup failed")
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			in := input(saved)
			in.Variants[0].StockQuantity = int64(12 + i)
			_, e := service.SaveMaker(ctx, token, first.ID, in)
			results <- e
		})
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
			t.Fatal("unexpected stock race failure")
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("concurrent count overwrite")
	}
	if e = owners.Logout(ctx, token); e != nil {
		t.Fatal(e)
	}
	if _, e = service.SaveMaker(ctx, token, first.ID, input(saved)); !errors.Is(e, auth.ErrCredential) {
		t.Fatal("logged out stock write accepted")
	}
	pool.Close()
	if send("GET", fmt.Sprintf("/%d/maker", second.ID), "", other, "").Code != 503 {
		t.Fatal("outage failed open")
	}
}

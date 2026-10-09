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
	"github.com/techize/selloovy/internal/storefronthttp"
	"github.com/techize/selloovy/internal/web"
)

func TestPublicationSnapshotIsolationAndPublicBrowsing(t *testing.T) {
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
	if pool.QueryRow(ctx, "INSERT INTO shops(name) VALUES('Second maker') RETURNING id").Scan(&shopID) != nil {
		t.Fatal("fixture failed")
	}
	if _, _, e = owners.CreateOwner(ctx, shopID, "second@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	a, _ := owners.Login(ctx, "first@example.com", "fixture8")
	b, _ := owners.Login(ctx, "second@example.com", "fixture8")
	token, other := a.Token.Reveal(), b.Token.Reveal()
	service := catalog.NewStore(pool)
	create := func(token, key, name string) catalog.Product {
		t.Helper()
		p, e := service.Save(ctx, token, 0, catalog.Input{Name: name, Description: "Story\n<script>alert(1)</script>", PricePence: 1999, CertificateName: "required", CreationKey: key})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	first := create(token, "00000000-0000-4000-8000-000000000001", "Demo dragon")
	draft := create(token, "00000000-0000-4000-8000-000000000002", "Private creation")
	second := create(other, "00000000-0000-4000-8000-000000000001", "Other creation")
	guard, _ := authhttp.New(owners, "https://shop.example.com")
	protected := guard.Protect(cataloghttp.New(service))
	send := func(method, path string, body any, token, origin string) *httptest.ResponseRecorder {
		t.Helper()
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "https://shop.example.com"+path, strings.NewReader(string(payload)))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Selloovy-Request", "owner-auth")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "__Host-selloovy_session", Value: token})
		}
		w := httptest.NewRecorder()
		protected.ServeHTTP(w, r)
		return w
	}
	path := fmt.Sprintf("/%d/publication", first.ID)
	origin := "https://shop.example.com"
	in := catalog.PublicationInput{ShopRevision: 1, Revision: 1, Publish: true}
	if send("GET", path, nil, "", "").Code != 401 || send("GET", path, nil, other, "").Code != 404 {
		t.Fatal("publication read boundary")
	}
	if send("PUT", path, in, token, "https://other.example.com").Code != 403 || send("PUT", path, in, other, origin).Code != 404 {
		t.Fatal("publication write boundary")
	}
	if send("PUT", path, map[string]any{"shopId": shopID}, token, origin).Code != 400 {
		t.Fatal("selector accepted")
	}
	if send("PUT", path, map[string]any{"revision": 1}, token, origin).Code != 400 || send("PUT", path, map[string]any{"revision": 1, "publish": nil}, token, origin).Code != 400 {
		t.Fatal("implicit publication action accepted")
	}
	if send("PUT", path, in, token, origin).Code != 422 {
		t.Fatal("published without variants")
	}
	unchanged, e := service.Read(ctx, token, first.ID)
	if e != nil || unchanged.Revision != 1 {
		t.Fatal("failed publication advanced revision")
	}
	premium := int64(2499)
	maker, e := service.SaveMaker(ctx, token, first.ID, catalog.MakerInput{Revision: 1, Variants: []catalog.VariantInput{
		{Label: "Amber / white", ColourPair: "Amber / white", SupplyMode: "stocked", StockQuantity: 3},
		{Label: "Teal / copper", ColourPair: "Teal / copper", SupplyMode: "stocked", PricePence: &premium},
		{Label: "Custom", SupplyMode: "made_to_order"},
	}})
	if e != nil {
		t.Fatal(e)
	}
	if send("PUT", path, in, token, origin).Code != 409 {
		t.Fatal("stale publish accepted")
	}
	in.Revision = maker.Revision
	w := send("PUT", path, in, token, origin)
	var pub catalog.Publication
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &pub) != nil || pub.Revision != 3 || pub.PublishedRevision != 3 {
		t.Fatal("publish failed")
	}
	key := strings.Split(pub.PublicPath, "/")[2]
	page, e := service.PublicList(ctx, key, 0)
	if e != nil || len(page.Products) != 1 || page.Products[0].ID != first.ID {
		t.Fatal("draft leaked into list")
	}
	_, p, e := service.PublicRead(ctx, key, first.ID)
	if e != nil || len(p.Variants) != 3 || p.Variants[0].Availability != "in_stock" || p.Variants[1].Availability != "unavailable" || p.Variants[1].PricePence != 2499 || p.Variants[2].Availability != "made_to_order" {
		t.Fatal("public prices/availability")
	}
	if _, _, e = service.PublicRead(ctx, key, draft.ID); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("draft publicly readable")
	}
	if _, _, e = service.PublicRead(ctx, key, second.ID); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("public shop crossed")
	}
	if _, e = service.PublicList(ctx, strings.Repeat("0", 32), 0); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("unknown shop revealed")
	}
	handler, e := web.NewHandler(nil, nil, nil, nil, storefronthttp.New(service))
	if e != nil {
		t.Fatal(e)
	}
	browse := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "https://shop.example.com"+path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w = browse(pub.PublicPath)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "£19.99") || !strings.Contains(w.Body.String(), "Dispatch within 2 working days") || strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") || strings.Contains(w.Body.String(), "first@example.com") || strings.Contains(w.Body.String(), "stockQuantity") {
		t.Fatal("unsafe or wrong public render")
	}
	if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatal("headers missing")
	}
	w = browse(fmt.Sprintf("%s?variant=%d", pub.PublicPath, p.Variants[1].ID))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<p class=\"price\">£24.99</p>") || !strings.Contains(w.Body.String(), "Currently unavailable") {
		t.Fatal("variant selector wrong")
	}
	if browse(pub.PublicPath+"?variant=999999").Code != 404 || browse(pub.PublicPath+"?shopId=2").Code != 404 || browse(pub.PublicPath+"?variant=1&variant=2").Code != 404 {
		t.Fatal("public selectors failed closed")
	}
	if browse("/shop/assets/storefront.css").Code != 200 {
		t.Fatal("stylesheet unavailable")
	}
	// Draft edits cannot rewrite reviewed prices or descriptions.
	edit, e := service.Save(ctx, token, first.ID, catalog.Input{Name: "Changed draft", Description: "Unreviewed story", PricePence: 2999, CertificateName: "optional", Revision: pub.Revision})
	if e != nil {
		t.Fatal(e)
	}
	_, p, e = service.PublicRead(ctx, key, first.ID)
	if e != nil || p.Name != "Demo dragon" || p.Variants[0].PricePence != 1999 || p.CertificateName != "required" {
		t.Fatal("draft edit reached public projection")
	}
	if _, e = service.SavePublication(ctx, token, first.ID, catalog.PublicationInput{ShopRevision: 1, Revision: pub.Revision, Publish: true}); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("stale revision republished")
	}
	maker, e = service.ReadMaker(ctx, token, first.ID)
	if e != nil {
		t.Fatal(e)
	}
	variants := make([]catalog.VariantInput, 0, len(maker.Variants)+1)
	for _, v := range maker.Variants {
		variants = append(variants, v.VariantInput)
	}
	variants[0].StockQuantity = 0
	variants = append(variants, catalog.VariantInput{Label: "Unreviewed option", SupplyMode: "made_to_order"})
	maker, e = service.SaveMaker(ctx, token, first.ID, catalog.MakerInput{Revision: edit.Revision, MadeToOrderFallback: true, Variants: variants})
	if e != nil {
		t.Fatal(e)
	}
	_, p, e = service.PublicRead(ctx, key, first.ID)
	if e != nil || len(p.Variants) != 3 || p.Variants[0].Availability != "made_to_order" || p.Variants[0].PricePence != 1999 {
		t.Fatal("live availability or snapshot boundary")
	}
	// Persisted snapshots survive reconstructing the service; racing publication actions conflict.
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, e := catalog.NewStore(pool).SavePublication(ctx, token, first.ID, catalog.PublicationInput{ShopRevision: 1, Revision: maker.Revision, Publish: true})
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
			t.Fatal("unexpected publication error")
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("publication race")
	}
	state, e := service.ReadPublication(ctx, token, first.ID)
	if e != nil || state.PublicPath != pub.PublicPath {
		t.Fatal("unstable public path")
	}
	_, p, e = service.PublicRead(ctx, key, first.ID)
	if e != nil || p.Name != "Changed draft" || len(p.Variants) != 4 || p.Variants[0].PricePence != 2999 {
		t.Fatal("republish failed")
	}
	// A failed projection write rolls back revision and public content together.
	if _, e = pool.Exec(ctx, "ALTER TABLE product_publications ADD CONSTRAINT fixture_fail CHECK(revision<0) NOT VALID"); e != nil {
		t.Fatal("fixture constraint failed")
	}
	if _, e = service.SavePublication(ctx, token, first.ID, catalog.PublicationInput{ShopRevision: 1, Revision: state.Revision, Publish: true}); !errors.Is(e, catalog.ErrStorage) {
		t.Fatal("projection failure not detected")
	}
	after, e := service.ReadPublication(ctx, token, first.ID)
	if e != nil || after.Revision != state.Revision || after.PublishedRevision != state.PublishedRevision || after.PublicPath != state.PublicPath {
		t.Fatal("publication failure partially committed")
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE product_publications DROP CONSTRAINT fixture_fail"); e != nil {
		t.Fatal("fixture reset failed")
	}

	// Shop identity is part of the reviewed publish action too.
	if _, e = pool.Exec(ctx, "UPDATE shops SET tagline='New identity',revision=revision+1 WHERE id<>$1", shopID); e != nil {
		t.Fatal("shop fixture update")
	}
	if _, e = service.SavePublication(ctx, token, first.ID, catalog.PublicationInput{ShopRevision: state.ShopRevision, Revision: state.Revision, Publish: true}); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("unreviewed shop identity published")
	}
	state, e = service.SavePublication(ctx, token, first.ID, catalog.PublicationInput{ShopRevision: 1, Revision: state.Revision, Publish: false})
	if e != nil || state.PublishedRevision != 0 || state.PublicPath != "" || browse(pub.PublicPath).Code != 404 || browse("/shop/"+key).Code != 404 {
		t.Fatal("unpublish failed")
	}
	if e = owners.Logout(ctx, token); e != nil {
		t.Fatal(e)
	}
	if _, e = service.SavePublication(ctx, token, first.ID, catalog.PublicationInput{ShopRevision: 1, Revision: state.Revision, Publish: true}); !errors.Is(e, auth.ErrCredential) {
		t.Fatal("logged-out publish")
	}
	pool.Close()
	if browse(pub.PublicPath).Code != 503 {
		t.Fatal("outage did not fail closed")
	}
}

func TestPublicCataloguePagination(t *testing.T) {
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = owners.CreateFirstOwner(ctx, "Demo maker", "owner@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	login, e := owners.Login(ctx, "owner@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	token := login.Token.Reveal()
	service := catalog.NewStore(pool)
	var key string
	for i := 1; i <= 52; i++ {
		p, e := service.Save(ctx, token, 0, catalog.Input{Name: fmt.Sprintf("Creation %d", i), PricePence: 1999, CertificateName: "none", CreationKey: fmt.Sprintf("00000000-0000-4000-8000-%012d", i)})
		if e != nil {
			t.Fatal(e)
		}
		m, e := service.SaveMaker(ctx, token, p.ID, catalog.MakerInput{Revision: p.Revision, Variants: []catalog.VariantInput{{Label: "Standard", SupplyMode: "made_to_order"}}})
		if e != nil {
			t.Fatal(e)
		}
		pub, e := service.SavePublication(ctx, token, p.ID, catalog.PublicationInput{Revision: m.Revision, ShopRevision: 1, Publish: true})
		if e != nil {
			t.Fatal(e)
		}
		current := strings.Split(pub.PublicPath, "/")[2]
		if key != "" && current != key {
			t.Fatal("shop key changed")
		}
		key = current
	}
	first, e := service.PublicList(ctx, key, 0)
	if e != nil || len(first.Products) != 50 || first.NextAfter == 0 {
		t.Fatal("first public page")
	}
	last, e := service.PublicList(ctx, key, first.NextAfter)
	if e != nil || len(last.Products) != 2 || last.NextAfter != 0 || last.Products[0].ID <= first.NextAfter {
		t.Fatal("last public page")
	}
}

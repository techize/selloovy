package shophttp_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/shop"
	"github.com/techize/selloovy/internal/shophttp"
	"github.com/techize/selloovy/internal/storefronthttp"
)

func TestPagesDraftPublicationIsolationAndAtomicChanges(t *testing.T) {
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = owners.CreateFirstOwner(ctx, "Page fixture", "owner@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	login, e := owners.Login(ctx, "owner@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	token := login.Token.Reveal()
	var secondID int64
	if pool.QueryRow(ctx, "INSERT INTO public.shops(name) VALUES('Other page fixture') RETURNING id").Scan(&secondID) != nil {
		t.Fatal("fixture")
	}
	if _, _, e = owners.CreateOwner(ctx, secondID, "other@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	other, e := owners.Login(ctx, "other@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	otherToken := other.Token.Reveal()
	shops := shop.NewStore(pool)
	products := catalog.NewStore(pool)
	makeProduct := func(tok, key, name string) catalog.Product {
		p, e := products.Save(ctx, tok, 0, catalog.Input{Name: name, PricePence: 1200, CertificateName: "none", CreationKey: key})
		if e != nil {
			t.Fatal(e)
		}
		m, e := products.SaveMaker(ctx, tok, p.ID, catalog.MakerInput{Revision: p.Revision, Variants: []catalog.VariantInput{{Label: "Standard", SupplyMode: "stocked", StockQuantity: 5}}})
		if e != nil {
			t.Fatal(e)
		}
		pub, e := products.ReadPublication(ctx, tok, p.ID)
		if e != nil {
			t.Fatal(e)
		}
		_, e = products.SavePublication(ctx, tok, p.ID, catalog.PublicationInput{Revision: m.Revision, ShopRevision: pub.ShopRevision, Publish: true})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	own := makeProduct(token, "00000000-0000-4000-8000-000000000001", "Sample creation")
	foreign := makeProduct(otherToken, "00000000-0000-4000-8000-000000000002", "Foreign creation")
	state, e := shops.ReadPages(ctx, token)
	if e != nil || len(state.Drafts) != 0 || len(state.Products) != 1 || state.Products[0].ID != own.ID {
		t.Fatal("page product ownership")
	}
	homepage := shop.Page{Slug: "home", Title: "Fixture home", Sections: []shop.Section{{ID: "00000000000000000000000000000001", Kind: "hero", Heading: "Private heading", Text: "Our maker story", Theme: "cream", ButtonLabel: "Explore", ButtonPath: "/products", ProductIDs: []int64{}, Questions: []shop.FAQ{}}, {ID: "00000000000000000000000000000002", Kind: "collection", Heading: "Creations", Theme: "sage", ProductIDs: []int64{own.ID}, Questions: []shop.FAQ{}}}}
	change := func(action string, p shop.Page) {
		t.Helper()
		var err error
		state, err = shops.ChangePage(ctx, token, shop.PageChange{Revision: state.Revision, Action: action, Slug: p.Slug, Page: p})
		if err != nil {
			t.Fatal(err)
		}
	}
	change("save", homepage)
	public, e := shops.PublicPages(ctx, state.PublicKey)
	if e != nil || len(public) != 0 {
		t.Fatal("draft leaked")
	}
	otherState, e := shops.ReadPages(ctx, otherToken)
	if e != nil || len(otherState.Drafts) != 0 {
		t.Fatal("foreign draft leaked")
	}
	publicHandler := chi.NewRouter()
	publicHandler.Mount("/shop", storefronthttp.NewWithCalendar(products, "", nil, shops))
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		publicHandler.ServeHTTP(w, r)
		return w
	}
	base := "/shop/" + state.PublicKey
	if w := get(base); w.Code != 200 || strings.Contains(w.Body.String(), "Private heading") {
		t.Fatal("private homepage rendered")
	}
	change("publish", homepage)
	if w := get(base); w.Code != 200 || !strings.Contains(w.Body.String(), "Private heading") || !strings.Contains(w.Body.String(), "/products\"") {
		t.Fatal("homepage publication")
	}
	if w := get(base + "/products"); w.Code != 200 || strings.Contains(w.Body.String(), "Private heading") || !strings.Contains(w.Body.String(), "Sample creation") {
		t.Fatal("catalogue lost")
	}
	homepage.Sections[0].Heading = "Updated private heading"
	change("save", homepage)
	if strings.Contains(get(base).Body.String(), "Updated private heading") {
		t.Fatal("draft changed live")
	}
	change("publish", homepage)
	if !strings.Contains(get(base).Body.String(), "Updated private heading") {
		t.Fatal("republish failed")
	}
	info := shop.Page{Slug: "our-story", Title: "Our story", Sections: []shop.Section{{ID: "00000000000000000000000000000003", Kind: "text-image", Heading: "Story <script>alert(1)</script>", Text: "We make things", Theme: "ink", ImageProductID: own.ID, ProductIDs: []int64{}, Questions: []shop.FAQ{}}, {ID: "00000000000000000000000000000004", Kind: "faq", Heading: "Questions", Theme: "sage", ProductIDs: []int64{}, Questions: []shop.FAQ{{Question: "How?", Answer: "With care."}}}}}
	change("save", info)
	if get(base+"/pages/our-story").Code != 404 {
		t.Fatal("private information page exposed")
	}
	change("publish", info)
	w := get(base + "/pages/our-story")
	if w.Code != 200 || strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") || !strings.Contains(w.Body.String(), "With care.") {
		t.Fatal("escaped info rendering")
	}
	if !strings.Contains(get(base).Body.String(), "/pages/our-story") {
		t.Fatal("missing navigation")
	}
	if get(base+"/pages/home").Code != 404 || get(base+"/pages/our-story?draft=1").Code != 404 {
		t.Fatal("unexpected page aliases")
	}
	bad := homepage
	bad.Sections = append([]shop.Section{}, homepage.Sections...)
	bad.Sections[1].ProductIDs = []int64{foreign.ID}
	change("save", bad)
	old := state
	_, e = shops.ChangePage(ctx, token, shop.PageChange{Revision: state.Revision, Action: "publish", Slug: "home"})
	var validation *shop.ValidationError
	if !errors.As(e, &validation) {
		t.Fatal("foreign product published")
	}
	state, e = shops.ReadPages(ctx, token)
	if e != nil || state.Revision != old.Revision || !strings.Contains(get(base).Body.String(), "Updated private heading") {
		t.Fatal("failed publish changed state")
	}
	_, e = shops.ChangePage(ctx, token, shop.PageChange{Revision: 1, Action: "unpublish", Slug: "home"})
	if !errors.Is(e, shop.ErrConflict) {
		t.Fatal("stale publication accepted")
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE public.shops ADD CONSTRAINT fixture_page_rollback CHECK(page_drafts::text NOT LIKE '%Reject fixture%')"); e != nil {
		t.Fatal("rollback fixture")
	}
	bad = homepage
	bad.Title = "Reject fixture"
	_, e = shops.ChangePage(ctx, token, shop.PageChange{Revision: state.Revision, Action: "save", Slug: "home", Page: bad})
	if !errors.Is(e, shop.ErrStorage) {
		t.Fatal("database failure ignored")
	}
	after, e := shops.ReadPages(ctx, token)
	if e != nil || after.Revision != state.Revision {
		t.Fatal("failed write advanced revision")
	}
	guard, e := authhttp.New(owners, "https://shop.example.com")
	if e != nil {
		t.Fatal(e)
	}
	routes := chi.NewRouter()
	routes.Mount("/api/admin", shophttp.New(shops))
	h := guard.ProtectAdmin(routes)
	send := func(method, body, tok, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://shop.example.com/api/admin/pages", strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Selloovy-Request", "owner-auth")
		if tok != "" {
			r.AddCookie(&http.Cookie{Name: "__Host-selloovy_session", Value: tok})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if send("GET", "", "", "").Code != 401 || send("PUT", `{}`, token, "https://other.example.com").Code != 403 || send("PUT", `{"shopId":9}`, token, "https://shop.example.com").Code != 400 {
		t.Fatal("page API boundary")
	}
	body, _ := json.Marshal(shop.PageChange{Revision: state.Revision, Action: "unpublish", Slug: "home"})
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		wg.Go(func() { statuses <- send("PUT", string(body), token, "https://shop.example.com").Code })
	}
	wg.Wait()
	close(statuses)
	ok, conflict := 0, 0
	for status := range statuses {
		if status == 200 {
			ok++
		}
		if status == 409 {
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal("concurrent publication was not serialized")
	}
	if w := get(base); w.Code != 200 || strings.Contains(w.Body.String(), "Updated private heading") || !strings.Contains(w.Body.String(), "Sample creation") {
		t.Fatal("homepage unpublish did not restore catalogue")
	}
	state, e = shops.ReadPages(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	change("unpublish", info)
	if get(base+"/pages/our-story").Code != 404 || strings.Contains(get(base).Body.String(), "/pages/our-story") {
		t.Fatal("unpublished info remained visible")
	}
	change("delete", info)
	// Publications follow product visibility and do not retain an unpublished cover/product.
	second := makeProduct(token, "00000000-0000-4000-8000-000000000003", "Another sample")
	state, e = shops.ReadPages(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	change("save", homepage)
	change("publish", homepage)
	// Page-owned metadata does not keep an unpublished product publicly accessible.
	pub, e := products.ReadPublication(ctx, token, own.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = products.SavePublication(ctx, token, own.ID, catalog.PublicationInput{Revision: pub.Revision, ShopRevision: pub.ShopRevision, Publish: false})
	if e != nil {
		t.Fatal(e)
	}
	if w := get(base); w.Code != 200 || strings.Contains(w.Body.String(), "Sample creation") || !strings.Contains(w.Body.String(), "These creations are currently unavailable.") {
		t.Fatal("unpublished product remained in collection")
	}
	state, e = shops.ReadPages(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	_, e = shops.ChangePage(ctx, token, shop.PageChange{Revision: state.Revision, Action: "publish", Slug: "home"})
	if !errors.As(e, &validation) {
		t.Fatal("unpublished product republished through page")
	}
	pub, e = products.ReadPublication(ctx, token, second.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = products.SavePublication(ctx, token, second.ID, catalog.PublicationInput{Revision: pub.Revision, ShopRevision: pub.ShopRevision, Publish: false})
	if e != nil {
		t.Fatal(e)
	}
	if get(base).Code != 404 {
		t.Fatal("last product visibility boundary lost")
	}
}

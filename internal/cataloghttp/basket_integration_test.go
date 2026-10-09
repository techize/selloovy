package cataloghttp_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/storefronthttp"
)

func TestGuestBasketPersonalisationPricesIsolationAndHTTP(t *testing.T) {
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = owners.CreateFirstOwner(ctx, "Basket fixture", "owner@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	login, e := owners.Login(ctx, "owner@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	token := login.Token.Reveal()
	c := catalog.NewStore(pool)
	create := func(name, policy, key string) (catalog.Product, int64, string) {
		t.Helper()
		p, e := c.Save(ctx, token, 0, catalog.Input{Name: name, PricePence: 1999, CertificateName: policy, CreationKey: key})
		if e != nil {
			t.Fatal(e)
		}
		m, e := c.SaveMaker(ctx, token, p.ID, catalog.MakerInput{Revision: p.Revision, Variants: []catalog.VariantInput{{Label: "Original", SupplyMode: "made_to_order"}}})
		if e != nil {
			t.Fatal(e)
		}
		pub, e := c.ReadPublication(ctx, token, p.ID)
		if e != nil {
			t.Fatal(e)
		}
		pub, e = c.SavePublication(ctx, token, p.ID, catalog.PublicationInput{Revision: m.Revision, ShopRevision: pub.ShopRevision, Publish: true})
		if e != nil {
			t.Fatal(e)
		}
		p, e = c.Read(ctx, token, p.ID)
		if e != nil {
			t.Fatal(e)
		}
		return p, m.Variants[0].ID, strings.Split(pub.PublicPath, "/")[2]
	}
	p, variant, key := create("Demo creation", "required", "00000000-0000-4000-8000-000000000001")
	plain, plainVariant, _ := create("Plain creation", "none", "00000000-0000-4000-8000-000000000002")
	guest := strings.Repeat("a", 64)
	other := strings.Repeat("b", 64)
	input := catalog.BasketChange{ProductID: p.ID, VariantID: variant, Quantity: 2}
	if _, e = c.ChangeBasket(ctx, key, guest, input); e == nil {
		t.Fatal("required owner name missing")
	}
	input.OwnerName = "  Demo owner  "
	basket, e := c.ChangeBasket(ctx, key, guest, input)
	if e != nil || basket.TotalPence != 3998 || basket.Lines[0].OwnerName != "Demo owner" {
		t.Fatal("add or exact trimmed name failed")
	}
	input.Revision = basket.Revision
	basket, e = c.ChangeBasket(ctx, key, guest, input)
	if e != nil || len(basket.Lines) != 1 || basket.Lines[0].Quantity != 4 {
		t.Fatal("matching line merge")
	}
	if _, e = c.ChangeBasket(ctx, key, guest, input); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("duplicate form accepted")
	}
	input.Revision = basket.Revision
	input.OwnerName = "Other owner"
	basket, e = c.ChangeBasket(ctx, key, guest, input)
	if e != nil || len(basket.Lines) != 2 {
		t.Fatal("different owners not separate")
	}
	empty, e := c.ReadBasket(ctx, key, other)
	if e != nil || len(empty.Lines) != 0 {
		t.Fatal("browser basket isolation")
	}
	bad := input
	bad.Revision = basket.Revision
	bad.ProductID = plain.ID
	bad.VariantID = plainVariant
	if _, e = c.ChangeBasket(ctx, key, guest, bad); e == nil {
		t.Fatal("unsupported certificate accepted")
	}
	bad = input
	bad.Revision = basket.Revision
	bad.VariantID = plainVariant
	if _, e = c.ChangeBasket(ctx, key, guest, bad); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("foreign variant accepted")
	}
	bad = input
	bad.Revision = basket.Revision
	bad.OwnerName = "Bad\nname"
	if _, e = c.ChangeBasket(ctx, key, guest, bad); e == nil {
		t.Fatal("control character accepted")
	}
	bad.OwnerName = strings.Repeat("x", 81)
	if _, e = c.ChangeBasket(ctx, key, guest, bad); e == nil {
		t.Fatal("long owner name accepted")
	}
	bad.OwnerName = "Demo owner"
	bad.Quantity = 100
	if _, e = c.ChangeBasket(ctx, key, guest, bad); e == nil {
		t.Fatal("quantity bound")
	}

	// Another shop cannot resolve this shop's product or basket lines.
	var otherShop int64
	if pool.QueryRow(ctx, "INSERT INTO public.shops(name) VALUES('Other fixture') RETURNING id").Scan(&otherShop) != nil {
		t.Fatal("fixture failed")
	}
	_, _, e = owners.CreateOwner(ctx, otherShop, "other@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	otherLogin, e := owners.Login(ctx, "other@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	ownerToken := token
	token = otherLogin.Token.Reveal()
	foreign, foreignVariant, foreignKey := create("Other creation", "optional", "00000000-0000-4000-8000-000000000003")
	token = ownerToken
	empty, e = c.ReadBasket(ctx, foreignKey, guest)
	if e != nil || len(empty.Lines) != 0 {
		t.Fatal("shop basket isolation")
	}
	bad = input
	bad.Revision = basket.Revision
	bad.ProductID = foreign.ID
	bad.VariantID = foreignVariant
	if _, e = c.ChangeBasket(ctx, key, guest, bad); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("foreign shop product accepted")
	}
	persistent, e := catalog.NewStore(pool).ReadBasket(ctx, key, guest)
	if e != nil || persistent.Revision != basket.Revision || len(persistent.Lines) != 2 {
		t.Fatal("basket persistence")
	}
	// Force a storage failure after mutation assembly; the prior basket must survive.
	if _, e = pool.Exec(ctx, "ALTER TABLE public.baskets ADD CONSTRAINT fixture_rollback CHECK(lines::text NOT LIKE '%Reject fixture%')"); e != nil {
		t.Fatal("fixture failed")
	}
	bad = input
	bad.Revision = basket.Revision
	bad.OwnerName = "Reject fixture"
	if _, e = c.ChangeBasket(ctx, key, guest, bad); e == nil {
		t.Fatal("induced failure missing")
	}
	persistent, e = c.ReadBasket(ctx, key, guest)
	if e != nil || persistent.Revision != basket.Revision || len(persistent.Lines) != 2 {
		t.Fatal("failed write changed basket")
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE public.baskets DROP CONSTRAINT fixture_rollback"); e != nil {
		t.Fatal("fixture failed")
	}
	// A base-price draft does not affect baskets until publication.
	p, e = c.Save(ctx, token, p.ID, catalog.Input{Name: p.Name, PricePence: 2500, CertificateName: p.CertificateName, Revision: p.Revision})
	if e != nil {
		t.Fatal(e)
	}
	same, e := c.ReadBasket(ctx, key, guest)
	if e != nil || same.TotalPence != 11994 || same.Lines[0].Changed {
		t.Fatal("draft price leaked")
	}
	pub, e := c.ReadPublication(ctx, token, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.SavePublication(ctx, token, p.ID, catalog.PublicationInput{Revision: p.Revision, ShopRevision: pub.ShopRevision, Publish: true})
	if e != nil {
		t.Fatal(e)
	}
	repriced, e := c.ReadBasket(ctx, key, guest)
	if e != nil || repriced.TotalPence != 15000 || !repriced.Lines[0].Changed {
		t.Fatal("published reprice not surfaced")
	}
	// Racing writes to one revision cannot overwrite one another.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, e := c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: basket.Revision, LineID: basket.Lines[0].ID, Quantity: 3, OwnerName: "Demo owner"})
			results <- e
		})
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for e := range results {
		if e == nil {
			wins++
		} else if errors.Is(e, catalog.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal("racing basket update")
	}
	// HTML forms are origin-bound, cookie-bound and reject client prices/selectors.
	h := chi.NewRouter()
	h.Mount("/shop", storefronthttp.New(c, "http://127.0.0.1:8081"))
	get := httptest.NewRequest("GET", "http://127.0.0.1:8081/shop/"+key+"/products/"+strconv.FormatInt(p.ID, 10), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 || w.Header().Get("Referrer-Policy") != "same-origin" {
		t.Fatal("product basket form")
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/shop/"+key {
		t.Fatal("basket cookie scope")
	}
	csrf := regexp.MustCompile(`name="csrf" value="([0-9a-f]{64})"`).FindStringSubmatch(w.Body.String())
	if len(csrf) != 2 {
		t.Fatal("missing CSRF form")
	}
	values := url.Values{"csrf": {csrf[1]}, "revision": {"0"}, "product": {strconv.FormatInt(p.ID, 10)}, "variant": {strconv.FormatInt(variant, 10)}, "quantity": {"1"}, "ownerName": {"<Demo & owner>"}}
	send := func(v url.Values, origin string, cook bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8081/shop/"+key+"/basket", strings.NewReader(v.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", origin)
		if cook {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if send(values, "http://other.example.com", true).Code != 403 || send(values, "http://127.0.0.1:8081", false).Code != 403 {
		t.Fatal("origin or cookie boundary")
	}
	values.Set("price", "1")
	if send(values, "http://127.0.0.1:8081", true).Code != 400 {
		t.Fatal("client price accepted")
	}
	values.Del("price")

	values.Set("csrf", "invalid")
	if send(values, "http://127.0.0.1:8081", true).Code != 403 {
		t.Fatal("CSRF accepted")
	}
	values.Set("csrf", csrf[1])
	values["quantity"] = []string{"1", "2"}
	if send(values, "http://127.0.0.1:8081", true).Code != 400 {
		t.Fatal("duplicate field accepted")
	}
	values.Set("quantity", "1")
	if send(values, "http://127.0.0.1:8081", true).Code != 303 {
		t.Fatal("valid basket add")
	}
	if send(values, "http://127.0.0.1:8081", true).Code != 409 {
		t.Fatal("HTTP duplicate add")
	}
	get = httptest.NewRequest("GET", "http://127.0.0.1:8081/shop/"+key+"/basket", nil)
	get.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, get)
	if w.Code != 200 || strings.Contains(w.Body.String(), "<Demo & owner>") || !strings.Contains(w.Body.String(), "&lt;Demo &amp; owner&gt;") {
		t.Fatal("basket escaping")
	}

	secure := chi.NewRouter()
	secure.Mount("/shop", storefronthttp.New(c, "https://shop.example.com"))
	secureGet := httptest.NewRequest("GET", "https://shop.example.com/shop/"+key+"/products/"+strconv.FormatInt(p.ID, 10), nil)
	secureW := httptest.NewRecorder()
	secure.ServeHTTP(secureW, secureGet)
	if secureW.Code != 200 || len(secureW.Result().Cookies()) != 1 || !secureW.Result().Cookies()[0].Secure || secureW.Result().Cookies()[0].Name != "__Secure-selloovy_basket" {
		t.Fatal("HTTPS cookie protection")
	}
	pub, e = c.ReadPublication(ctx, token, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.SavePublication(ctx, token, p.ID, catalog.PublicationInput{Revision: pub.Revision, Publish: false})
	if e != nil {
		t.Fatal(e)
	}
	unavailable, e := c.ReadBasket(ctx, key, guest)
	if e != nil || unavailable.Complete || unavailable.Lines[0].Available {
		t.Fatal("unpublished product still available")
	}
	removed, e := c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: unavailable.Revision, LineID: unavailable.Lines[0].ID, Quantity: 0})
	if e != nil || len(removed.Lines) != 1 {
		t.Fatal("unavailable line removal")
	}
	if _, e = pool.Exec(ctx, "UPDATE public.baskets SET expires_at=clock_timestamp()-interval '1 second'"); e != nil {
		t.Fatal("fixture expiry failed")
	}
	expired, e := c.ReadBasket(ctx, key, guest)
	if e != nil || expired.Revision != 0 || len(expired.Lines) != 0 {
		t.Fatal("expired personalisation exposed")
	}
}

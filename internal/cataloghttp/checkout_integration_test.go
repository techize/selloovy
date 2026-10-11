package cataloghttp_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/delivery"
	"github.com/techize/selloovy/internal/shop"
	"github.com/techize/selloovy/internal/storefronthttp"
)

func TestCheckoutReviewHoldsConcurrencyAndPendingProtection(t *testing.T) {
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = owners.CreateFirstOwner(ctx, "Checkout fixture", "owner@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	login, e := owners.Login(ctx, "owner@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	token := login.Token.Reveal()
	c := catalog.NewStore(pool)
	p, e := c.Save(ctx, token, 0, catalog.Input{Name: "Fixture dragon", PricePence: 1999, CertificateName: "optional", CreationKey: "00000000-0000-4000-8000-000000000001"})
	if e != nil {
		t.Fatal(e)
	}
	m, e := c.SaveMaker(ctx, token, p.ID, catalog.MakerInput{Revision: p.Revision, Variants: []catalog.VariantInput{{Label: "Amber", SupplyMode: "stocked", StockQuantity: 1}}})
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
	key := strings.Split(pub.PublicPath, "/")[2]
	variant := m.Variants[0].ID
	shipping, e := shop.NewStore(pool).ReadShipping(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	shipping.Services = []delivery.Service{{ID: "00000000000000000000000000000001", Name: "Standard", FeePence: 499, DaysMin: 3, DaysMax: 5, Enabled: true}}
	shipping, e = shop.NewStore(pool).SaveShipping(ctx, token, shipping)
	if e != nil {
		t.Fatal(e)
	}
	customer := catalog.Customer{Email: "customer@example.com", Name: "Test Buyer", Address1: "1 Example Lane", Town: "Exampletown", Postcode: "AB1 2CD", Country: "GB"}
	prepare := func(guest string) catalog.Checkout {
		t.Helper()
		b, e := c.ChangeBasket(ctx, key, guest, catalog.BasketChange{ProductID: p.ID, VariantID: variant, Quantity: 1, OwnerName: "Test owner"})
		if e != nil {
			t.Fatal(e)
		}
		_, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: b.Revision, ServiceID: shipping.Services[0].ID, ShippingRevision: shipping.Revision})
		if e != nil {
			t.Fatal(e)
		}
		review, e := c.ReadCheckout(ctx, key, guest)
		if e != nil || !review.Ready || review.Saved != nil {
			t.Fatal("review failed")
		}
		saved, e := c.SaveCheckout(ctx, key, guest, catalog.CheckoutChange{Revision: review.Revision, QuoteHash: review.QuoteHash, Customer: customer})
		if e != nil || saved.Saved == nil || saved.Changed {
			t.Fatal("save failed")
		}
		return saved
	}
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	ra, rb := prepare(a), prepare(b)
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_stock_holds").Scan(&count) != nil || count != 0 {
		t.Fatal("browsing/saving reserved stock")
	}
	if _, e = c.ReadCheckout(ctx, key, strings.Repeat("c", 64)); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("other browser accessed checkout")
	}
	if _, e = c.SaveCheckout(ctx, key, a, catalog.CheckoutChange{Revision: 0, QuoteHash: ra.QuoteHash, Customer: customer}); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("stale review accepted")
	}
	if _, e = c.SaveCheckout(ctx, key, a, catalog.CheckoutChange{Revision: ra.Revision, QuoteHash: strings.Repeat("0", 64), Customer: customer}); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("tampered quote accepted")
	}
	start := make(chan struct{})
	results := make(chan struct {
		guest string
		err   error
	}, 2)
	var wg sync.WaitGroup
	for _, x := range []struct {
		guest string
		rev   int64
	}{{a, ra.Revision}, {b, rb.Revision}} {
		wg.Add(1)
		go func(guest string, rev int64) {
			defer wg.Done()
			<-start
			results <- struct {
				guest string
				err   error
			}{guest, c.HoldCheckout(ctx, key, guest, rev)}
		}(x.guest, x.rev)
	}
	close(start)
	wg.Wait()
	close(results)
	winner, loser := "", ""
	for result := range results {
		if result.err == nil {
			if winner != "" {
				t.Fatal("last item held twice")
			}
			winner = result.guest
		} else if errors.Is(result.err, catalog.ErrConflict) {
			loser = result.guest
		} else {
			t.Fatal(result.err)
		}
	}
	if winner == "" || loser == "" {
		t.Fatal("allocation outcomes wrong")
	}
	own, e := c.ReadCheckout(ctx, key, winner)
	if e != nil || !own.Ready || own.Changed {
		t.Fatal("own hold blocked own checkout")
	}
	if e = c.HoldCheckout(ctx, key, winner, own.Revision); e != nil {
		t.Fatal("hold retry failed")
	}
	var quantity int64
	if pool.QueryRow(ctx, "SELECT sum(quantity) FROM checkout_stock_holds").Scan(&quantity) != nil || quantity != 1 {
		t.Fatal("allocation wrong")
	}
	if _, e = pool.Exec(ctx, "UPDATE checkout_stock_holds SET expires_at=clock_timestamp()-interval '1 second'; UPDATE checkout_reviews SET hold_expires_at=clock_timestamp()-interval '1 second'"); e != nil {
		t.Fatal("expiry fixture failed")
	}
	if e = c.MarkCheckoutPaymentPending(ctx, key, winner, own.Revision); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("expired hold submitted")
	}
	if e = c.HoldCheckout(ctx, key, loser, 1); e != nil {
		t.Fatal("expired allocation not released")
	}
	if e = c.MarkCheckoutPaymentPending(ctx, key, loser, 1); e != nil {
		t.Fatal("pending protection failed")
	}
	if _, e = pool.Exec(ctx, "UPDATE checkout_stock_holds SET expires_at=clock_timestamp()-interval '1 second'"); e != nil {
		t.Fatal("expiry fixture failed")
	}
	if e = c.HoldCheckout(ctx, key, winner, 1); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("uncertain payment allocation expired")
	}
	basket, e := c.ReadBasket(ctx, key, loser)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.ChangeBasket(ctx, key, loser, catalog.BasketChange{Revision: basket.Revision, LineID: basket.Lines[0].ID, Quantity: 0}); e == nil {
		t.Fatal("pending basket editable")
	}
	pending, e := c.ReadCheckout(ctx, key, loser)
	if e != nil || !pending.PaymentPending {
		t.Fatal("pending state lost")
	}
	if _, e = c.SaveCheckout(ctx, key, loser, catalog.CheckoutChange{Revision: pending.Revision, QuoteHash: pending.QuoteHash, Customer: customer}); e == nil {
		t.Fatal("pending customer details editable")
	}
	m, e = c.ReadMaker(ctx, token, p.ID)
	if e != nil || m.Variants[0].AllocatedQuantity != 1 {
		t.Fatal("owner cannot see allocations")
	}
	input := catalog.MakerInput{Revision: m.Revision, Variants: []catalog.VariantInput{m.Variants[0].VariantInput}}
	input.Variants[0].StockQuantity = 0
	if _, e = c.SaveMaker(ctx, token, p.ID, input); e == nil {
		t.Fatal("allocated stock reduced")
	}
	unchanged, e := c.ReadMaker(ctx, token, p.ID)
	if e != nil || unchanged.Revision != m.Revision || unchanged.Variants[0].StockQuantity != 1 {
		t.Fatal("stock failure did not roll back")
	}
	input.Variants[0].StockQuantity = 1
	input.Variants[0].SupplyMode = "made_to_order"
	if _, e = c.SaveMaker(ctx, token, p.ID, input); e == nil {
		t.Fatal("allocated stock mode changed")
	}

	// Remaining stock can be allocated once across differently personalised lines;
	// the excess stays made to order under the product-controlled fallback.
	input.Variants[0].SupplyMode = "stocked"
	input.Variants[0].StockQuantity = 2
	input.MadeToOrderFallback = true
	m, e = c.SaveMaker(ctx, token, p.ID, input)
	if e != nil {
		t.Fatal(e)
	}
	pub, e = c.ReadPublication(ctx, token, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.SavePublication(ctx, token, p.ID, catalog.PublicationInput{Revision: m.Revision, ShopRevision: pub.ShopRevision, Publish: true}); e != nil {
		t.Fatal(e)
	}
	third := strings.Repeat("c", 64)
	prepare(third)
	extra, e := c.ReadBasket(ctx, key, third)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.ChangeBasket(ctx, key, third, catalog.BasketChange{Revision: extra.Revision, ProductID: p.ID, VariantID: variant, Quantity: 2, OwnerName: "Another owner"}); e != nil {
		t.Fatal(e)
	}
	review, e := c.ReadCheckout(ctx, key, third)
	if e != nil || !review.Ready || review.Current.DispatchDaysMin != 5 {
		t.Fatal("partial-stock preparation incorrect")
	}
	review, e = c.SaveCheckout(ctx, key, third, catalog.CheckoutChange{Revision: review.Revision, QuoteHash: review.QuoteHash, Customer: customer})
	if e != nil {
		t.Fatal(e)
	}
	if e = c.HoldCheckout(ctx, key, third, review.Revision); e != nil {
		t.Fatal(e)
	}
	if pool.QueryRow(ctx, "SELECT sum(quantity) FROM checkout_stock_holds WHERE retained OR expires_at>clock_timestamp()").Scan(&quantity) != nil || quantity != 2 {
		t.Fatal("personalisation/fallback overallocated stock")
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE checkout_reviews ADD CONSTRAINT fixture_reject_save CHECK(revision<=2)"); e != nil {
		t.Fatal("rollback fixture failed")
	}
	if _, e = c.SaveCheckout(ctx, key, third, catalog.CheckoutChange{Revision: review.Revision, QuoteHash: review.QuoteHash, Customer: customer}); !errors.Is(e, catalog.ErrStorage) {
		t.Fatal("forced persistence failure not reported")
	}
	if pool.QueryRow(ctx, "SELECT sum(quantity) FROM checkout_stock_holds WHERE retained OR expires_at>clock_timestamp()").Scan(&quantity) != nil || quantity != 2 {
		t.Fatal("failed save released allocation")
	}
	after, e := c.ReadCheckout(ctx, key, third)
	if e != nil || after.Revision != review.Revision || after.Changed {
		t.Fatal("failed save changed review")
	}
}

func TestCheckoutHTTPAndStaleShipping(t *testing.T) {
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = owners.CreateFirstOwner(ctx, "Checkout fixture", "owner@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	login, e := owners.Login(ctx, "owner@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	token := login.Token.Reveal()
	c := catalog.NewStore(pool)
	shops := shop.NewStore(pool)
	p, e := c.Save(ctx, token, 0, catalog.Input{Name: "Fixture dinosaur", PricePence: 1999, CertificateName: "optional", CreationKey: "00000000-0000-4000-8000-000000000001"})
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
	key := strings.Split(pub.PublicPath, "/")[2]
	guest := strings.Repeat("a", 64)
	shipping, e := shops.ReadShipping(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	shipping.Services = []delivery.Service{{ID: "00000000000000000000000000000001", Name: "Standard", FeePence: 499, DaysMin: 3, DaysMax: 5, Enabled: true}}
	shipping, e = shops.SaveShipping(ctx, token, shipping)
	if e != nil {
		t.Fatal(e)
	}
	basket, e := c.ChangeBasket(ctx, key, guest, catalog.BasketChange{ProductID: p.ID, VariantID: m.Variants[0].ID, Quantity: 1})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: basket.Revision, ServiceID: shipping.Services[0].ID, ShippingRevision: shipping.Revision})
	if e != nil {
		t.Fatal(e)
	}
	routes := chi.NewRouter()
	routes.Mount("/shop", storefronthttp.NewWithCalendar(c, "https://shop.example.com", delivery.DefaultCalendar(), shops))
	path := "/shop/" + key + "/checkout"
	send := func(method string, form url.Values, origin, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://shop.example.com"+path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", origin)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: "__Secure-selloovy_basket", Value: cookie})
		}
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, r)
		return w
	}
	page := send("GET", nil, "", guest)
	if page.Code != 200 || page.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private checkout page failed")
	}
	hidden := func(name string) string {
		v := regexp.MustCompile(`name="` + name + `" value="([^"]*)"`).FindStringSubmatch(page.Body.String())
		if len(v) != 2 {
			t.Fatal("missing review field")
		}
		return v[1]
	}
	form := url.Values{"csrf": {hidden("csrf")}, "revision": {hidden("revision")}, "quote": {hidden("quote")}, "email": {"customer@example.com"}, "name": {"<script>buyer</script>"}, "address1": {"1 Example Lane"}, "town": {"Exampletown"}, "postcode": {"AB1 2CD"}, "country": {"GB"}}
	if send("POST", form, "https://other.example.com", guest).Code != 403 {
		t.Fatal("cross origin checkout accepted")
	}
	if send("POST", form, "https://shop.example.com", strings.Repeat("b", 64)).Code != 403 {
		t.Fatal("foreign cookie accepted CSRF")
	}
	form.Set("total", "1")
	if send("POST", form, "https://shop.example.com", guest).Code != 400 {
		t.Fatal("client total accepted")
	}
	form.Del("total")
	form.Add("email", "other@example.com")
	if send("POST", form, "https://shop.example.com", guest).Code != 400 {
		t.Fatal("duplicate field accepted")
	}
	form.Set("email", "customer@example.com")
	form.Set("email", "bad")
	if send("POST", form, "https://shop.example.com", guest).Code != 422 {
		t.Fatal("validation feedback missing")
	}
	form.Set("email", "customer@example.com")
	saved := send("POST", form, "https://shop.example.com", guest)
	if saved.Code != 303 || len(saved.Result().Cookies()) != 1 || saved.Result().Cookies()[0].MaxAge != 30*24*60*60 {
		t.Fatal("save/retention cookie failed")
	}
	page = send("GET", nil, "", guest)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Your checkout details are saved") || strings.Contains(page.Body.String(), "<script>buyer</script>") {
		t.Fatal("saved feedback/escaping failed")
	}
	review, e := c.ReadCheckout(ctx, key, guest)
	if e != nil {
		t.Fatal(e)
	}
	shipping.Services[0].FeePence++
	shipping, e = shops.SaveShipping(ctx, token, shipping)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.SaveCheckout(ctx, key, guest, catalog.CheckoutChange{Revision: review.Revision, QuoteHash: review.QuoteHash, Customer: review.Customer}); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("stale shipping total saved")
	}
	current, e := c.ReadCheckout(ctx, key, guest)
	if e != nil || !current.Changed || current.Saved.TotalPence != review.Saved.TotalPence {
		t.Fatal("stale review overwritten")
	}
	if e = c.HoldCheckout(ctx, key, guest, current.Revision); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("stale shipping submitted")
	}
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_stock_holds").Scan(&count) != nil || count != 0 {
		t.Fatal("made-to-order physical allocation")
	}
	current, e = c.SaveCheckout(ctx, key, guest, catalog.CheckoutChange{Revision: current.Revision, QuoteHash: current.QuoteHash, Customer: current.Customer})
	if e != nil {
		t.Fatal(e)
	}
	if e = c.HoldCheckout(ctx, key, guest, current.Revision); e != nil {
		t.Fatal(e)
	}
	if e = c.MarkCheckoutPaymentPending(ctx, key, guest, current.Revision); e != nil {
		t.Fatal(e)
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_stock_holds").Scan(&count) != nil || count != 0 {
		t.Fatal("made-to-order stock held")
	}
	shipping.Services[0].FeePence = 999
	if _, e = shops.SaveShipping(ctx, token, shipping); e != nil {
		t.Fatal(e)
	}
	page = send("GET", nil, "", guest)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Total: £24.99") || strings.Contains(page.Body.String(), "Total: £29.98") {
		t.Fatal("pending financial summary changed")
	}
	if _, e = pool.Exec(ctx, "UPDATE baskets SET expires_at=clock_timestamp()-interval '1 second'; UPDATE checkout_reviews SET expires_at=clock_timestamp()-interval '1 second'"); e != nil {
		t.Fatal("expiry fixture failed")
	}
	if _, e = c.ChangeBasket(ctx, key, strings.Repeat("b", 64), catalog.BasketChange{ProductID: p.ID, VariantID: m.Variants[0].ID, Quantity: 1}); e != nil {
		t.Fatal(e)
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_reviews WHERE payment_pending").Scan(&count) != nil || count != 1 {
		t.Fatal("unresolved payment record purged")
	}
}

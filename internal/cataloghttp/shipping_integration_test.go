package cataloghttp_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/delivery"
	"github.com/techize/selloovy/internal/shop"
	"github.com/techize/selloovy/internal/shophttp"
)

func TestShippingSelectionThresholdsUpdatesStockAndPreparation(t *testing.T) {
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = owners.CreateFirstOwner(ctx, "Shipping fixture", "owner@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	login, e := owners.Login(ctx, "owner@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	token := login.Token.Reveal()
	shops := shop.NewStore(pool)
	c := catalog.NewStore(pool)
	s, e := shops.ReadShipping(ctx, token)
	if e != nil || len(s.Services) != 0 {
		t.Fatal("initial services")
	}
	threshold := int64(3000)
	standard := delivery.Service{ID: "00000000000000000000000000000001", Name: "Standard", FeePence: 499, DaysMin: 3, DaysMax: 5, FreeFromPence: &threshold, Enabled: true}
	tracked := delivery.Service{ID: "00000000000000000000000000000002", Name: "Tracked", FeePence: 599, DaysMin: 2, DaysMax: 3, Enabled: true}

	guard, e := authhttp.New(owners, "https://shop.example.com")
	if e != nil {
		t.Fatal(e)
	}
	routes := chi.NewRouter()
	routes.Mount("/api/admin", shophttp.New(shops))
	h := guard.ProtectAdmin(routes)
	send := func(method, body, tok, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://shop.example.com/api/admin/shipping", strings.NewReader(body))
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
	if send("GET", "", "", "").Code != 401 || send("PUT", `{}`, token, "https://other.example.com").Code != 403 || send("PUT", `{"shopId":99}`, token, "https://shop.example.com").Code != 400 {
		t.Fatal("shipping HTTP boundary")
	}
	badConfig := delivery.Settings{Revision: s.Revision, Services: []delivery.Service{standard, standard}}
	body, _ := json.Marshal(badConfig)
	if send("PUT", string(body), token, "https://shop.example.com").Code != 422 {
		t.Fatal("duplicate service accepted")
	}
	var otherShopID int64
	if pool.QueryRow(ctx, "INSERT INTO public.shops(name) VALUES('Other shipping fixture') RETURNING id").Scan(&otherShopID) != nil {
		t.Fatal("fixture failed")
	}
	_, _, e = owners.CreateOwner(ctx, otherShopID, "other@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	otherLogin, e := owners.Login(ctx, "other@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	otherSettings, e := shops.ReadShipping(ctx, otherLogin.Token.Reveal())
	if e != nil || len(otherSettings.Services) != 0 {
		t.Fatal("shipping shop isolation")
	}
	s.Services = []delivery.Service{standard, tracked}
	s, e = shops.SaveShipping(ctx, token, s)
	if e != nil {
		t.Fatal(e)
	}

	if _, e = pool.Exec(ctx, "ALTER TABLE public.shops ADD CONSTRAINT fixture_shipping_rollback CHECK(shipping_services::text NOT LIKE '%Reject fixture%')"); e != nil {
		t.Fatal("fixture failed")
	}
	rollback := s
	rollback.Services = append([]delivery.Service{}, s.Services...)
	rollback.Services[0].Name = "Reject fixture"
	if _, e = shops.SaveShipping(ctx, token, rollback); e == nil {
		t.Fatal("induced shipping failure")
	}
	saved, e := shops.ReadShipping(ctx, token)
	if e != nil || saved.Revision != s.Revision || saved.Services[0].Name != "Standard" {
		t.Fatal("shipping rollback failed")
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE public.shops DROP CONSTRAINT fixture_shipping_rollback"); e != nil {
		t.Fatal("fixture failed")
	}
	old := s
	old.Revision--
	if _, e = shops.SaveShipping(ctx, token, old); !errors.Is(e, shop.ErrConflict) {
		t.Fatal("stale shipping config")
	}
	create := func(name, key string, price int64, stock int64, fallback bool, minimum, maximum int) (catalog.Product, int64, string) {
		p, e := c.Save(ctx, token, 0, catalog.Input{Name: name, PricePence: price, CertificateName: "optional", CreationKey: key})
		if e != nil {
			t.Fatal(e)
		}
		m, e := c.SaveMaker(ctx, token, p.ID, catalog.MakerInput{Revision: p.Revision, MadeToOrderFallback: fallback, PreparationDaysMin: &minimum, PreparationDaysMax: &maximum, Variants: []catalog.VariantInput{{Label: "Original", SupplyMode: "stocked", StockQuantity: stock}}})
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
		return p, m.Variants[0].ID, strings.Split(pub.PublicPath, "/")[2]
	}
	p, v, key := create("Threshold fixture", "00000000-0000-4000-8000-000000000001", 3000, 1, true, 3, 5)
	guest := strings.Repeat("c", 64)
	basket, e := c.ChangeBasket(ctx, key, guest, catalog.BasketChange{ProductID: p.ID, VariantID: v, Quantity: 1})
	if e != nil || basket.DispatchDaysMax != 2 {
		t.Fatal("stocked dispatch")
	}
	basket, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: basket.Revision, ServiceID: standard.ID, ShippingRevision: s.Revision})
	if e != nil || !basket.ShippingSelected || basket.ShippingPence != 0 || basket.GrandTotalPence != 3000 {
		t.Fatal("free standard choice")
	}
	basket, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: basket.Revision, ServiceID: tracked.ID, ShippingRevision: s.Revision})
	if e != nil || basket.GrandTotalPence != 3599 {
		t.Fatal("paid upgrade choice")
	}
	basket, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: basket.Revision, LineID: basket.Lines[0].ID, OwnerName: "", Quantity: 2})
	if e != nil || basket.DispatchDaysMin != 3 || basket.DispatchDaysMax != 5 || !basket.Complete {
		t.Fatal("partial stock uses product preparation override")
	}
	// A separate personalised line must share the same physical stock count.
	basket, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: basket.Revision, ProductID: p.ID, VariantID: v, Quantity: 1, OwnerName: "Different owner"})
	if e != nil || basket.DispatchDaysMax != 5 {
		t.Fatal("combined demand")
	}
	m, e := c.ReadMaker(ctx, token, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	inputs := []catalog.VariantInput{m.Variants[0].VariantInput}
	_, e = c.SaveMaker(ctx, token, p.ID, catalog.MakerInput{Revision: m.Revision, Variants: inputs, MadeToOrderFallback: false})
	if e != nil {
		t.Fatal(e)
	}
	blocked, e := c.ReadBasket(ctx, key, guest)
	if e != nil || blocked.Complete || !blocked.Lines[0].Editable {
		t.Fatal("insufficient stock not blocked/editable")
	}
	if _, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: blocked.Revision, ServiceID: standard.ID, ShippingRevision: s.Revision}); e == nil {
		t.Fatal("shipping chosen for blocked basket")
	}
	// Changing preparation with omitted bounds preserves the existing override.
	m, e = c.ReadMaker(ctx, token, p.ID)
	if e != nil || m.PreparationDaysMin != 3 || m.PreparationDaysMax != 5 {
		t.Fatal("preparation overwrite")
	}
	_, e = c.SaveMaker(ctx, token, p.ID, catalog.MakerInput{Revision: m.Revision, Variants: inputs, MadeToOrderFallback: true})
	if e != nil {
		t.Fatal(e)
	}
	s, e = shops.ReadShipping(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	s.Services[1].FeePence = 899
	s.Services[1].DaysMax = 4
	s, e = shops.SaveShipping(ctx, token, s)
	if e != nil {
		t.Fatal(e)
	}
	changed, e := c.ReadBasket(ctx, key, guest)
	if e != nil || !changed.ShippingChanged || changed.ShippingPence != 899 || changed.SelectedShipping.DaysMax != 4 {
		t.Fatal("shipping reprice/details not surfaced")
	}
	if _, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: changed.Revision, ServiceID: tracked.ID, ShippingRevision: s.Revision - 1}); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("stale delivery quote accepted")
	}
	s.Services[1].Enabled = false
	s, e = shops.SaveShipping(ctx, token, s)
	if e != nil {
		t.Fatal(e)
	}
	disabled, e := c.ReadBasket(ctx, key, guest)
	if e != nil || disabled.ShippingSelected || len(disabled.ShippingOptions) != 1 {
		t.Fatal("disabled shipping available")
	}
	if _, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: disabled.Revision, ServiceID: tracked.ID, ShippingRevision: s.Revision}); e == nil {
		t.Fatal("disabled shipping chosen")
	}
	if _, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: disabled.Revision, ServiceID: "00000000000000000000000000000003", ShippingRevision: s.Revision}); e == nil {
		t.Fatal("unknown shipping chosen")
	}
	pub, e := c.ReadPublication(ctx, token, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.SavePublication(ctx, token, p.ID, catalog.PublicationInput{Revision: pub.Revision, Publish: false})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.PublicList(ctx, key, 0); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("withdrawn catalogue visible")
	}
	retained, e := c.ReadBasket(ctx, key, guest)
	if e != nil || retained.Complete || len(retained.Lines) != 2 {
		t.Fatal("last-product withdrawal hid owned basket")
	}
	for len(retained.Lines) > 0 {
		retained, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: retained.Revision, LineID: retained.Lines[0].ID, Quantity: 0})
		if e != nil {
			t.Fatal("withdrawn line removal failed")
		}
	}
}

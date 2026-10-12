package cataloghttp_test

import (
	"context"
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
	"github.com/techize/selloovy/internal/cataloghttp"
	"github.com/techize/selloovy/internal/square"
)

func TestOwnerOrdersDispatchAndImmutableFinancialEvidence(t *testing.T) {
	pool, c, key, owner, review := paymentFixture(t, "made_to_order")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) { return capturedFixture(), nil }}
	payments := financialService(t, c, f)
	if _, e = payments.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-owner-order", "192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	// A separate saved checkout remains visible without recovery emails or payment.
	review(strings.Repeat("b", 64))
	page, e := c.ListOrders(ctx, owner, "")
	if e != nil || len(page.Orders) != 1 || len(page.Unfinished) != 1 || page.Orders[0].FulfilmentState != "awaiting_dispatch" {
		t.Fatal("owner list missing paid or unfinished checkout")
	}
	order, e := c.ReadOrder(ctx, owner, prep.ID)
	if e != nil || order.Customer.Email != "customer@example.com" || order.Snapshot.Lines[0].OwnerName != "Test owner" || order.Allocations[0].ProductionQuantity != 1 || order.Fulfilment.Revision != 1 || len(order.Notifications) != 2 {
		t.Fatal("saved order details lost")
	}
	data, _ := json.Marshal(order)
	for _, private := range []string{"cnon:", "SourceCipher", "sourceCipher", "IdempotencyKey", prep.IdempotencyKey, "BasketDigest"} {
		if strings.Contains(string(data), private) {
			t.Fatal("owner view exported payment secrets")
		}
	}
	for _, in := range []catalog.DispatchInput{{Revision: 1, TrackingURL: "javascript:alert(1)"}, {Revision: 1, TrackingURL: "https://user:password@example.com/track"}, {Revision: 1, Carrier: "invalid\ncarrier"}} {
		if _, e = c.SaveDispatch(ctx, owner, prep.ID, in); e == nil {
			t.Fatal("unsafe shipment accepted")
		}
	}
	shipment, e := c.SaveDispatch(ctx, owner, prep.ID, catalog.DispatchInput{Revision: 1, Carrier: "Example Carrier", TrackingReference: "FIXTURE-123", TrackingURL: "https://carrier.example.com/track/FIXTURE-123"})
	if e != nil || shipment.Fulfilment.State != "dispatched" || shipment.Fulfilment.DispatchedAt == nil {
		t.Fatal("dispatch not recorded")
	}
	firstDate := *shipment.Fulfilment.DispatchedAt
	if _, e = c.SaveDispatch(ctx, owner, prep.ID, catalog.DispatchInput{Revision: 1}); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("stale dispatch form accepted")
	}
	shipment, e = c.SaveDispatch(ctx, owner, prep.ID, catalog.DispatchInput{Revision: shipment.Fulfilment.Revision, Carrier: "Updated Carrier"})
	if e != nil || !shipment.Fulfilment.DispatchedAt.Equal(firstDate) || shipment.Fulfilment.Revision != 3 {
		t.Fatal("tracking edit rewrote original dispatch time")
	}
	if _, e = pool.Exec(ctx, "UPDATE order_fulfilment SET state='awaiting_dispatch',revision=revision+1,dispatched_at=NULL"); e == nil {
		t.Fatal("dispatched order reopened")
	}
	var paid bool
	var jobs int
	if pool.QueryRow(ctx, "SELECT state='paid' AND total_pence=2498 AND customer->>'name'='Test Buyer' FROM payment_attempts").Scan(&paid) != nil || !paid {
		t.Fatal("dispatch changed financial evidence")
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM order_notifications").Scan(&jobs) != nil || jobs != 2 {
		t.Fatal("dispatch created duplicate receipts")
	}
	if _, e = c.ListOrders(ctx, "invalid-token", ""); !errors.Is(e, auth.ErrCredential) {
		t.Fatal("expired/invalid owner read allowed")
	}
	receipt, e := payments.GuestView(ctx, key, guest, prep.ID)
	if e != nil || receipt.Fulfilment == nil || receipt.Fulfilment.State != "dispatched" || receipt.Fulfilment.Carrier != "Updated Carrier" {
		t.Fatal("customer receipt missed shipment update")
	}
	if _, e = pool.Exec(ctx, "DELETE FROM baskets"); e != nil {
		t.Fatal(e)
	}
	if _, e = c.ReadOrder(ctx, owner, prep.ID); e != nil {
		t.Fatal("order lost after terminal basket cleanup")
	}
}

func TestOwnerUncertainPaymentReconciliationAndCrossShopGuards(t *testing.T) {
	pool, c, key, owner, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) {
		return square.Result{State: square.Unknown}, nil
	}, get: func(_ context.Context, id string, q square.Request) (square.Result, error) {
		if id != "fixture-payment" {
			return square.Result{State: square.Unknown}, nil
		}
		if q.Reference != prep.ID || q.AmountPence != 2498 {
			t.Error("operator changed financial intent")
		}
		return capturedFixture(), nil
	}}
	payments := financialService(t, c, f)
	if _, e = payments.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-operator", "192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	var shopID int64
	if pool.QueryRow(ctx, "INSERT INTO shops(name) VALUES('Other fixture') RETURNING id").Scan(&shopID) != nil {
		t.Fatal("other shop fixture failed")
	}
	if _, _, e = owners.CreateOwner(ctx, shopID, "other@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	login, e := owners.Login(ctx, "other@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	other := login.Token.Reveal()
	if _, e = c.ReadOrder(ctx, other, prep.ID); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("foreign owner read allowed")
	}
	if _, e = c.SaveDispatch(ctx, other, prep.ID, catalog.DispatchInput{Revision: 1}); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("foreign owner dispatch allowed")
	}
	if _, e = payments.ReconcileOwned(ctx, other, prep.ID, "fixture-payment"); !errors.Is(e, catalog.ErrNotFound) || f.reads.Load() != 0 {
		t.Fatal("foreign owner reconciliation contacted provider")
	}
	// Fixture-only time travel to prove all-shop attention counts and filtering.
	if _, e = pool.Exec(ctx, "ALTER TABLE payment_attempts DISABLE TRIGGER payment_attempt_immutable; UPDATE payment_attempts SET created_at=clock_timestamp()-interval '6 minutes'; ALTER TABLE payment_attempts ENABLE TRIGGER payment_attempt_immutable"); e != nil {
		t.Fatal(e)
	}
	attention, e := c.ListAttentionOrders(ctx, owner, "")
	if e != nil || attention.NeedsAttentionCount != 1 || len(attention.Orders) != 1 || !attention.Orders[0].NeedsAttention {
		t.Fatal("uncertain payment hidden from owner attention view")
	}
	if _, e = c.SaveDispatch(ctx, owner, prep.ID, catalog.DispatchInput{Revision: 1}); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("uncertain order dispatched")
	}
	if _, e = pool.Exec(ctx, "INSERT INTO order_fulfilment(attempt_id) SELECT id FROM payment_attempts"); e == nil {
		t.Fatal("database allowed unpaid fulfilment")
	}
	if _, e = payments.ReconcileOwned(ctx, owner, prep.ID, "wrong-provider-id"); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("unmatched Square observation accepted")
	}
	if state, e := payments.ReconcileOwned(ctx, owner, prep.ID, "fixture-payment"); e != nil || state.State != "paid" || f.calls.Load() != 1 {
		t.Fatal("verified operator recovery failed or charged again")
	}
	guard, e := authhttp.New(owners, "https://shop.example.com")
	if e != nil {
		t.Fatal(e)
	}
	routes := chi.NewRouter()
	routes.Mount("/orders", cataloghttp.NewOrders(c, payments))
	handler := guard.ProtectAdmin(routes)
	send := func(method, path, body, token, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://shop.example.com"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Selloovy-Request", "owner-auth")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "__Host-selloovy_session", Value: token})
		}
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		return out
	}
	if out := send("GET", "/orders/", "", "", ""); out.Code != 401 {
		t.Fatal("anonymous orders exposed")
	}
	if out := send("GET", "/orders/"+prep.ID, "", other, ""); out.Code != 404 || strings.Contains(out.Body.String(), "customer@example.com") {
		t.Fatal("foreign HTTP owner exposed order")
	}
	if out := send("PUT", "/orders/"+prep.ID+"/dispatch", `{"revision":1}`, owner, "https://other.example.com"); out.Code != 403 {
		t.Fatal("cross-origin shipment allowed")
	}
	if out := send("PUT", "/orders/"+prep.ID+"/dispatch", `{"revision":1,"state":"paid"}`, owner, "https://shop.example.com"); out.Code != 400 {
		t.Fatal("browser financial state accepted")
	}
	if out := send("GET", "/orders/"+prep.ID, "", owner, ""); out.Code != 200 || !strings.Contains(out.Body.String(), `"productionQuantity":0`) {
		t.Fatal("owned HTTP detail unavailable")
	}
	if out := send("GET", "/orders/?after="+prep.ID, "", other, ""); out.Code != 404 {
		t.Fatal("foreign pagination cursor accepted")
	}
	if e = owners.Logout(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if _, e = c.SaveDispatch(ctx, owner, prep.ID, catalog.DispatchInput{Revision: 1}); !errors.Is(e, auth.ErrCredential) {
		t.Fatal("revoked session dispatched order")
	}
}

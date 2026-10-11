package cataloghttp_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/delivery"
	"github.com/techize/selloovy/internal/shop"
)

func paymentFixture(t *testing.T, mode string) (*pgxpool.Pool, *catalog.Store, string, string, func(string) catalog.Checkout) {
	t.Helper()
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = owners.CreateFirstOwner(ctx, "Payment fixture", "owner@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	login, e := owners.Login(ctx, "owner@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	owner := login.Token.Reveal()
	c := catalog.NewStore(pool)
	p, e := c.Save(ctx, owner, 0, catalog.Input{Name: "Fixture dragon", PricePence: 1999, CertificateName: "optional", CreationKey: "00000000-0000-4000-8000-000000000001"})
	if e != nil {
		t.Fatal(e)
	}
	stock := int64(1)
	if mode == "made_to_order" {
		stock = 0
	}
	m, e := c.SaveMaker(ctx, owner, p.ID, catalog.MakerInput{Revision: p.Revision, Variants: []catalog.VariantInput{{Label: "Amber", SupplyMode: mode, StockQuantity: stock}}})
	if e != nil {
		t.Fatal(e)
	}
	pub, e := c.ReadPublication(ctx, owner, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	pub, e = c.SavePublication(ctx, owner, p.ID, catalog.PublicationInput{Revision: m.Revision, ShopRevision: pub.ShopRevision, Publish: true})
	if e != nil {
		t.Fatal(e)
	}
	key := strings.Split(pub.PublicPath, "/")[2]
	shops := shop.NewStore(pool)
	shipping, e := shops.ReadShipping(ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	shipping.Services = []delivery.Service{{ID: strings.Repeat("0", 31) + "1", Name: "Standard", FeePence: 499, DaysMin: 3, DaysMax: 5, Enabled: true}}
	shipping, e = shops.SaveShipping(ctx, owner, shipping)
	if e != nil {
		t.Fatal(e)
	}
	prepare := func(guest string) catalog.Checkout {
		t.Helper()
		b, e := c.ChangeBasket(ctx, key, guest, catalog.BasketChange{ProductID: p.ID, VariantID: m.Variants[0].ID, Quantity: 1, OwnerName: "Test owner"})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: b.Revision, ServiceID: shipping.Services[0].ID, ShippingRevision: shipping.Revision}); e != nil {
			t.Fatal(e)
		}
		r, e := c.ReadCheckout(ctx, key, guest)
		if e != nil {
			t.Fatal(e)
		}
		r, e = c.SaveCheckout(ctx, key, guest, catalog.CheckoutChange{Revision: r.Revision, QuoteHash: r.QuoteHash, Customer: catalog.Customer{Email: "customer@example.com", Name: "Test Buyer", Address1: "1 Example Lane", Town: "Exampletown", Postcode: "AB1 2CD", Country: "GB"}})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	return pool, c, key, owner, prepare
}

func TestPaymentPreparationRetriesCancellationAndImmutability(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	if _, e := c.PreparePayment(ctx, key, guest, 0); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("unsaved revision accepted")
	}
	if _, e := c.PreparePayment(ctx, key, strings.Repeat("b", 64), r.Revision); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("foreign browser accepted")
	}
	start := make(chan struct{})
	results := make(chan catalog.PaymentPreparation, 2)
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			p, e := c.PreparePayment(ctx, key, guest, r.Revision)
			results <- p
			failures <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first catalog.PaymentPreparation
	for p := range results {
		if first.ID == "" {
			first = p
		} else if p.ID != first.ID || p.IdempotencyKey != first.IdempotencyKey || !p.ExpiresAt.Equal(first.ExpiresAt) {
			t.Fatal("duplicate start created different intent")
		}
	}
	if first.Snapshot.TotalPence != 2498 || first.Customer.Email != "customer@example.com" || first.Snapshot.Lines[0].OwnerName != "Test owner" {
		t.Fatal("financial snapshot lost")
	}
	var holds, attempts, stock int
	if pool.QueryRow(ctx, "SELECT count(*) FROM payment_preparations").Scan(&attempts) != nil || attempts != 1 {
		t.Fatal("multiple intents")
	}
	if pool.QueryRow(ctx, "SELECT sum(quantity) FROM checkout_stock_holds").Scan(&holds) != nil || holds != 1 {
		t.Fatal("duplicate physical allocation")
	}
	if pool.QueryRow(ctx, "SELECT stock_quantity FROM product_variants").Scan(&stock) != nil || stock != 1 {
		t.Fatal("preparation consumed stock")
	}
	restarted := catalog.NewStore(pool)
	retry, e := restarted.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil || retry.ID != first.ID || retry.IdempotencyKey != first.IdempotencyKey {
		t.Fatal("retry lost across store restart")
	}
	for _, stmt := range []string{
		"UPDATE payment_preparations SET total_pence=1",
		"UPDATE payment_preparations SET customer='{}'",
		"UPDATE payment_preparations SET snapshot='{}'",
		"UPDATE payment_preparations SET idempotency_key=repeat('0',32)",
		"UPDATE payment_preparations SET expires_at=clock_timestamp()",
		"UPDATE payment_preparations SET state='paid'",
	} {
		if _, e = pool.Exec(ctx, stmt); e == nil {
			t.Fatal("immutable or unsupported transition accepted")
		}
	}
	if e = c.CancelPaymentPreparation(ctx, key, strings.Repeat("b", 64), first.ID); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("foreign cancellation accepted")
	}
	if e = c.CancelPaymentPreparation(ctx, key, guest, first.ID); e != nil {
		t.Fatal(e)
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_stock_holds").Scan(&holds) != nil || holds != 0 {
		t.Fatal("cancellation did not release")
	}
	second, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil || second.ID == first.ID || second.IdempotencyKey == first.IdempotencyKey {
		t.Fatal("cancelled intent reopened")
	}
	if e = c.CancelPaymentPreparation(ctx, key, guest, first.ID); e != nil {
		t.Fatal(e)
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_stock_holds").Scan(&holds) != nil || holds != 1 {
		t.Fatal("old cancellation cleared newer allocation")
	}
	if _, e = pool.Exec(ctx, "UPDATE payment_preparations SET state='prepared' WHERE id=$1", first.ID); e == nil {
		t.Fatal("terminal preparation reopened")
	}
	current, e := c.ReadCheckout(ctx, key, guest)
	if e != nil {
		t.Fatal(e)
	}
	current.Customer.Name = "Changed Buyer"
	current, e = c.SaveCheckout(ctx, key, guest, catalog.CheckoutChange{Revision: current.Revision, QuoteHash: current.QuoteHash, Customer: current.Customer})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.PreparePayment(ctx, key, guest, r.Revision); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("old checkout revision reused")
	}
	third, e := c.PreparePayment(ctx, key, guest, current.Revision)
	if e != nil || third.ID == second.ID || third.Customer.Name != "Changed Buyer" {
		t.Fatal("edited checkout reused previous intent")
	}
	var originalName, oldState string
	if pool.QueryRow(ctx, "SELECT customer->>'name',state FROM payment_preparations WHERE id=$1", second.ID).Scan(&originalName, &oldState) != nil || originalName != "Test Buyer" || oldState != "cancelled" {
		t.Fatal("edited checkout rewrote old intent")
	}
	if e = c.CancelPaymentPreparation(ctx, key, guest, second.ID); e != nil {
		t.Fatal(e)
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_stock_holds").Scan(&holds) != nil || holds != 1 {
		t.Fatal("old edited intent released new allocation")
	}

}

func TestPaymentPreparationCompetingStockAndStaleQuote(t *testing.T) {
	pool, c, key, owner, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	ra, rb := review(a), review(b)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, x := range []struct {
		guest    string
		revision int64
	}{{a, ra.Revision}, {b, rb.Revision}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := c.PreparePayment(ctx, key, x.guest, x.revision)
			results <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, catalog.ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("last physical item prepared twice")
	}
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM payment_preparations").Scan(&count) != nil || count != 1 {
		t.Fatal("failed allocation left intent")
	}
	shipping, e := shop.NewStore(pool).ReadShipping(ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	shipping.Services[0].FeePence++
	if _, e = shop.NewStore(pool).SaveShipping(ctx, owner, shipping); e != nil {
		t.Fatal(e)
	}
	for _, g := range []string{a, b} {
		if _, e = c.PreparePayment(ctx, key, g, 1); !errors.Is(e, catalog.ErrConflict) {
			t.Fatal("stale shipping accepted")
		}
	}
	if pool.QueryRow(ctx, "SELECT total_pence FROM payment_preparations").Scan(&count) != nil || count != 2498 {
		t.Fatal("stale quote overwrote financial intent")
	}
}

func TestPaymentPreparationExpiryRollbackAndPendingGuards(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "made_to_order")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	first, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_stock_holds").Scan(&count) != nil || count != 0 {
		t.Fatal("made-to-order physically allocated")
	}
	// Time travel only in this disposable fixture. Production never rewrites intents.
	if _, e = pool.Exec(ctx, "ALTER TABLE payment_preparations DISABLE TRIGGER payment_preparation_immutable; UPDATE payment_preparations SET expires_at=clock_timestamp()-interval '1 second'; ALTER TABLE payment_preparations ENABLE TRIGGER payment_preparation_immutable; UPDATE checkout_reviews SET hold_expires_at=clock_timestamp()-interval '1 second'"); e != nil {
		t.Fatal("expiry fixture failed")
	}
	if _, e = pool.Exec(ctx, "CREATE FUNCTION public.fixture_reject_preparation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture insert failure'; END $$; CREATE TRIGGER fixture_reject_insert BEFORE INSERT ON payment_preparations FOR EACH ROW EXECUTE FUNCTION public.fixture_reject_preparation()"); e != nil {
		t.Fatal("rollback fixture failed")
	}
	if _, e = c.PreparePayment(ctx, key, guest, r.Revision); !errors.Is(e, catalog.ErrStorage) {
		t.Fatal("forced insert failure accepted")
	}
	var state string
	if pool.QueryRow(ctx, "SELECT state FROM payment_preparations WHERE id=$1", first.ID).Scan(&state) != nil || state != "prepared" {
		t.Fatal("failed insert committed old expiry")
	}
	var active bool
	if pool.QueryRow(ctx, "SELECT hold_expires_at>clock_timestamp() FROM checkout_reviews").Scan(&active) != nil || active {
		t.Fatal("failed insert extended deadline")
	}
	if _, e = pool.Exec(ctx, "DROP TRIGGER fixture_reject_insert ON payment_preparations; DROP FUNCTION public.fixture_reject_preparation()"); e != nil {
		t.Fatal(e)
	}
	next, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil || next.ID == first.ID || next.IdempotencyKey == first.IdempotencyKey {
		t.Fatal("expired preparation reused")
	}
	if pool.QueryRow(ctx, "SELECT state FROM payment_preparations WHERE id=$1", first.ID).Scan(&state) != nil || state != "expired" {
		t.Fatal("expiry not recorded")
	}
	if e = c.MarkCheckoutPaymentPending(ctx, key, guest, r.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = c.PreparePayment(ctx, key, guest, r.Revision); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("pending payment prepared again")
	}
	if e = c.CancelPaymentPreparation(ctx, key, guest, next.ID); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("pending payment cancelled with preauth API")
	}
}

func TestPaymentPreparationPhysicalInsertFailureRollsBack(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	if _, e := pool.Exec(ctx, "ALTER TABLE payment_preparations ADD CONSTRAINT fixture_reject_insert CHECK(total_pence<1)"); e != nil {
		t.Fatal(e)
	}
	if _, e := c.PreparePayment(ctx, key, guest, r.Revision); !errors.Is(e, catalog.ErrStorage) {
		t.Fatal("forced insert failure accepted")
	}
	var count int
	for _, table := range []string{"payment_preparations", "checkout_stock_holds"} {
		if pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count) != nil || count != 0 {
			t.Fatal("failed preparation left intent/allocation")
		}
	}
	var absent bool
	if pool.QueryRow(ctx, "SELECT hold_expires_at IS NULL FROM checkout_reviews").Scan(&absent) != nil || !absent {
		t.Fatal("failed preparation set deadline")
	}
	if _, e := pool.Exec(ctx, "ALTER TABLE payment_preparations DROP CONSTRAINT fixture_reject_insert"); e != nil {
		t.Fatal(e)
	}
	if _, e := c.PreparePayment(ctx, key, guest, r.Revision); e != nil {
		t.Fatal(e)
	}
	if pool.QueryRow(ctx, "SELECT sum(quantity) FROM checkout_stock_holds").Scan(&count) != nil || count != 1 {
		t.Fatal("retry allocation incorrect")
	}
}

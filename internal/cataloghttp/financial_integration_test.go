package cataloghttp_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/paymentvault"
	"github.com/techize/selloovy/internal/square"
)

type financialProvider struct {
	charge       func(context.Context, square.Request) (square.Result, error)
	get          func(context.Context, string, square.Request) (square.Result, error)
	calls, reads atomic.Int32
}

func (*financialProvider) Location() string { return "fixture-location" }
func (f *financialProvider) Charge(ctx context.Context, r square.Request) (square.Result, error) {
	f.calls.Add(1)
	return f.charge(ctx, r)
}
func (f *financialProvider) Reconcile(ctx context.Context, id string, r square.Request) (square.Result, error) {
	f.reads.Add(1)
	return f.get(ctx, id, r)
}
func financialService(t *testing.T, c *catalog.Store, f *financialProvider) *catalog.Payments {
	t.Helper()
	v, e := paymentvault.New(make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	p, e := catalog.NewPayments(c, v, f)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func capturedFixture() square.Result {
	return square.Result{State: square.Paid, ProviderID: "fixture-payment", CapturedAt: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)}
}

func TestFinancialCommitBeforeChargeAndConcurrentDuplicate(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	f := &financialProvider{charge: func(ctx context.Context, q square.Request) (square.Result, error) {
		var ready bool
		if pool.QueryRow(ctx, `SELECT c.payment_pending AND h.retained AND a.state='submitting' AND a.source_cipher IS NOT NULL AND a.total_pence=2498 AND a.idempotency_key=$2 FROM payment_attempts a JOIN checkout_reviews c ON c.id=a.checkout_id JOIN checkout_stock_holds h ON h.checkout_id=c.id WHERE a.id=$1`, q.Reference, q.IdempotencyKey).Scan(&ready) != nil || !ready {
			t.Error("provider called before durable intent and allocation")
		}
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return square.Result{}, ctx.Err()
		}
		return capturedFixture(), nil
	}}
	p := financialService(t, c, f)
	done := make(chan error, 1)
	go func() {
		result, e := p.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-first")
		if e == nil && result.State != "paid" {
			e = errors.New("not paid")
		}
		done <- e
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("charge not reached")
	}
	duplicate, e := p.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-other")
	if e != nil || duplicate.State != "submitting" || f.calls.Load() != 1 {
		t.Fatal("duplicate submitted another charge")
	}
	if _, e = p.Submit(ctx, key, strings.Repeat("b", 64), r.Revision, prep.ID, "cnon:fixture-foreign"); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("foreign browser accepted")
	}
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if result, e := p.Recover(ctx, prep.ID); e != nil || result.State != "paid" {
			t.Fatal("terminal recovery failed")
		}
	}
	var stock, jobs, holds int
	var cipherGone, completed bool
	var paidAt time.Time
	if pool.QueryRow(ctx, "SELECT stock_quantity FROM product_variants").Scan(&stock) != nil || stock != 0 {
		t.Fatal("stock not consumed once")
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM order_notifications").Scan(&jobs) != nil || jobs != 2 {
		t.Fatal("notifications not queued once")
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_stock_holds").Scan(&holds) != nil || holds != 0 {
		t.Fatal("paid hold retained")
	}
	if pool.QueryRow(ctx, "SELECT source_cipher IS NULL,paid_at FROM payment_attempts").Scan(&cipherGone, &paidAt) != nil || !cipherGone || !paidAt.Equal(capturedFixture().CapturedAt) {
		t.Fatal("source retained or provider capture time lost")
	}
	if pool.QueryRow(ctx, "SELECT completed FROM checkout_reviews").Scan(&completed) != nil || !completed {
		t.Fatal("checkout not completed")
	}
	if f.calls.Load() != 1 || f.reads.Load() != 0 {
		t.Fatal("terminal payment contacted provider again")
	}
	if _, e = c.PreparePayment(ctx, key, guest, r.Revision); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("paid checkout prepared again")
	}
	if _, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: 1, LineID: "fixture", Quantity: 1}); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("paid basket edited")
	}
	if status, e := p.ReadOwned(ctx, key, guest, prep.ID); e != nil || status.State != "paid" {
		t.Fatal("owned result unavailable")
	}
	for _, stmt := range []string{"UPDATE payment_attempts SET total_pence=1", "UPDATE payment_attempts SET customer='{}'", "UPDATE payment_attempts SET state='unknown'", "UPDATE payment_attempts SET provider_id='other-provider'", "UPDATE payment_attempts SET source_cipher=decode(repeat('00',28),'hex')"} {
		if _, e = pool.Exec(ctx, stmt); e == nil {
			t.Fatal("financial evidence rewritten")
		}
	}
	// Completed evidence survives removal of expired customer baskets.
	if _, e = pool.Exec(ctx, "DELETE FROM baskets"); e != nil {
		t.Fatal("terminal evidence prevented basket cleanup")
	}
	if pool.QueryRow(ctx, "SELECT source_cipher IS NULL AND checkout_id IS NULL FROM payment_attempts").Scan(&cipherGone) != nil || !cipherGone {
		t.Fatal("financial record lost with basket")
	}
}

func TestFinancialDeclineRetryAndSourceReplay(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) {
		return square.Result{State: square.Declined}, nil
	}}
	p := financialService(t, c, f)
	if out, e := p.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-declined"); e != nil || out.State != "failed" {
		t.Fatal("decline not recorded")
	}
	var stock, holds int
	var cleared bool
	if pool.QueryRow(ctx, "SELECT stock_quantity FROM product_variants").Scan(&stock) != nil || stock != 1 {
		t.Fatal("decline consumed stock")
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_stock_holds").Scan(&holds) != nil || holds != 0 {
		t.Fatal("decline did not release holds")
	}
	if pool.QueryRow(ctx, "SELECT source_cipher IS NULL FROM payment_attempts").Scan(&cleared) != nil || !cleared {
		t.Fatal("declined source retained")
	}
	next, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil || next.ID == prep.ID {
		t.Fatal("fresh intent unavailable after definitive decline")
	}
	if _, e = p.Submit(ctx, key, guest, r.Revision, next.ID, "cnon:fixture-declined"); !errors.Is(e, catalog.ErrConflict) || f.calls.Load() != 1 {
		t.Fatal("same nonce replay contacted provider")
	}
	if out, e := p.Submit(ctx, key, guest, r.Revision, next.ID, "cnon:fixture-new"); e != nil || out.State != "failed" || f.calls.Load() != 2 {
		t.Fatal("fresh token retry failed")
	}
}

func TestFinancialUnknownRecoveryAndAtomicRollback(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) {
		return square.Result{State: square.Unknown, ProviderID: "fixture-payment"}, nil
	}, get: func(_ context.Context, id string, r square.Request) (square.Result, error) {
		if id != "fixture-payment" || r.AmountPence != 2498 || r.Reference != prep.ID {
			t.Error("recovery changed intent")
		}
		return capturedFixture(), nil
	}}
	p := financialService(t, c, f)
	if out, e := p.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-unknown"); e != nil || out.State != "unknown" {
		t.Fatal("uncertain state lost")
	}
	if _, e = pool.Exec(ctx, "DELETE FROM baskets"); e == nil {
		t.Fatal("unresolved evidence detached")
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE order_notifications ADD CONSTRAINT fixture_fail CHECK(kind='impossible')"); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Recover(ctx, prep.ID); !errors.Is(e, catalog.ErrStorage) {
		t.Fatal("forced outbox failure accepted")
	}
	var state string
	var stock, holds, jobs int
	if pool.QueryRow(ctx, "SELECT state FROM payment_attempts").Scan(&state) != nil || state != "unknown" {
		t.Fatal("partial paid outcome committed")
	}
	if pool.QueryRow(ctx, "SELECT stock_quantity FROM product_variants").Scan(&stock) != nil || stock != 1 {
		t.Fatal("failed commit consumed stock")
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM checkout_stock_holds WHERE retained").Scan(&holds) != nil || holds != 1 {
		t.Fatal("uncertain hold released")
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM order_notifications").Scan(&jobs) != nil || jobs != 0 {
		t.Fatal("partial notifications committed")
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE order_notifications DROP CONSTRAINT fixture_fail"); e != nil {
		t.Fatal(e)
	}
	if out, e := p.Recover(ctx, prep.ID); e != nil || out.State != "paid" {
		t.Fatal("reconciliation could not finish rolled-back order")
	}
	if f.calls.Load() != 1 || f.reads.Load() != 2 {
		t.Fatal("recovery sent another charge")
	}
}

func TestFinancialNoProviderIDAndExpiredSourceCleanup(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "made_to_order")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) {
		return square.Result{}, errors.New("synthetic transport failure")
	}}
	p := financialService(t, c, f)
	if out, e := p.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-timeout"); e != nil || out.State != "unknown" {
		t.Fatal("transport failure not retained")
	}
	if out, e := p.Recover(ctx, prep.ID); e != nil || out.State != "unknown" || f.calls.Load() != 1 || f.reads.Load() != 0 {
		t.Fatal("unknown identity automatically retried")
	}
	if n, e := p.EraseExpiredSources(ctx); e != nil || n != 0 {
		t.Fatal("source erased prematurely")
	}
	// Fixture-only clock change; real service never modifies expiry or immutable intent.
	if _, e = pool.Exec(ctx, "ALTER TABLE payment_attempts DISABLE TRIGGER payment_attempt_immutable; UPDATE payment_attempts SET source_expires_at=clock_timestamp()-interval '1 second'; ALTER TABLE payment_attempts ENABLE TRIGGER payment_attempt_immutable"); e != nil {
		t.Fatal(e)
	}
	if n, e := p.EraseExpiredSources(ctx); e != nil || n != 1 {
		t.Fatal("expired envelope not removed")
	}
	var retained bool
	var production int
	if pool.QueryRow(ctx, "SELECT payment_pending FROM checkout_reviews").Scan(&retained) != nil || !retained {
		t.Fatal("expiry released uncertain payment")
	}
	if pool.QueryRow(ctx, "SELECT production_quantity FROM payment_allocations").Scan(&production) != nil || production != 1 {
		t.Fatal("production allocation lost")
	}
}

func TestFinancialPaidObservationSurvivesLocalCommitFailure(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE order_notifications ADD CONSTRAINT fixture_fail CHECK(kind='impossible')"); e != nil {
		t.Fatal(e)
	}
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) { return capturedFixture(), nil }, get: func(context.Context, string, square.Request) (square.Result, error) { return capturedFixture(), nil }}
	p := financialService(t, c, f)
	if _, e = p.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-commit-failure"); !errors.Is(e, catalog.ErrStorage) {
		t.Fatal("forced local failure accepted")
	}
	var saved bool
	if pool.QueryRow(ctx, "SELECT provider_id='fixture-payment' AND state='submitting' AND source_cipher IS NOT NULL FROM payment_attempts").Scan(&saved) != nil || !saved {
		t.Fatal("local failure erased provider identity")
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE order_notifications DROP CONSTRAINT fixture_fail; DELETE FROM product_publications"); e != nil {
		t.Fatal(e)
	}
	if out, e := p.Recover(ctx, prep.ID); e != nil || out.State != "paid" || f.calls.Load() != 1 || f.reads.Load() != 1 {
		t.Fatal("unpublished product prevented charged order recovery")
	}
}

func TestFinancialProductionOrderAndExpiredPreparation(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "made_to_order")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) { return capturedFixture(), nil }}
	p := financialService(t, c, f)
	if _, e = pool.Exec(ctx, "UPDATE checkout_reviews SET hold_expires_at=clock_timestamp()-interval '1 second'"); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-expired"); !errors.Is(e, catalog.ErrConflict) || f.calls.Load() != 0 {
		t.Fatal("expired allocation reached processor")
	}
	next, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if out, e := p.Submit(ctx, key, guest, r.Revision, next.ID, "cnon:fixture-production"); e != nil || out.State != "paid" {
		t.Fatal("production order failed")
	}
	var production, physical, stock int
	if pool.QueryRow(ctx, "SELECT production_quantity,physical_quantity FROM payment_allocations").Scan(&production, &physical) != nil || production != 1 || physical != 0 {
		t.Fatal("production allocation wrong")
	}
	if pool.QueryRow(ctx, "SELECT stock_quantity FROM product_variants").Scan(&stock) != nil || stock != 0 {
		t.Fatal("production order consumed unavailable stock")
	}
}

func TestFinancialPartialStockFallback(t *testing.T) {
	pool, c, key, owner, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	maker, e := c.ReadMaker(ctx, owner, 1)
	if e != nil {
		t.Fatal(e)
	}
	variants := []catalog.VariantInput{{ID: maker.Variants[0].ID, Label: "Amber", SupplyMode: "stocked", StockQuantity: 1}}
	maker, e = c.SaveMaker(ctx, owner, 1, catalog.MakerInput{Revision: maker.Revision, Variants: variants, MadeToOrderFallback: true})
	if e != nil {
		t.Fatal(e)
	}
	pub, e := c.ReadPublication(ctx, owner, 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.SavePublication(ctx, owner, 1, catalog.PublicationInput{Revision: maker.Revision, ShopRevision: pub.ShopRevision, Publish: true}); e != nil {
		t.Fatal(e)
	}
	guest := strings.Repeat("a", 64)
	r := review(guest)
	b, e := c.ReadBasket(ctx, key, guest)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.ChangeBasket(ctx, key, guest, catalog.BasketChange{Revision: b.Revision, LineID: b.Lines[0].ID, Quantity: 2}); e != nil {
		t.Fatal(e)
	}
	current, e := c.ReadCheckout(ctx, key, guest)
	if e != nil {
		t.Fatal(e)
	}
	r, e = c.SaveCheckout(ctx, key, guest, catalog.CheckoutChange{Revision: current.Revision, QuoteHash: current.QuoteHash, Customer: r.Customer})
	if e != nil {
		t.Fatal(e)
	}
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) { return capturedFixture(), nil }}
	p := financialService(t, c, f)
	if out, e := p.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-partial"); e != nil || out.State != "paid" {
		t.Fatal("partial fallback failed")
	}
	var physical, production, stock int
	if pool.QueryRow(ctx, "SELECT physical_quantity,production_quantity FROM payment_allocations").Scan(&physical, &production) != nil || physical != 1 || production != 1 {
		t.Fatal("mixed allocation wrong")
	}
	if pool.QueryRow(ctx, "SELECT stock_quantity FROM product_variants").Scan(&stock) != nil || stock != 0 {
		t.Fatal("partial fallback over-consumed")
	}
}

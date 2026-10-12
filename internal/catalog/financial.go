package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/techize/selloovy/internal/catalogdb"
	"github.com/techize/selloovy/internal/paymentvault"
	"github.com/techize/selloovy/internal/square"
)

type PaymentProvider interface {
	Location() string
	Charge(context.Context, square.Request) (square.Result, error)
	Reconcile(context.Context, string, square.Request) (square.Result, error)
}
type Payments struct {
	s        *Store
	vault    *paymentvault.Vault
	provider PaymentProvider
}
type PaymentState struct{ ID, State string }

func NewPayments(s *Store, v *paymentvault.Vault, p PaymentProvider) (*Payments, error) {
	if s == nil || v == nil || p == nil {
		return nil, ErrStorage
	}
	return &Payments{s: s, vault: v, provider: p}, nil
}

// Submit has no HTTP caller yet. It commits the intent/retained allocation before
// exactly one provider call. Duplicates return the saved state without resubmission.
func (p *Payments) Submit(ctx context.Context, key, token string, revision int64, id, source string) (PaymentState, error) {
	a, newAttempt, e := p.begin(ctx, key, token, revision, id, source)
	if e != nil {
		return PaymentState{}, e
	}
	if !newAttempt {
		return PaymentState{a.ID, a.State}, nil
	}
	request := square.Request{SourceToken: source, IdempotencyKey: a.IdempotencyKey, Reference: a.ID, AmountPence: a.TotalPence}
	result, e := p.provider.Charge(ctx, request)
	if e != nil {
		result = square.Result{State: square.Unknown}
	}
	// A customer disconnect must not prevent saving an already-observed result.
	done, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.resolve(done, a.ID, result)
}
func (p *Payments) begin(ctx context.Context, key, token string, revision int64, id, source string) (catalogdb.PaymentAttempt, bool, error) {
	fail := func(e error) (catalogdb.PaymentAttempt, bool, error) { return catalogdb.PaymentAttempt{}, false, e }
	if !publicKeyPattern.MatchString(id) {
		return fail(ErrNotFound)
	}
	tx, q, r, b, e := p.s.lockCheckout(ctx, key, token, false)
	if e != nil {
		return fail(e)
	}
	defer cleanupTransaction(tx)
	digest, _ := basketDigest(token)
	prior, e := q.ReadPaymentAttempt(ctx, id)
	if e == nil {
		if !prior.CheckoutID.Valid || prior.CheckoutID.String != r.ID || prior.Revision != revision {
			return fail(ErrNotFound)
		}
		if tx.Commit(ctx) != nil {
			return fail(ErrStorage)
		}
		return prior, false, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return fail(ErrStorage)
	}
	if r.PaymentPending || r.Completed || r.Revision != revision || revision < 1 {
		return fail(ErrConflict)
	}
	c, e := checkoutFromBasket(b, catalogdb.ReadCheckoutRow(r))
	if e != nil {
		return fail(e)
	}
	if !c.Ready || c.Changed || c.Saved == nil {
		return fail(ErrConflict)
	}
	prep, e := q.OwnedPaymentPreparation(ctx, catalogdb.OwnedPaymentPreparationParams{CheckoutID: r.ID, ID: id})
	if errors.Is(e, pgx.ErrNoRows) {
		return fail(ErrNotFound)
	}
	if e != nil {
		return fail(ErrStorage)
	}
	if prep.State != "prepared" || !prep.Active || prep.CheckoutRevision != revision || prep.QuoteHash != r.QuoteHash || !prep.ExpiresAt.Time.Equal(r.HoldExpiresAt.Time) {
		return fail(ErrConflict)
	}
	shopID, e := q.LockCheckoutShop(ctx, key)
	if e != nil {
		return fail(ErrStorage)
	}
	envelope, e := p.vault.Seal(paymentvault.Context{ShopID: shopID, AmountPence: prep.TotalPence, AttemptID: id, IdempotencyKey: prep.IdempotencyKey, Location: p.provider.Location(), QuoteHash: r.QuoteHash}, source)
	if e != nil {
		return fail(ErrConflict)
	}
	n, e := q.BeginFinancialPending(ctx, r.ID)
	if e != nil {
		return fail(basketWriteError(e))
	}
	if n != 1 {
		return fail(ErrConflict)
	}
	if e = q.RetainCheckoutHolds(ctx, r.ID); e != nil {
		return fail(basketWriteError(e))
	}
	holds, e := q.CheckoutPhysicalHolds(ctx, r.ID)
	if e != nil {
		return fail(ErrStorage)
	}
	hash := sha256.Sum256([]byte(source))
	a, e := q.CreatePaymentAttempt(ctx, catalogdb.CreatePaymentAttemptParams{ID: id, ShopID: shopID, CheckoutID: pgtype.Text{String: r.ID, Valid: true}, BasketDigest: digest, Revision: revision, IdempotencyKey: prep.IdempotencyKey, LocationID: p.provider.Location(), QuoteHash: r.QuoteHash, Customer: r.Customer, Snapshot: r.Snapshot, TotalPence: prep.TotalPence, SourceDigest: hash[:], SourceCipher: envelope})
	if e != nil {
		var databaseError *pgconn.PgError
		if errors.As(e, &databaseError) && databaseError.Code == "23505" {
			return fail(ErrConflict)
		}
		return fail(basketWriteError(e))
	}
	physical := map[int64]int64{}
	for _, h := range holds {
		physical[h.VariantID] = h.Quantity
	}
	demand := map[int64]int64{}
	for _, line := range c.Saved.Lines {
		demand[line.VariantID] += line.Quantity
	}
	for variant, quantity := range demand {
		if physical[variant] > quantity {
			return fail(ErrStorage)
		}
		if e = q.AddPaymentAllocation(ctx, catalogdb.AddPaymentAllocationParams{AttemptID: id, VariantID: variant, PhysicalQuantity: physical[variant], ProductionQuantity: quantity - physical[variant]}); e != nil {
			return fail(basketWriteError(e))
		}
	}
	if e = q.EndPaymentPreparation(ctx, catalogdb.EndPaymentPreparationParams{ID: id, State: "submitted"}); e != nil {
		return fail(basketWriteError(e))
	}
	if tx.Commit(ctx) != nil {
		return fail(ErrStorage)
	}
	return a, true, nil
}

// Recover never sends another charge. A known provider ID is reconciled via GET;
// unknown-ID outcomes require operator reconciliation rather than guessing.
func (p *Payments) Recover(ctx context.Context, id string) (PaymentState, error) {
	a, e := catalogdb.New(p.s.pool).ReadPaymentAttempt(ctx, id)
	if e != nil {
		return PaymentState{}, ErrNotFound
	}
	if a.State == "paid" || a.State == "failed" {
		return PaymentState{a.ID, a.State}, nil
	}
	if !a.ProviderID.Valid || a.LocationID != p.provider.Location() {
		return PaymentState{a.ID, a.State}, nil
	}
	result, e := p.provider.Reconcile(ctx, a.ProviderID.String, square.Request{Reference: a.ID, AmountPence: a.TotalPence, IdempotencyKey: a.IdempotencyKey})
	if e != nil {
		result = square.Result{State: square.Unknown}
	}
	return p.resolve(ctx, id, result)
}
func (p *Payments) resolve(ctx context.Context, id string, result square.Result) (PaymentState, error) {
	base := catalogdb.New(p.s.pool)
	initial, e := base.ReadPaymentAttempt(ctx, id)
	if e != nil {
		return PaymentState{}, ErrStorage
	}
	if initial.State == "paid" || initial.State == "failed" {
		return PaymentState{initial.ID, initial.State}, nil
	}
	if initial.LocationID != p.provider.Location() {
		return PaymentState{}, ErrConflict
	}
	// Save the trusted provider identity independently; stock/outbox rollback must
	// not erase the identity needed to reconcile a charge that already occurred.
	if result.ProviderID != "" {
		if initial.ProviderID.Valid && initial.ProviderID.String != result.ProviderID {
			return PaymentState{}, ErrConflict
		}
		n, attachErr := base.AttachPaymentProviderID(ctx, catalogdb.AttachPaymentProviderIDParams{ID: id, ProviderID: pgtype.Text{String: result.ProviderID, Valid: true}})
		if attachErr != nil {
			return PaymentState{}, ErrStorage
		}
		if n != 1 {
			return PaymentState{}, ErrConflict
		}
	}
	tx, e := p.s.pool.Begin(ctx)
	if e != nil {
		return PaymentState{}, ErrStorage
	}
	defer cleanupTransaction(tx)
	q := catalogdb.New(tx)
	if initial.CheckoutID.Valid {
		if _, e = q.LockPaymentBasket(ctx, initial.CheckoutID.String); e != nil {
			return PaymentState{}, ErrStorage
		}
		if _, e = q.LockPaymentReview(ctx, initial.CheckoutID.String); e != nil {
			return PaymentState{}, ErrStorage
		}
	}
	a, e := q.LockPaymentAttempt(ctx, id)
	if e != nil {
		return PaymentState{}, ErrStorage
	}
	if a.State == "paid" || a.State == "failed" {
		return PaymentState{a.ID, a.State}, nil
	}
	if !a.CheckoutID.Valid {
		return PaymentState{}, ErrStorage
	}
	state := "unknown"
	paid := pgtype.Timestamptz{}
	switch result.State {
	case square.Paid:
		if result.ProviderID == "" || result.CapturedAt.IsZero() {
			return PaymentState{}, ErrStorage
		}
		state = "paid"
		paid = pgtype.Timestamptz{Time: result.CapturedAt.UTC(), Valid: true}
	case square.Declined:
		state = "failed"
	case square.Pending:
		state = "pending"
	}
	if state == "paid" {
		products, e := q.LockPaymentProducts(ctx, id)
		if e != nil {
			return PaymentState{}, ErrStorage
		}
		for _, product := range products {
			if _, e = q.LockPaymentProduct(ctx, product); e != nil {
				return PaymentState{}, ErrStorage
			}
		}
		if _, e = q.LockPaymentVariants(ctx, id); e != nil {
			return PaymentState{}, ErrStorage
		}
		expected, e := q.PhysicalPaymentVariantCount(ctx, id)
		if e != nil {
			return PaymentState{}, ErrStorage
		}
		consumed, e := q.ConsumePaymentStock(ctx, id)
		if e != nil || consumed != expected {
			return PaymentState{}, ErrStorage
		}
		if e = q.TouchPaidProductRevisions(ctx, id); e != nil {
			return PaymentState{}, ErrStorage
		}
		if e = q.CompletePaidCheckout(ctx, a.CheckoutID.String); e != nil {
			return PaymentState{}, ErrStorage
		}
		if e = q.QueueOrderNotifications(ctx, id); e != nil {
			return PaymentState{}, ErrStorage
		}
	}
	if state == "failed" {
		if e = q.FailFinancialCheckout(ctx, a.CheckoutID.String); e != nil {
			return PaymentState{}, ErrStorage
		}
	}
	if state == "paid" || state == "failed" {
		if e = q.ReleaseFinancialHolds(ctx, a.CheckoutID.String); e != nil {
			return PaymentState{}, ErrStorage
		}
	}
	if e = q.SetPaymentOutcome(ctx, catalogdb.SetPaymentOutcomeParams{ID: id, State: state, PaidAt: paid}); e != nil {
		return PaymentState{}, ErrStorage
	}
	if tx.Commit(ctx) != nil {
		return PaymentState{}, ErrStorage
	}
	return PaymentState{id, state}, nil
}

// ReadOwned exposes no source token, envelope or provider credentials.
func (p *Payments) ReadOwned(ctx context.Context, key, token, id string) (PaymentState, error) {
	if !publicKeyPattern.MatchString(id) {
		return PaymentState{}, ErrNotFound
	}
	c, e := p.s.ReadCheckout(ctx, key, token)
	if e != nil {
		return PaymentState{}, e
	}
	a, e := catalogdb.New(p.s.pool).ReadPaymentAttempt(ctx, id)
	if e != nil {
		return PaymentState{}, ErrNotFound
	}
	d, e := basketDigest(token)
	if e != nil || !equalDigest(d, a.BasketDigest) || a.Revision > c.Revision {
		return PaymentState{}, ErrNotFound
	}
	// ReadCheckout verified shop/cookie scope; the locked checkout ID must also
	// match before this route can report an attempt from another shop.
	owned, e := catalogdb.New(p.s.pool).ReadCheckout(ctx, catalogdb.ReadCheckoutParams{PublicKey: key, BasketDigest: d})
	if e != nil || !a.CheckoutID.Valid || owned.ID != a.CheckoutID.String {
		return PaymentState{}, ErrNotFound
	}
	return PaymentState{id, a.State}, nil
}
func equalDigest(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var n byte
	for i := range a {
		n |= a[i] ^ b[i]
	}
	return n == 0
}

// IntentSnapshot is used by the owner order view later, never public catalogue.
func intentSnapshot(a catalogdb.PaymentAttempt) (CheckoutSnapshot, error) {
	var s CheckoutSnapshot
	if json.Unmarshal(a.Snapshot, &s) != nil {
		return s, ErrStorage
	}
	return s, nil
}

// EraseExpiredSources removes at most 1,000 expired encrypted tokens per call.
// Financial evidence and uncertain allocations remain available for reconciliation.
func (p *Payments) EraseExpiredSources(ctx context.Context) (int64, error) {
	n, err := catalogdb.New(p.s.pool).EraseExpiredPaymentSources(ctx)
	if err != nil {
		return 0, ErrStorage
	}
	return n, nil
}

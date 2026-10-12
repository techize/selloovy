package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/techize/selloovy/internal/catalogdb"
)

// PaymentPreparation is a pre-authorization intent, not a paid order or permission
// to submit to a provider. The sandbox browser flow exposes only its owned view.
type PaymentPreparation struct {
	ID, IdempotencyKey string
	CheckoutRevision   int64
	Customer           Customer
	Snapshot           CheckoutSnapshot
	ExpiresAt          time.Time
}

func preparationView(r catalogdb.ActivePaymentPreparationRow) (PaymentPreparation, error) {
	p := PaymentPreparation{ID: r.ID, IdempotencyKey: r.IdempotencyKey, CheckoutRevision: r.CheckoutRevision, ExpiresAt: r.ExpiresAt.Time}
	if json.Unmarshal(r.Customer, &p.Customer) != nil || json.Unmarshal(r.Snapshot, &p.Snapshot) != nil || p.Snapshot.Currency != r.Currency || p.Snapshot.TotalPence != r.TotalPence {
		return PaymentPreparation{}, ErrStorage
	}
	return p, nil
}

// PreparePayment atomically saves the agreed intent and allocates physical stock.
// Serialized retries reuse the ID, key and original deadline. No provider call
// is made and no browser-supplied amount, customer snapshot or key is accepted.
func (s *Store) PreparePayment(ctx context.Context, key, token string, revision int64) (PaymentPreparation, error) {
	tx, q, stored, b, e := s.lockCheckout(ctx, key, token, false)
	if e != nil {
		return PaymentPreparation{}, e
	}
	defer cleanupTransaction(tx)
	if stored.PaymentPending || stored.Completed || revision < 1 || stored.Revision != revision {
		return PaymentPreparation{}, ErrConflict
	}
	c, e := checkoutFromBasket(b, catalogdb.ReadCheckoutRow(stored))
	if e != nil {
		return PaymentPreparation{}, e
	}
	if !c.Ready || c.Changed || c.Saved == nil {
		return PaymentPreparation{}, ErrConflict
	}
	prior, e := q.ActivePaymentPreparation(ctx, stored.ID)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return PaymentPreparation{}, ErrStorage
	}
	if e == nil {
		held, e := q.CheckoutHoldActive(ctx, stored.ID)
		if e != nil {
			return PaymentPreparation{}, ErrStorage
		}
		if prior.Active && held && prior.CheckoutRevision == revision && prior.QuoteHash == c.QuoteHash && prior.ExpiresAt.Time.Equal(stored.HoldExpiresAt.Time) {
			result, e := preparationView(prior)
			if e != nil {
				return PaymentPreparation{}, e
			}
			if tx.Commit(ctx) != nil {
				return PaymentPreparation{}, ErrStorage
			}
			return result, nil
		}
		state := "cancelled"
		if !prior.Active {
			state = "expired"
		}
		if e = q.EndPaymentPreparation(ctx, catalogdb.EndPaymentPreparationParams{ID: prior.ID, State: state}); e != nil {
			return PaymentPreparation{}, basketWriteError(e)
		}
		if e = q.ClearCheckoutHolds(ctx, stored.ID); e != nil {
			return PaymentPreparation{}, basketWriteError(e)
		}
		if e = q.ClearCheckoutHoldDeadline(ctx, stored.ID); e != nil {
			return PaymentPreparation{}, basketWriteError(e)
		}
	}
	deadline, e := allocateCheckout(ctx, q, key, stored, b)
	if e != nil {
		return PaymentPreparation{}, e
	}
	var ids [32]byte
	if _, e = rand.Read(ids[:]); e != nil {
		return PaymentPreparation{}, ErrStorage
	}
	row, e := q.CreatePaymentPreparation(ctx, catalogdb.CreatePaymentPreparationParams{ID: hex.EncodeToString(ids[:16]), CheckoutID: stored.ID, CheckoutRevision: revision, QuoteHash: c.QuoteHash, IdempotencyKey: hex.EncodeToString(ids[16:]), Customer: stored.Customer, Snapshot: stored.Snapshot, Currency: c.Saved.Currency, TotalPence: c.Saved.TotalPence, ExpiresAt: deadline})
	if e != nil {
		return PaymentPreparation{}, basketWriteError(e)
	}
	result, e := preparationView(catalogdb.ActivePaymentPreparationRow(row))
	if e != nil {
		return PaymentPreparation{}, e
	}
	if tx.Commit(ctx) != nil {
		return PaymentPreparation{}, ErrStorage
	}
	return result, nil
}

// CancelPaymentPreparation only releases allocations for the matching owned,
// still-prepared intent. Repeating cancellation cannot clear a newer allocation.
// It must never be used as a financial failure/reconciliation API.
func (s *Store) CancelPaymentPreparation(ctx context.Context, key, token, id string) error {
	if !publicKeyPattern.MatchString(id) {
		return ErrNotFound
	}
	tx, q, stored, _, e := s.lockCheckout(ctx, key, token, false)
	if e != nil {
		return e
	}
	defer cleanupTransaction(tx)
	if stored.PaymentPending || stored.Completed {
		return ErrConflict
	}
	p, e := q.OwnedPaymentPreparation(ctx, catalogdb.OwnedPaymentPreparationParams{CheckoutID: stored.ID, ID: id})
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return ErrStorage
	}
	if p.State == "prepared" {
		state := "cancelled"
		if !p.Active {
			state = "expired"
		}
		if e = q.EndPaymentPreparation(ctx, catalogdb.EndPaymentPreparationParams{ID: id, State: state}); e != nil {
			return basketWriteError(e)
		}
		// A checkout edit may have cleared this preparation's holds and a separate
		// server-only hold start may have replaced them. Preserve that newer deadline.
		if p.ExpiresAt.Time.Equal(stored.HoldExpiresAt.Time) {
			if e = q.ClearCheckoutHolds(ctx, stored.ID); e != nil {
				return basketWriteError(e)
			}
			if e = q.ClearCheckoutHoldDeadline(ctx, stored.ID); e != nil {
				return basketWriteError(e)
			}
		}
	}
	if tx.Commit(ctx) != nil {
		return ErrStorage
	}
	return nil
}

package catalog

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/techize/selloovy/internal/catalogdb"
	"github.com/techize/selloovy/internal/delivery"
)

type Customer struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Address1 string `json:"address1"`
	Address2 string `json:"address2"`
	Town     string `json:"town"`
	Postcode string `json:"postcode"`
	Country  string `json:"country"`
}
type CheckoutSnapshot struct {
	Shop                                     PublicShop
	Currency                                 string
	BasketRevision, ShippingRevision         int64
	Lines                                    []BasketLine
	Shipping                                 delivery.Service
	SubtotalPence, ShippingPence, TotalPence int64
	DispatchDaysMin, DispatchDaysMax         int
}
type Checkout struct {
	Revision                                  int64
	Customer                                  Customer
	Saved                                     *CheckoutSnapshot
	Current                                   CheckoutSnapshot
	QuoteHash                                 string
	PaymentID                                 string
	Ready, Changed, PaymentPending, Completed bool
}
type CheckoutChange struct {
	Revision  int64
	QuoteHash string
	Customer  Customer
}

var postcodeText = regexp.MustCompile(`^[A-Z0-9 ]{5,10}$`)

func validateCustomer(c Customer) (Customer, error) {
	c.Email = strings.TrimSpace(c.Email)
	c.Name = strings.TrimSpace(c.Name)
	c.Address1 = strings.TrimSpace(c.Address1)
	c.Address2 = strings.TrimSpace(c.Address2)
	c.Town = strings.TrimSpace(c.Town)
	c.Postcode = strings.ToUpper(strings.Join(strings.Fields(c.Postcode), " "))
	fields := map[string]string{}
	for _, f := range []struct {
		name, value string
		required    bool
		max         int
	}{{"email", c.Email, true, 254}, {"name", c.Name, true, 120}, {"address1", c.Address1, true, 160}, {"address2", c.Address2, false, 160}, {"town", c.Town, true, 120}, {"postcode", c.Postcode, true, 10}} {
		invalid := !utf8.ValidString(f.value) || utf8.RuneCountInString(f.value) > f.max || (f.required && f.value == "")
		for _, r := range f.value {
			if unicode.IsControl(r) {
				invalid = true
			}
		}
		if invalid {
			fields[f.name] = "Enter plain text within the displayed limit."
		}
	}
	address, e := mail.ParseAddress(c.Email)
	if e != nil || address.Address != c.Email {
		fields["email"] = "Enter a plain email address."
	}
	if !postcodeText.MatchString(c.Postcode) {
		fields["postcode"] = "Enter your UK postcode."
	}
	if c.Country != "GB" {
		fields["country"] = "Delivery is currently available to UK addresses only."
	}
	if len(fields) > 0 {
		return Customer{}, &ValidationError{Fields: fields}
	}
	return c, nil
}
func snapshot(b Basket) CheckoutSnapshot {
	lines := append([]BasketLine{}, b.Lines...)
	for i := range lines {
		lines[i].Changed = false
		lines[i].Editable = false
	}
	return CheckoutSnapshot{Shop: b.Shop, Currency: "GBP", BasketRevision: b.Revision, ShippingRevision: b.ShippingRevision, Lines: lines, Shipping: b.SelectedShipping, SubtotalPence: b.TotalPence, ShippingPence: b.ShippingPence, TotalPence: b.GrandTotalPence, DispatchDaysMin: b.DispatchDaysMin, DispatchDaysMax: b.DispatchDaysMax}
}
func quoteHash(s CheckoutSnapshot) (string, error) {
	data, e := json.Marshal(s)
	if e != nil {
		return "", ErrStorage
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func ownCheckoutID(ctx context.Context, q *catalogdb.Queries, key string, d []byte) (string, error) {
	r, e := q.ReadCheckout(ctx, catalogdb.ReadCheckoutParams{PublicKey: key, BasketDigest: d})
	if errors.Is(e, pgx.ErrNoRows) {
		return "", nil
	}
	if e != nil {
		return "", ErrStorage
	}
	return r.ID, nil
}
func checkoutFromBasket(b Basket, r catalogdb.ReadCheckoutRow) (Checkout, error) {
	c := Checkout{Revision: r.Revision, Current: snapshot(b), PaymentPending: r.PaymentPending, Completed: r.Completed, PaymentID: r.PaymentID, Ready: !r.Completed && !r.PaymentPending && b.Complete && len(b.Lines) > 0 && b.ShippingSelected}
	var e error
	c.QuoteHash, e = quoteHash(c.Current)
	if e != nil {
		return Checkout{}, e
	}
	if r.Revision > 0 {
		var saved CheckoutSnapshot
		if json.Unmarshal(r.Customer, &c.Customer) != nil || json.Unmarshal(r.Snapshot, &saved) != nil {
			return Checkout{}, ErrStorage
		}
		c.Saved = &saved
		c.Changed = r.QuoteHash != c.QuoteHash
	}
	return c, nil
}
func (s *Store) ReadCheckout(ctx context.Context, key, token string) (Checkout, error) {
	if !publicKeyPattern.MatchString(key) {
		return Checkout{}, ErrNotFound
	}
	d, e := basketDigest(token)
	if e != nil {
		return Checkout{}, e
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return Checkout{}, ErrStorage
	}
	defer cleanupTransaction(tx)
	q := catalogdb.New(tx)
	row, e := q.ReadBasket(ctx, catalogdb.ReadBasketParams{PublicKey: key, Digest: d})
	if errors.Is(e, pgx.ErrNoRows) {
		return Checkout{}, ErrNotFound
	}
	if e != nil {
		return Checkout{}, ErrStorage
	}
	items, e := decodeBasket(row.Lines)
	if e != nil {
		return Checkout{}, e
	}
	stored, e := q.ReadCheckout(ctx, catalogdb.ReadCheckoutParams{PublicKey: key, BasketDigest: d})
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return Checkout{}, ErrStorage
	}
	b, e := resolveBasket(ctx, q, key, row.Revision, items, row.ShippingChoice, stored.ID)
	if e != nil {
		return Checkout{}, e
	}
	c, e := checkoutFromBasket(b, stored)
	if e != nil {
		return Checkout{}, e
	}
	if tx.Commit(ctx) != nil {
		return Checkout{}, ErrStorage
	}
	return c, nil
}

// lockCheckout serializes a basket, its saved review, shipping, products and variant allocations.
// All checkout writers take these locks in this order; products and variants lock by ascending ID.
func (s *Store) lockCheckout(ctx context.Context, key, token string, create bool) (pgx.Tx, *catalogdb.Queries, catalogdb.LockCheckoutRow, Basket, error) {
	fail := func(tx pgx.Tx, e error) (pgx.Tx, *catalogdb.Queries, catalogdb.LockCheckoutRow, Basket, error) {
		if tx != nil {
			cleanupTransaction(tx)
		}
		return nil, nil, catalogdb.LockCheckoutRow{}, Basket{}, e
	}
	if !publicKeyPattern.MatchString(key) {
		return fail(nil, ErrNotFound)
	}
	d, e := basketDigest(token)
	if e != nil {
		return fail(nil, e)
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return fail(nil, ErrStorage)
	}
	q := catalogdb.New(tx)
	row, e := q.LockBasket(ctx, catalogdb.LockBasketParams{PublicKey: key, Digest: d})
	if errors.Is(e, pgx.ErrNoRows) {
		return fail(tx, ErrNotFound)
	}
	if e != nil {
		return fail(tx, basketWriteError(e))
	}
	if create {
		if e = q.PurgeOwnedExpiredCheckout(ctx, catalogdb.PurgeOwnedExpiredCheckoutParams{PublicKey: key, BasketDigest: d}); e != nil {
			return fail(tx, basketWriteError(e))
		}
		var id [16]byte
		if _, e = rand.Read(id[:]); e != nil {
			return fail(tx, ErrStorage)
		}
		if e = q.CreateCheckout(ctx, catalogdb.CreateCheckoutParams{PublicKey: key, Digest: d, ID: hex.EncodeToString(id[:])}); e != nil {
			return fail(tx, basketWriteError(e))
		}
	}
	stored, e := q.LockCheckout(ctx, catalogdb.LockCheckoutParams{PublicKey: key, BasketDigest: d})
	if errors.Is(e, pgx.ErrNoRows) {
		return fail(tx, ErrNotFound)
	}
	if e != nil {
		return fail(tx, basketWriteError(e))
	}
	if _, e = q.LockCheckoutShop(ctx, key); e != nil {
		return fail(tx, basketWriteError(e))
	}
	items, e := decodeBasket(row.Lines)
	if e != nil {
		return fail(tx, e)
	}
	productIDs, variantIDs := []int64{}, []int64{}
	for _, line := range items {
		productIDs = append(productIDs, line.ProductID)
		variantIDs = append(variantIDs, line.VariantID)
	}
	if _, e = q.LockCheckoutProducts(ctx, catalogdb.LockCheckoutProductsParams{PublicKey: key, ProductIds: productIDs}); e != nil {
		return fail(tx, basketWriteError(e))
	}
	if _, e = q.LockCheckoutVariants(ctx, catalogdb.LockCheckoutVariantsParams{PublicKey: key, VariantIds: variantIDs}); e != nil {
		return fail(tx, basketWriteError(e))
	}
	b, e := resolveBasket(ctx, q, key, row.Revision, items, row.ShippingChoice, stored.ID)
	if e != nil {
		return fail(tx, e)
	}
	return tx, q, stored, b, nil
}
func (s *Store) SaveCheckout(ctx context.Context, key, token string, in CheckoutChange) (Checkout, error) {
	customer, e := validateCustomer(in.Customer)
	if e != nil {
		return Checkout{}, e
	}
	if in.Revision < 0 || len(in.QuoteHash) != 64 {
		return Checkout{}, basketValidation("Reload checkout before saving.")
	}
	tx, q, stored, b, e := s.lockCheckout(ctx, key, token, true)
	if e != nil {
		return Checkout{}, e
	}
	defer cleanupTransaction(tx)
	if stored.Completed {
		return Checkout{}, basketValidation("This order is paid. Its details cannot be edited.")
	}
	if stored.PaymentPending {
		return Checkout{}, basketValidation("Payment is being reconciled. Your details cannot be changed yet.")
	}
	if stored.Revision != in.Revision {
		return Checkout{}, ErrConflict
	}
	c, e := checkoutFromBasket(b, catalogdb.ReadCheckoutRow(stored))
	if e != nil {
		return Checkout{}, e
	}
	if !c.Ready {
		return Checkout{}, basketValidation("Resolve unavailable items and choose delivery in your basket first.")
	}
	if c.QuoteHash != in.QuoteHash {
		return Checkout{}, ErrConflict
	}
	data, _ := json.Marshal(customer)
	review, _ := json.Marshal(c.Current)
	if e = q.ClearCheckoutHolds(ctx, stored.ID); e != nil {
		return Checkout{}, basketWriteError(e)
	}
	if e = q.SaveCheckout(ctx, catalogdb.SaveCheckoutParams{ID: stored.ID, Customer: data, Snapshot: review, QuoteHash: c.QuoteHash}); e != nil {
		return Checkout{}, basketWriteError(e)
	}
	d, _ := basketDigest(token)
	if e = q.TouchCheckoutBasket(ctx, catalogdb.TouchCheckoutBasketParams{PublicKey: key, Digest: d}); e != nil {
		return Checkout{}, basketWriteError(e)
	}
	if tx.Commit(ctx) != nil {
		return Checkout{}, ErrStorage
	}
	c.Revision++
	c.Customer = customer
	c.Saved = &c.Current
	c.Changed = false
	return c, nil
}

// HoldCheckout is a server-only payment-start primitive. No customer HTTP route calls it yet.
// Physical stock is held for 15 minutes; made-to-order units are not physical allocations.
func (s *Store) HoldCheckout(ctx context.Context, key, token string, revision int64) error {
	tx, q, stored, b, e := s.lockCheckout(ctx, key, token, false)
	if e != nil {
		return e
	}
	defer cleanupTransaction(tx)
	if stored.PaymentPending || stored.Completed {
		return basketValidation("A payment is already pending.")
	}
	if stored.Revision != revision || revision < 1 {
		return ErrConflict
	}
	c, e := checkoutFromBasket(b, catalogdb.ReadCheckoutRow(stored))
	if e != nil {
		return e
	}
	if !c.Ready || c.Changed {
		return ErrConflict
	}
	if _, e = allocateCheckout(ctx, q, key, stored, b); e != nil {
		return e
	}
	if tx.Commit(ctx) != nil {
		return ErrStorage
	}
	return nil
}

// allocateCheckout shares the caller's transaction and lock order. Retries never
// extend an active allocation; cancellation clears it before another preparation.
func allocateCheckout(ctx context.Context, q *catalogdb.Queries, key string, stored catalogdb.LockCheckoutRow, b Basket) (pgtype.Timestamptz, error) {
	active, e := q.CheckoutHoldActive(ctx, stored.ID)
	if e != nil {
		return pgtype.Timestamptz{}, ErrStorage
	}
	if active {
		return stored.HoldExpiresAt, nil
	}
	if e = q.ClearCheckoutHolds(ctx, stored.ID); e != nil {
		return pgtype.Timestamptz{}, basketWriteError(e)
	}
	deadline, e := q.SetCheckoutHoldDeadline(ctx, stored.ID)
	if e != nil {
		return pgtype.Timestamptz{}, basketWriteError(e)
	}
	demand := map[int64]int64{}
	for _, line := range b.Lines {
		demand[line.VariantID] += line.Quantity
	}
	ids := []int64{}
	for id := range demand {
		ids = append(ids, id)
	}
	variants, e := q.LockCheckoutVariants(ctx, catalogdb.LockCheckoutVariantsParams{PublicKey: key, VariantIds: ids})
	if e != nil {
		return pgtype.Timestamptz{}, basketWriteError(e)
	}
	for _, v := range variants {
		if v.SupplyMode != "stocked" {
			continue
		}
		held, e := q.HeldQuantity(ctx, catalogdb.HeldQuantityParams{VariantID: v.ID, CheckoutID: stored.ID})
		if e != nil {
			return pgtype.Timestamptz{}, ErrStorage
		}
		available := v.StockQuantity - held
		if available < 0 {
			available = 0
		}
		quantity := demand[v.ID]
		if quantity > available {
			quantity = available
		}
		if quantity > 0 {
			if e = q.AddCheckoutHold(ctx, catalogdb.AddCheckoutHoldParams{CheckoutID: stored.ID, VariantID: v.ID, Quantity: quantity, ExpiresAt: deadline}); e != nil {
				return pgtype.Timestamptz{}, basketWriteError(e)
			}
		}
	}
	return deadline, nil
}

// MarkCheckoutPaymentPending is only a low-level allocation-protection primitive.
// It does not establish authentication or a durable financial submission boundary.
// Retained allocations cannot expire while the outcome is unknown. No HTTP caller exists.
func (s *Store) MarkCheckoutPaymentPending(ctx context.Context, key, token string, revision int64) error {
	tx, q, stored, b, e := s.lockCheckout(ctx, key, token, false)
	if e != nil {
		return e
	}
	defer cleanupTransaction(tx)
	if stored.PaymentPending || stored.Completed {
		return ErrConflict
	}
	if stored.Revision != revision || revision < 1 {
		return ErrConflict
	}
	active, e := q.CheckoutHoldActive(ctx, stored.ID)
	if e != nil {
		return ErrStorage
	}
	c, e := checkoutFromBasket(b, catalogdb.ReadCheckoutRow(stored))
	if e != nil {
		return e
	}
	if !active || !c.Ready || c.Changed {
		return ErrConflict
	}
	if e = q.RetainCheckoutHolds(ctx, stored.ID); e != nil {
		return basketWriteError(e)
	}
	if e = q.MarkCheckoutPending(ctx, stored.ID); e != nil {
		return basketWriteError(e)
	}
	if tx.Commit(ctx) != nil {
		return ErrStorage
	}
	return nil
}

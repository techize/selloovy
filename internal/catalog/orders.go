package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/catalogdb"
	"github.com/techize/selloovy/internal/shopdb"
	"github.com/techize/selloovy/internal/square"
)

type OrderSummary struct {
	ID              string     `json:"id"`
	State           string     `json:"state"`
	CustomerName    string     `json:"customerName"`
	CustomerEmail   string     `json:"customerEmail"`
	TotalPence      int64      `json:"totalPence"`
	CreatedAt       time.Time  `json:"createdAt"`
	PaidAt          *time.Time `json:"paidAt"`
	NeedsAttention  bool       `json:"needsAttention"`
	ProviderKnown   bool       `json:"providerKnown"`
	FulfilmentState string     `json:"fulfilmentState"`
}
type Fulfilment struct {
	Revision          int64      `json:"revision"`
	State             string     `json:"state"`
	Carrier           string     `json:"carrier"`
	TrackingReference string     `json:"trackingReference"`
	TrackingURL       string     `json:"trackingUrl"`
	DispatchedAt      *time.Time `json:"dispatchedAt"`
}
type Allocation struct {
	VariantID          int64 `json:"variantId"`
	PhysicalQuantity   int64 `json:"physicalQuantity"`
	ProductionQuantity int64 `json:"productionQuantity"`
}
type NotificationStatus struct {
	Kind  string `json:"kind"`
	State string `json:"state"`
}
type OrderDetail struct {
	OrderSummary
	Customer      Customer             `json:"customer"`
	Snapshot      CheckoutSnapshot     `json:"snapshot"`
	Fulfilment    *Fulfilment          `json:"fulfilment"`
	Allocations   []Allocation         `json:"allocations"`
	Notifications []NotificationStatus `json:"notifications"`
}
type UnfinishedCheckout struct {
	ID        string           `json:"id"`
	Customer  Customer         `json:"customer"`
	Snapshot  CheckoutSnapshot `json:"snapshot"`
	SavedAt   time.Time        `json:"savedAt"`
	Abandoned bool             `json:"abandoned"`
}
type OrdersPage struct {
	NeedsAttentionCount int64                `json:"needsAttentionCount"`
	Orders              []OrderSummary       `json:"orders"`
	NextAfter           string               `json:"nextAfter"`
	Unfinished          []UnfinishedCheckout `json:"unfinished"`
}

func (s *Store) ownerDigest(ctx context.Context, token string) ([]byte, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return nil, e
	}
	if _, e = shopdb.New(s.pool).ReadShop(ctx, d); errors.Is(e, pgx.ErrNoRows) {
		return nil, auth.ErrCredential
	} else if e != nil {
		return nil, ErrStorage
	}
	return d, nil
}
func (s *Store) ownedPayment(ctx context.Context, token, id string) (catalogdb.PaymentAttempt, error) {
	if !publicKeyPattern.MatchString(id) {
		return catalogdb.PaymentAttempt{}, ErrNotFound
	}
	d, e := s.ownerDigest(ctx, token)
	if e != nil {
		return catalogdb.PaymentAttempt{}, e
	}
	a, e := catalogdb.New(s.pool).OwnedPaymentRecord(ctx, catalogdb.OwnedPaymentRecordParams{Digest: d, ID: id})
	if errors.Is(e, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	if e != nil {
		return a, ErrStorage
	}
	return a, nil
}
func orderSummary(a catalogdb.PaymentAttempt) (OrderSummary, error) {
	var c Customer
	if json.Unmarshal(a.Customer, &c) != nil {
		return OrderSummary{}, ErrStorage
	}
	out := OrderSummary{ID: a.ID, State: a.State, CustomerName: c.Name, CustomerEmail: c.Email, TotalPence: a.TotalPence, CreatedAt: a.CreatedAt.Time, ProviderKnown: a.ProviderID.Valid, NeedsAttention: (a.State == "unknown" || a.State == "pending" || a.State == "submitting") && time.Since(a.CreatedAt.Time) > 5*time.Minute}
	if a.PaidAt.Valid {
		v := a.PaidAt.Time
		out.PaidAt = &v
	}
	return out, nil
}
func (s *Store) ListOrders(ctx context.Context, token, after string) (OrdersPage, error) {
	return s.listOrders(ctx, token, after, false)
}
func (s *Store) ListAttentionOrders(ctx context.Context, token, after string) (OrdersPage, error) {
	return s.listOrders(ctx, token, after, true)
}
func (s *Store) listOrders(ctx context.Context, token, after string, attentionOnly bool) (OrdersPage, error) {
	if after != "" && !publicKeyPattern.MatchString(after) {
		return OrdersPage{}, ErrNotFound
	}
	d, e := s.ownerDigest(ctx, token)
	if e != nil {
		return OrdersPage{}, e
	}
	if after != "" {
		if _, e = s.ownedPayment(ctx, token, after); e != nil {
			return OrdersPage{}, e
		}
	}
	q := catalogdb.New(s.pool)
	rows, e := q.ListOwnedPayments(ctx, catalogdb.ListOwnedPaymentsParams{Digest: d, AfterID: after, AttentionOnly: attentionOnly})
	if e != nil {
		return OrdersPage{}, ErrStorage
	}
	page := OrdersPage{Orders: []OrderSummary{}, Unfinished: []UnfinishedCheckout{}}
	page.NeedsAttentionCount, e = q.OwnedPaymentAttentionCount(ctx, d)
	if e != nil {
		return OrdersPage{}, ErrStorage
	}
	if len(rows) > 50 {
		rows = rows[:50]
		page.NextAfter = rows[49].ID
	}
	for _, r := range rows {
		var customer Customer
		if json.Unmarshal(r.Customer, &customer) != nil {
			return OrdersPage{}, ErrStorage
		}
		v := OrderSummary{ID: r.ID, State: r.State, CustomerName: customer.Name, CustomerEmail: customer.Email, TotalPence: r.TotalPence, CreatedAt: r.CreatedAt.Time, ProviderKnown: r.ProviderKnown, FulfilmentState: r.FulfilmentState, NeedsAttention: (r.State == "unknown" || r.State == "pending" || r.State == "submitting") && time.Since(r.CreatedAt.Time) > 5*time.Minute}
		if r.PaidAt.Valid {
			paid := r.PaidAt.Time
			v.PaidAt = &paid
		}
		page.Orders = append(page.Orders, v)
	}
	drafts, e := q.ListOwnedUnfinishedCheckouts(ctx, d)
	if e != nil {
		return OrdersPage{}, ErrStorage
	}
	for _, r := range drafts {
		v := UnfinishedCheckout{ID: r.ID, SavedAt: r.SavedAt.Time, Abandoned: time.Since(r.SavedAt.Time) > time.Hour}
		if json.Unmarshal(r.Customer, &v.Customer) != nil || json.Unmarshal(r.Snapshot, &v.Snapshot) != nil {
			return OrdersPage{}, ErrStorage
		}
		page.Unfinished = append(page.Unfinished, v)
	}
	return page, nil
}
func (s *Store) ReadOrder(ctx context.Context, token, id string) (OrderDetail, error) {
	a, e := s.ownedPayment(ctx, token, id)
	if e != nil {
		return OrderDetail{}, e
	}
	summary, e := orderSummary(a)
	if e != nil {
		return OrderDetail{}, e
	}
	v := OrderDetail{OrderSummary: summary, Allocations: []Allocation{}, Notifications: []NotificationStatus{}}
	if json.Unmarshal(a.Customer, &v.Customer) != nil || json.Unmarshal(a.Snapshot, &v.Snapshot) != nil {
		return v, ErrStorage
	}
	q := catalogdb.New(s.pool)
	allocations, e := q.ReadOrderAllocations(ctx, id)
	if e != nil {
		return v, ErrStorage
	}
	for _, row := range allocations {
		v.Allocations = append(v.Allocations, Allocation{row.VariantID, row.PhysicalQuantity, row.ProductionQuantity})
	}
	notices, e := q.ReadOrderNotifications(ctx, id)
	if e != nil {
		return v, ErrStorage
	}
	for _, row := range notices {
		v.Notifications = append(v.Notifications, NotificationStatus{row.Kind, row.State})
	}
	if a.State == "paid" {
		row, e := q.ReadOrderFulfilment(ctx, id)
		if e != nil {
			return v, ErrStorage
		}
		fulfilment := Fulfilment{Revision: row.Revision, State: row.State, Carrier: row.Carrier, TrackingReference: row.TrackingReference, TrackingURL: row.TrackingUrl}
		if row.DispatchedAt.Valid {
			date := row.DispatchedAt.Time
			fulfilment.DispatchedAt = &date
		}
		v.Fulfilment = &fulfilment
		v.FulfilmentState = fulfilment.State
	}
	return v, nil
}

type DispatchInput struct {
	Revision          int64  `json:"revision"`
	Carrier           string `json:"carrier"`
	TrackingReference string `json:"trackingReference"`
	TrackingURL       string `json:"trackingUrl"`
}

func (s *Store) SaveDispatch(ctx context.Context, token, id string, in DispatchInput) (OrderDetail, error) {
	a, e := s.ownedPayment(ctx, token, id)
	if e != nil {
		return OrderDetail{}, e
	}
	if a.State != "paid" {
		return OrderDetail{}, ErrConflict
	}
	fields := map[string]string{}
	in.Carrier = strings.TrimSpace(in.Carrier)
	in.TrackingReference = strings.TrimSpace(in.TrackingReference)
	in.TrackingURL = strings.TrimSpace(in.TrackingURL)
	for _, value := range []struct {
		name, text string
		max        int
	}{{"carrier", in.Carrier, 80}, {"trackingReference", in.TrackingReference, 120}, {"trackingUrl", in.TrackingURL, 2048}} {
		invalid := !utf8.ValidString(value.text) || utf8.RuneCountInString(value.text) > value.max
		for _, c := range value.text {
			if unicode.IsControl(c) {
				invalid = true
			}
		}
		if invalid {
			fields[value.name] = "Use a shorter value without control characters."
		}
	}
	if in.Revision < 1 {
		fields["revision"] = "Reload the order before saving."
	}
	if in.TrackingURL != "" {
		u, e := url.Parse(in.TrackingURL)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" || len(in.TrackingURL) > 2048 {
			fields["trackingUrl"] = "Use a plain HTTPS tracking link without credentials or a fragment."
		}
	}
	if len(fields) > 0 {
		return OrderDetail{}, &ValidationError{fields}
	}
	digest, e := auth.SessionDigest(token)
	if e != nil {
		return OrderDetail{}, e
	}
	n, e := catalogdb.New(s.pool).SaveOrderDispatch(ctx, catalogdb.SaveOrderDispatchParams{Digest: digest, AttemptID: id, Revision: in.Revision, Carrier: in.Carrier, TrackingReference: in.TrackingReference, TrackingUrl: in.TrackingURL})
	if e != nil {
		return OrderDetail{}, basketWriteError(e)
	}
	if n != 1 {
		return OrderDetail{}, ErrConflict
	}
	return s.ReadOrder(ctx, token, id)
}

// ReconcileOwned never accepts an owner-supplied financial outcome. A provided
// Square ID must return evidence matching the original reference/location/amount.
func (p *Payments) ReconcileOwned(ctx context.Context, token, id, providerID string) (PaymentState, error) {
	a, e := p.s.ownedPayment(ctx, token, id)
	if e != nil {
		return PaymentState{}, e
	}
	if a.State == "paid" || a.State == "failed" {
		return PaymentState{a.ID, a.State}, nil
	}
	if a.ProviderID.Valid {
		return p.Recover(ctx, id)
	}
	if providerID == "" || len(providerID) > 80 {
		return PaymentState{}, &ValidationError{map[string]string{"providerId": "Find the matching payment in Square and enter its payment ID."}}
	}
	result, e := p.provider.Reconcile(ctx, providerID, square.Request{Reference: a.ID, AmountPence: a.TotalPence, IdempotencyKey: a.IdempotencyKey})
	if e != nil || result.ProviderID != providerID {
		return PaymentState{}, ErrConflict
	}
	return p.resolve(ctx, id, result)
}

package catalog

import (
	"bytes"
	"context"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/techize/selloovy/internal/catalogdb"
)

var ErrPaymentLimited = errors.New("payment attempt limit reached; wait before trying again")

func paymentBudgetKey(root []byte) ([]byte, error) {
	if len(root) != 32 {
		return nil, ErrStorage
	}
	key, e := hkdf.Key(sha256.New, root, nil, "selloovy/payment-attempt-limits/v1", 32)
	if e != nil {
		return nil, ErrStorage
	}
	return key, nil
}

// Connection addresses come from the socket, never untrusted forwarding headers.
// Proxy deployments require a separately configured, tested trusted-proxy policy.
func (p *Payments) takeBudgets(ctx context.Context, q *catalogdb.Queries, shop int64, checkout, email, peer string) error {
	address, e := netip.ParseAddr(peer)
	if e != nil || address.Zone() != "" {
		return ErrConflict
	}
	address = address.Unmap()
	// An IPv6 /64 prevents inexpensive per-address rotation within one connection network.
	network := address.String()
	if address.Is6() {
		network = netip.PrefixFrom(address, 64).Masked().String()
	}
	scopes := []struct {
		kind, value string
		maximum     int32
	}{
		{"checkout", checkout, 5},
		{"email", strconv.FormatInt(shop, 10) + ":" + strings.ToLower(strings.TrimSpace(email)), 10},
		{"connection", network, 20},
	}
	type budget struct {
		fingerprint []byte
		maximum     int32
	}
	budgets := make([]budget, 0, len(scopes))
	for _, scope := range scopes {
		h := hmac.New(sha256.New, p.budgetKey)
		h.Write([]byte(scope.kind + "\x00" + scope.value))
		budgets = append(budgets, budget{h.Sum(nil), scope.maximum})
	}
	sort.Slice(budgets, func(i, j int) bool { return bytes.Compare(budgets[i].fingerprint, budgets[j].fingerprint) < 0 })
	for _, b := range budgets {
		_, e := q.TakePaymentBudget(ctx, catalogdb.TakePaymentBudgetParams{Fingerprint: b.fingerprint, Maximum: b.maximum})
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrPaymentLimited
		}
		if e != nil {
			return basketWriteError(e)
		}
	}
	return nil
}

// PurgePaymentBudgets removes old fingerprints in bounded batches. It cannot
// reset a current payment budget or remove durable financial evidence.
func (s *Store) PurgePaymentBudgets(ctx context.Context) (int64, error) {
	n, e := catalogdb.New(s.pool).PurgePaymentBudgets(ctx)
	if e != nil {
		return 0, ErrStorage
	}
	return n, nil
}

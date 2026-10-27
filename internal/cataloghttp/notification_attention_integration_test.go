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

func TestNotificationAttentionStates(t *testing.T) {
	for _, tc := range []struct {
		state       string
		stale, want bool
	}{
		{"queued", false, false}, {"sending", false, false}, {"sending", true, true}, {"validated", false, false}, {"sent", false, false}, {"failed", false, true}, {"unknown", false, true},
	} {
		t.Run(tc.state+map[bool]string{true: " stale", false: ""}[tc.stale], func(t *testing.T) {
			pool, c, key, owner, review := paymentFixture(t, "made_to_order")
			ctx := t.Context()
			guest := strings.Repeat("a", 64)
			r := review(guest)
			prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
			if e != nil {
				t.Fatal(e)
			}
			f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) { return capturedFixture(), nil }}
			if _, e = financialService(t, c, f).Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-notification-status", "192.0.2.1"); e != nil {
				t.Fatal(e)
			}
			if tc.state != "queued" {
				interval := "0 minutes"
				if tc.stale {
					interval = "3 minutes"
				}
				if _, e = pool.Exec(ctx, `UPDATE order_notifications SET state='sending',submission_key=gen_random_uuid(),payload='{}',started_at=clock_timestamp()-$1::interval WHERE kind='customer_receipt'`, interval); e != nil {
					t.Fatal(e)
				}
				if tc.state != "sending" {
					if _, e = pool.Exec(ctx, `UPDATE order_notifications SET state=$1,finished_at=clock_timestamp(),message_id='fixture-message' WHERE kind='customer_receipt'`, tc.state); e != nil {
						t.Fatal(e)
					}
				}
			}
			page, e := c.ListOrders(ctx, owner, "")
			if e != nil || len(page.Orders) != 1 || page.Orders[0].State != "paid" || page.Orders[0].NotificationNeedsAttention != tc.want || page.Orders[0].NeedsAttention || page.NeedsAttentionCount != 0 {
				t.Fatal("email outcome changed or hid payment status")
			}
			expected := int64(0)
			if tc.want {
				expected = 1
			}
			if page.NotificationAttentionCount != expected {
				t.Fatal("email attention count incorrect")
			}
			filtered, e := c.ListNotificationAttentionOrders(ctx, owner, "")
			if e != nil || int64(len(filtered.Orders)) != expected {
				t.Fatal("email filter incorrect")
			}
			paid, e := c.ListAttentionOrders(ctx, owner, "")
			if e != nil || len(paid.Orders) != 0 {
				t.Fatal("paid email issue incorrectly classified as uncertain payment")
			}
			detail, e := c.ReadOrder(ctx, owner, prep.ID)
			if e != nil || detail.State != "paid" || detail.NotificationNeedsAttention != tc.want || detail.Notifications[0].NeedsAttention != tc.want {
				t.Fatal("detail email state inconsistent")
			}
			if f.calls.Load() != 1 || f.reads.Load() != 0 {
				t.Fatal("email status caused provider side effects")
			}
		})
	}
}

func TestNotificationAttentionGlobalPaginationScopeAndHTTP(t *testing.T) {
	pool, c, key, owner, review := paymentFixture(t, "made_to_order")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) { return capturedFixture(), nil }}
	if _, e = financialService(t, c, f).Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-notification-pagination", "192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `UPDATE order_notifications SET state='sending',submission_key=gen_random_uuid(),payload='{}',started_at=clock_timestamp(); UPDATE order_notifications SET state='unknown',finished_at=clock_timestamp()`); e != nil {
		t.Fatal(e)
	}
	// Fixture-only historical clones: fifty newer orders must not hide the issue.
	if _, e = pool.Exec(ctx, `INSERT INTO payment_attempts(id,shop_id,checkout_id,basket_digest,revision,idempotency_key,location_id,quote_hash,customer,snapshot,total_pence,source_digest,source_cipher,source_expires_at,state,provider_id,created_at,paid_at)
 SELECT md5('fixture-order-'||i),a.shop_id,NULL,a.basket_digest,a.revision,md5('fixture-key-'||i),a.location_id,a.quote_hash,a.customer,a.snapshot,a.total_pence,decode(md5(i::text)||md5(i::text),'hex'),NULL,a.source_expires_at,'paid','fixture-'||i,clock_timestamp()+i*interval '1 second',a.paid_at FROM payment_attempts a CROSS JOIN generate_series(1,51) i WHERE a.id=$1`, prep.ID); e != nil {
		t.Fatal(e)
	}
	page, e := c.ListOrders(ctx, owner, "")
	if e != nil || len(page.Orders) != 50 || page.NextAfter == "" || page.NotificationAttentionCount != 1 {
		t.Fatal("global count lost older order or double counted two jobs")
	}
	for _, o := range page.Orders {
		if o.ID == prep.ID {
			t.Fatal("fixture issue was not beyond first page")
		}
	}
	issue, e := c.ListNotificationAttentionOrders(ctx, owner, "")
	if e != nil || len(issue.Orders) != 1 || issue.Orders[0].ID != prep.ID || !issue.Orders[0].NotificationNeedsAttention {
		t.Fatal("older email issue inaccessible")
	}
	tail, e := c.ListOrders(ctx, owner, page.NextAfter)
	if e != nil || len(tail.Orders) != 2 || tail.NotificationAttentionCount != 1 {
		t.Fatal("pagination lost issue count")
	}
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	var shopID int64
	if pool.QueryRow(ctx, "INSERT INTO shops(name) VALUES('Other fixture') RETURNING id").Scan(&shopID) != nil {
		t.Fatal("other fixture shop")
	}
	if _, _, e = owners.CreateOwner(ctx, shopID, "other@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	login, e := owners.Login(ctx, "other@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	other := login.Token.Reveal()
	foreign, e := c.ListNotificationAttentionOrders(ctx, other, "")
	if e != nil || foreign.NotificationAttentionCount != 0 || len(foreign.Orders) != 0 {
		t.Fatal("foreign email issue exposed")
	}
	if _, e = c.ListNotificationAttentionOrders(ctx, other, prep.ID); !errors.Is(e, catalog.ErrNotFound) {
		t.Fatal("foreign cursor accepted")
	}
	guard, e := authhttp.New(owners, "https://shop.example.com")
	if e != nil {
		t.Fatal(e)
	}
	routes := chi.NewRouter()
	routes.Mount("/orders", cataloghttp.NewOrders(c, nil))
	handler := guard.ProtectAdmin(routes)
	send := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "https://shop.example.com"+path, nil)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "__Host-selloovy_session", Value: token})
		}
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		return out
	}
	out := send("/orders/?attention=notifications", owner)
	if out.Code != 200 {
		t.Fatal("email filter HTTP unavailable")
	}
	var result catalog.OrdersPage
	if json.Unmarshal(out.Body.Bytes(), &result) != nil || len(result.Orders) != 1 || result.NotificationAttentionCount != 1 {
		t.Fatal("email filter response incorrect")
	}
	for _, private := range []string{"submissionKey", "payload", "messageId", "sourceCipher", "idempotencyKey"} {
		if strings.Contains(out.Body.String(), private) {
			t.Fatal("email or payment secrets exposed")
		}
	}
	for _, path := range []string{"/orders/?attention=notifications&attention=1", "/orders/?attention=invalid", "/orders/?attention=notifications&unknown=1"} {
		if send(path, owner).Code != 400 {
			t.Fatal("ambiguous filter accepted")
		}
	}
	if send("/orders/?attention=notifications", "").Code != 401 {
		t.Fatal("anonymous email issue list")
	}
	if e = owners.Logout(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if send("/orders/?attention=notifications", owner).Code != 401 {
		t.Fatal("revoked session email issues")
	}
}

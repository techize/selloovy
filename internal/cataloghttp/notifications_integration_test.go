package cataloghttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/techize/selloovy/internal/brevo"
	"github.com/techize/selloovy/internal/notifications"
	"github.com/techize/selloovy/internal/square"
)

func TestNotificationActivationExcludesHistoricalPaymentsAndJobs(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "made_to_order")
	ctx := t.Context()
	seed := func(guest string) string {
		r := review(guest)
		prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
		if e != nil {
			t.Fatal(e)
		}
		f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) {
			return square.Result{State: square.Paid, ProviderID: "fixture-" + guest[:1], CapturedAt: time.Now().UTC()}, nil
		}}
		if _, e = financialService(t, c, f).Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-email-"+guest[:1], "192.0.2.1"); e != nil {
			t.Fatal(e)
		}
		return prep.ID
	}
	old := seed(strings.Repeat("a", 64))
	var cutoff time.Time
	if e := pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&cutoff); e != nil {
		t.Fatal(e)
	}
	// A job created after activation for an old capture must also stay excluded.
	if _, e := pool.Exec(ctx, "DELETE FROM order_notifications WHERE attempt_id=$1 AND kind='owner_notification'", old); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, "INSERT INTO order_notifications(attempt_id,kind) VALUES($1,'owner_notification')", old); e != nil {
		t.Fatal(e)
	}
	p := &noticeProvider{call: func(context.Context, brevo.Request) (brevo.Result, error) {
		return brevo.Result{State: brevo.Accepted, MessageID: "<fixture@example.com>"}, nil
	}}
	w, e := notifications.NewSince(pool, p, cutoff)
	if e != nil {
		t.Fatal(e)
	}
	if n, e := w.RunDue(ctx); e != nil || n != 0 || p.calls.Load() != 0 {
		t.Fatal("historical notification submitted")
	}
	seed(strings.Repeat("b", 64))
	if n, e := w.RunDue(ctx); e != nil || n != 2 || p.calls.Load() != 2 {
		t.Fatal("new notifications not submitted")
	}
	newID := seed(strings.Repeat("c", 64))
	if _, e := pool.Exec(ctx, "DELETE FROM order_notifications WHERE attempt_id=$1 AND kind='owner_notification'", newID); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, "INSERT INTO order_notifications(attempt_id,kind,created_at) VALUES($1,'owner_notification',$2)", newID, cutoff.Add(-time.Second)); e != nil {
		t.Fatal(e)
	}
	if n, e := w.RunDue(ctx); e != nil || n != 1 || p.calls.Load() != 3 {
		t.Fatal("old job for new capture was not excluded")
	}
	// Recreating the worker with the same persisted activation boundary cannot replay.
	w, e = notifications.NewSince(pool, p, cutoff)
	if e != nil {
		t.Fatal(e)
	}
	if n, e := w.RunDue(ctx); e != nil || n != 0 || p.calls.Load() != 3 {
		t.Fatal("restart replayed notifications")
	}
	var queued, sent int
	if e := pool.QueryRow(ctx, "SELECT count(*) FILTER(WHERE state='queued'),count(*) FILTER(WHERE state='sent') FROM order_notifications").Scan(&queued, &sent); e != nil || queued != 3 || sent != 3 {
		t.Fatal("activation rewrote old jobs")
	}
}

type noticeProvider struct {
	call  func(context.Context, brevo.Request) (brevo.Result, error)
	calls atomic.Int32
}

func (p *noticeProvider) Send(ctx context.Context, r brevo.Request) (brevo.Result, error) {
	p.calls.Add(1)
	return p.call(ctx, r)
}
func TestNotificationCommitConcurrencyAndFinancialIsolation(t *testing.T) {
	pool, c, key, owner, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	financial := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) { return capturedFixture(), nil }}
	if _, e = financialService(t, c, financial).Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-email", "192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	var keys sync.Map
	p := &noticeProvider{call: func(ctx context.Context, r brevo.Request) (brevo.Result, error) {
		var durable bool
		if pool.QueryRow(ctx, `SELECT state='sending' AND payload->>'To'=$2 AND payload->>'IdempotencyKey'=$1 AND submission_key::text=$1 AND started_at IS NOT NULL FROM order_notifications WHERE submission_key::text=$1`, r.IdempotencyKey, r.To).Scan(&durable) != nil || !durable {
			t.Error("provider called before immutable envelope committed")
		}
		if _, exists := keys.LoadOrStore(r.IdempotencyKey, true); exists {
			t.Error("duplicate job submission")
		}
		if !strings.Contains(r.Text, "TEST ORDER") || !strings.Contains(r.Text, "£24.98") || !strings.Contains(r.Text, "Test owner") || strings.Contains(r.HTML, "<script>") {
			t.Error("receipt lost snapshot or test copy")
		}
		return brevo.Result{State: brevo.Validated, MessageID: "<fixture@example.com>"}, nil
	}}
	w, e := notifications.New(pool, p)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := w.RunDue(ctx); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if p.calls.Load() != 2 || financial.calls.Load() != 1 {
		t.Fatal("duplicate mail submission or financial side effect")
	}
	if _, e = w.RunDue(ctx); e != nil || p.calls.Load() != 2 {
		t.Fatal("completed mail replayed")
	}
	order, e := c.ReadOrder(ctx, owner, prep.ID)
	if e != nil || order.State != "paid" || len(order.Notifications) != 2 || order.Notifications[0].State != "validated" || order.Notifications[1].State != "validated" {
		t.Fatal("owner mail status unavailable")
	}
	data, _ := json.Marshal(order)
	if strings.Contains(string(data), "IdempotencyKey") || strings.Contains(string(data), "fixture@example.com") {
		t.Fatal("owner view leaked email envelope/provider identity")
	}
	var stock int64
	if pool.QueryRow(ctx, "SELECT stock_quantity FROM product_variants LIMIT 1").Scan(&stock) != nil || stock != 0 {
		t.Fatal("mail touched inventory")
	}
	for _, query := range []string{`UPDATE order_notifications SET payload='{}'`, `UPDATE order_notifications SET state='queued'`, `UPDATE order_notifications SET submission_key=gen_random_uuid()`} {
		if _, e = pool.Exec(ctx, query); e == nil {
			t.Fatal("submitted mail rewritten or requeued")
		}
	}
}
func TestNotificationUnknownFailureAndCrashNeverResend(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result brevo.Result
		err    error
		want   string
	}{
		{"uncertain", brevo.Result{State: brevo.Unknown}, nil, "unknown"}, {"explicit rejection", brevo.Result{State: brevo.Rejected}, nil, "failed"}, {"transport", brevo.Result{}, errors.New("private transport"), "unknown"}, {"accepted without identity", brevo.Result{State: brevo.Accepted}, nil, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, c, key, _, review := paymentFixture(t, "made_to_order")
			ctx := t.Context()
			guest := strings.Repeat("a", 64)
			r := review(guest)
			prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
			if e != nil {
				t.Fatal(e)
			}
			f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) { return capturedFixture(), nil }}
			if _, e = financialService(t, c, f).Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-email", "192.0.2.1"); e != nil {
				t.Fatal(e)
			}
			p := &noticeProvider{call: func(context.Context, brevo.Request) (brevo.Result, error) { return tc.result, tc.err }}
			w, _ := notifications.New(pool, p)
			if n, e := w.RunDue(ctx); e != nil || n != 2 {
				t.Fatal("notifications not processed")
			}
			if _, e = w.RunDue(ctx); e != nil || p.calls.Load() != 2 {
				t.Fatal("uncertain/failure retried")
			}
			var count int
			if pool.QueryRow(ctx, "SELECT count(*) FROM order_notifications WHERE state=$1", tc.want).Scan(&count) != nil || count != 2 {
				t.Fatal("outcome lost")
			}
		})
	}
	pool, c, key, _, review := paymentFixture(t, "made_to_order")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	prep, e := c.PreparePayment(ctx, key, guest, r.Revision)
	if e != nil {
		t.Fatal(e)
	}
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) { return capturedFixture(), nil }}
	if _, e = financialService(t, c, f).Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-crashed-email", "192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `UPDATE order_notifications SET state='sending',submission_key=gen_random_uuid(),payload='{}',started_at=clock_timestamp()-interval '3 minutes'`); e != nil {
		t.Fatal(e)
	}
	p := &noticeProvider{call: func(context.Context, brevo.Request) (brevo.Result, error) {
		t.Fatal("crashed submission resent")
		return brevo.Result{}, nil
	}}
	w, _ := notifications.New(pool, p)
	if n, e := w.RunDue(ctx); e != nil || n != 0 || p.calls.Load() != 0 {
		t.Fatal("crash handling unsafe")
	}
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM order_notifications WHERE state='unknown'").Scan(&count) != nil || count != 2 {
		t.Fatal("crashed job not held for review")
	}
}

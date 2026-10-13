// Package notifications submits durable paid-order email jobs independently of
// payment and inventory transactions. Uncertain/crashed jobs are never resent.
package notifications

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techize/selloovy/internal/brevo"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/notificationdb"
)

var ErrStorage = errors.New("notification storage unavailable")
var ErrContent = errors.New("notification snapshot unavailable")

type Provider interface {
	Send(context.Context, brevo.Request) (brevo.Result, error)
}
type Worker struct {
	pool     *pgxpool.Pool
	provider Provider
}

func New(pool *pgxpool.Pool, provider Provider) (*Worker, error) {
	if pool == nil || provider == nil {
		return nil, ErrStorage
	}
	return &Worker{pool, provider}, nil
}
func (*Worker) String() string     { return "Notification worker [private]" }
func (w *Worker) GoString() string { return w.String() }

// Payload is deliberately separate from the redacted public Brevo request.
// Its JSON is stored privately in PostgreSQL, never exported to owner/guest APIs.
type payload struct{ To, Name, Subject, HTML, Text, IdempotencyKey string }

func (payload) String() string     { return "Notification payload [private]" }
func (p payload) GoString() string { return p.String() }
func (r payload) request() brevo.Request {
	return brevo.Request{To: r.To, Name: r.Name, Subject: r.Subject, HTML: r.HTML, Text: r.Text, IdempotencyKey: r.IdempotencyKey}
}
func pounds(n int64) string { return fmt.Sprintf("£%d.%02d", n/100, n%100) }
func render(row notificationdb.LockQueuedNotificationRow, key string) (payload, error) {
	var c catalog.Customer
	var s catalog.CheckoutSnapshot
	if json.Unmarshal(row.Customer, &c) != nil || json.Unmarshal(row.Snapshot, &s) != nil || !row.PaidAt.Valid || s.Currency != "GBP" || s.TotalPence < 1 || len(s.Lines) == 0 {
		return payload{}, ErrContent
	}
	to, name := c.Email, c.Name
	heading := "Order confirmation"
	if row.Kind == "owner_notification" {
		to, name = row.OwnerEmail, ""
		heading = "New order to fulfil"
	} else if row.Kind != "customer_receipt" {
		return payload{}, ErrContent
	}
	var b strings.Builder
	fmt.Fprintf(&b, "TEST ORDER — Square sandbox; no real payment taken.\n\n%s\n%s\nOrder reference: %s\nPaid in sandbox: %s\n\n", heading, s.Shop.Name, row.AttemptID, row.PaidAt.Time.UTC().Format(time.RFC3339))
	for _, l := range s.Lines {
		fmt.Fprintf(&b, "%s — %s\n%d × %s = %s\n", l.ProductName, l.VariantLabel, l.Quantity, pounds(l.UnitPricePence), pounds(l.TotalPence))
		if l.OwnerName != "" {
			fmt.Fprintf(&b, "Certificate owner: %s\n", l.OwnerName)
		}
		fmt.Fprintf(&b, "Preparation: %d–%d working days\n\n", l.DispatchDaysMin, l.DispatchDaysMax)
	}
	fmt.Fprintf(&b, "Items: %s\nShipping: %s (%s)\nTotal: %s\n\nDeliver together to:\n%s\n%s\n%s\n%s\n%s\n%s\n", pounds(s.SubtotalPence), pounds(s.ShippingPence), s.Shipping.Name, pounds(s.TotalPence), c.Name, c.Address1, c.Address2, c.Town, c.Postcode, c.Country)
	fmt.Fprintf(&b, "\nCombined preparation: %d–%d working days. Shipping transit is additional, according to the selected service.\nThis is a test confirmation, not a VAT invoice.\n", s.DispatchDaysMin, s.DispatchDaysMax)
	text := b.String()
	return payload{to, name, "[Selloovy test] " + heading, "<!doctype html><html><body><pre style=\"font-family:system-ui,sans-serif;white-space:pre-wrap\">" + html.EscapeString(text) + "</pre></body></html>", text, key}, nil
}

// RunDue handles at most five jobs. Each submission is committed before network
// access; no locks are held during the provider call. No terminal state is retried.
func (w *Worker) RunDue(ctx context.Context) (int, error) {
	q := notificationdb.New(w.pool)
	if _, e := q.ExpireSendingNotifications(ctx); e != nil {
		return 0, ErrStorage
	}
	count := 0
	for count < 5 && ctx.Err() == nil {
		found, e := w.once(ctx)
		if e != nil {
			return count, e
		}
		if !found {
			break
		}
		count++
	}
	return count, nil
}
func (w *Worker) once(ctx context.Context) (bool, error) {
	tx, e := w.pool.Begin(ctx)
	if e != nil {
		return false, ErrStorage
	}
	defer tx.Rollback(context.Background())
	q := notificationdb.New(tx)
	row, e := q.LockQueuedNotification(ctx)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, ErrStorage
	}
	var uuid pgtype.UUID
	uuid.Valid = true
	if _, e = rand.Read(uuid.Bytes[:]); e != nil {
		return false, ErrStorage
	}
	uuid.Bytes[6] = uuid.Bytes[6]&15 | 64
	uuid.Bytes[8] = uuid.Bytes[8]&63 | 128
	key := fmt.Sprintf("%x-%x-%x-%x-%x", uuid.Bytes[:4], uuid.Bytes[4:6], uuid.Bytes[6:8], uuid.Bytes[8:10], uuid.Bytes[10:])
	p, e := render(row, key)
	if e != nil {
		return false, e
	}
	data, e := json.Marshal(p)
	if e != nil {
		return false, ErrContent
	}
	defer clear(data)
	n, e := q.StartNotification(ctx, notificationdb.StartNotificationParams{AttemptID: row.AttemptID, Kind: row.Kind, SubmissionKey: uuid, Payload: data})
	if e != nil || n != 1 {
		return false, ErrStorage
	}
	if tx.Commit(ctx) != nil {
		return false, ErrStorage
	}
	call, cancel := context.WithTimeout(ctx, 20*time.Second)
	result, sendErr := w.provider.Send(call, p.request())
	cancel()
	state := "unknown"
	message := pgtype.Text{}
	if sendErr == nil {
		switch result.State {
		case brevo.Accepted:
			state = "sent"
		case brevo.Validated:
			state = "validated"
		case brevo.Rejected:
			state = "failed"
		}
	}
	if (state == "sent" || state == "validated") && result.MessageID != "" {
		message = pgtype.Text{String: result.MessageID, Valid: true}
	} else if state == "sent" || state == "validated" {
		state = "unknown"
	}
	// Preserve observations even if shutdown/customer cancellation interrupted the
	// caller. If this write fails, the expired sending record becomes unknown.
	save, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	n, e = notificationdb.New(w.pool).FinishNotification(save, notificationdb.FinishNotificationParams{AttemptID: row.AttemptID, Kind: row.Kind, State: state, MessageID: message, SubmissionKey: uuid})
	if e != nil || n != 1 {
		return true, ErrStorage
	}
	return true, nil
}

package cataloghttp_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/delivery"
	"github.com/techize/selloovy/internal/square"
	"github.com/techize/selloovy/internal/storefronthttp"
)

func TestSandboxPaymentHTTPTrustAndReceiptRecovery(t *testing.T) {
	_, c, key, _, review := paymentFixture(t, "stocked")
	ctx := t.Context()
	guest := strings.Repeat("a", 64)
	r := review(guest)
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) { return capturedFixture(), nil }}
	p := financialService(t, c, f)
	origin := "http://localhost:8080"
	h, e := storefronthttp.NewWithPayments(c, origin, delivery.DefaultCalendar(), p, storefronthttp.SandboxCard{AppID: "sandbox-fixture", LocationID: "fixture-location"})
	if e != nil {
		t.Fatal(e)
	}
	router := chi.NewRouter()
	router.Mount("/shop", h)
	request := func(method, path, body, kind, browser string, headers map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, origin+path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "selloovy_basket", Value: browser})
		req.Header.Set("Content-Type", kind)
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Selloovy-Request", "checkout-payment")
		for k, v := range headers {
			if k == "Host" {
				req.Host = v
			} else {
				req.Header.Set(k, v)
			}
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out
	}
	hash := sha256.Sum256([]byte("selloovy-basket-form:" + guest))
	csrf := hex.EncodeToString(hash[:])
	form := url.Values{"csrf": {csrf}, "revision": {strconv.FormatInt(r.Revision, 10)}}.Encode()
	prefix := "/shop/" + key
	prepared := request("POST", prefix+"/checkout/payment", form, "application/x-www-form-urlencoded", guest, nil)
	if prepared.Code != 303 {
		t.Fatal("could not prepare from saved checkout")
	}
	path := prepared.Header().Get("Location")
	id := path[strings.LastIndex(path, "/")+1:]
	page := request("GET", path, "", "", guest, nil)
	if page.Code != 200 || !strings.Contains(page.Body.String(), `data-amount="24.98"`) || !strings.Contains(page.Body.String(), "sandbox.web.squarecdn.com/v1/square.js") || !strings.Contains(page.Header().Get("Content-Security-Policy"), "pci-connect.squareupsandbox.com") || page.Header().Get("Referrer-Policy") != "same-origin" || !strings.Contains(page.Header().Get("Content-Security-Policy"), "form-action 'self' https://api.squareupsandbox.com") {
		t.Fatal("authoritative sandbox form or CSP missing")
	}
	for _, headers := range []map[string]string{{"Host": "other.example.com"}} {
		if out := request("GET", path, "", "", guest, headers); out.Code != 404 {
			t.Fatal("wrong host exposed checkout")
		}
	}
	if out := request("GET", path, "", "", strings.Repeat("b", 64), nil); out.Code != 404 || strings.Contains(out.Body.String(), "customer@example.com") {
		t.Fatal("foreign cookie exposed customer snapshot")
	}
	if out := request("GET", path+"?unexpected=1", "", "", guest, nil); out.Code != 404 {
		t.Fatal("ambiguous payment URL accepted")
	}
	payload, _ := json.Marshal(map[string]any{"csrf": csrf, "revision": r.Revision, "source": "cnon:fixture-http"})
	for _, tc := range []struct {
		body, kind string
		headers    map[string]string
		want       int
	}{
		{string(payload), "application/json", map[string]string{"Origin": "https://other.example.com"}, 403},
		{string(payload), "application/json", map[string]string{"X-Selloovy-Request": ""}, 415},
		{strings.Replace(string(payload), csrf, "wrong", 1), "application/json", nil, 403},
		{strings.TrimSuffix(string(payload), "}") + `,"amount":1}`, "application/json", nil, 400},
		{strings.TrimSuffix(string(payload), "}") + `,"idempotency_key":"browser-choice"}`, "application/json", nil, 400},
		{strings.Repeat("x", 9000), "application/json", nil, 400},
	} {
		if out := request("POST", path, tc.body, tc.kind, guest, tc.headers); out.Code != tc.want {
			t.Fatal("payment request trust boundary failed")
		}
	}
	if f.calls.Load() != 0 {
		t.Fatal("rejected request reached Square")
	}
	paid := request("POST", path, string(payload), "application/json", guest, map[string]string{"X-Forwarded-For": "untrusted"})
	if paid.Code != 200 || !strings.Contains(paid.Body.String(), `"state":"paid"`) {
		t.Fatal("owned payment could not complete")
	}
	receipt := request("GET", path, "", "", guest, nil)
	if receipt.Code != 200 || !strings.Contains(receipt.Body.String(), "Test order confirmed") || strings.Contains(receipt.Body.String(), "sandbox.web.squarecdn.com/v1/square.js") {
		t.Fatal("confirmed receipt missing or card form still active")
	}
	var receiptCookie *http.Cookie
	for _, cookie := range receipt.Result().Cookies() {
		if cookie.Name == "selloovy_receipt" {
			receiptCookie = cookie
		}
	}
	if receiptCookie == nil || !receiptCookie.HttpOnly || receiptCookie.Path != path {
		t.Fatal("private receipt recovery cookie missing")
	}
	if out := request("POST", path, string(payload), "application/json", guest, nil); out.Code != 200 || f.calls.Load() != 1 {
		t.Fatal("duplicate payment sent second charge")
	}
	if out := request("POST", path+"/cancel", url.Values{"csrf": {csrf}}.Encode(), "application/x-www-form-urlencoded", guest, nil); out.Code != 409 {
		t.Fatal("paid payment cancelled with preauthorization action")
	}
	fresh := request("POST", path+"/new-basket", url.Values{"csrf": {csrf}}.Encode(), "application/x-www-form-urlencoded", guest, nil)
	if fresh.Code != 303 {
		t.Fatal("paid customer cannot continue shopping")
	}
	var freshToken string
	for _, cookie := range fresh.Result().Cookies() {
		if cookie.Name == "selloovy_basket" {
			freshToken = cookie.Value
		}
	}
	if freshToken == "" || freshToken == guest {
		t.Fatal("completed basket cookie reused")
	}
	// A scoped receipt remains usable after the normal basket cookie changes.
	req := httptest.NewRequest("GET", origin+path, nil)
	req.AddCookie(&http.Cookie{Name: "selloovy_basket", Value: freshToken})
	req.AddCookie(receiptCookie)
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	if out.Code != 200 || !strings.Contains(out.Body.String(), "Test order confirmed") {
		t.Fatal("new basket lost private receipt")
	}
	again := httptest.NewRequest("POST", origin+path+"/new-basket", strings.NewReader(url.Values{"csrf": {csrf}}.Encode()))
	again.AddCookie(&http.Cookie{Name: "selloovy_basket", Value: freshToken})
	again.AddCookie(receiptCookie)
	again.Header.Set("Origin", origin)
	again.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	repeated := httptest.NewRecorder()
	router.ServeHTTP(repeated, again)
	if repeated.Code != 303 || len(repeated.Result().Cookies()) != 0 {
		t.Fatal("old receipt replaced newer basket")
	}
	if _, e = p.GuestView(ctx, key, guest, id); e != nil {
		t.Fatal("receipt backend lost immutable evidence")
	}
}

func TestSandboxReconciliationWorkerLease(t *testing.T) {
	pool, c, key, _, review := paymentFixture(t, "made_to_order")
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
	f := &financialProvider{charge: func(context.Context, square.Request) (square.Result, error) {
		return square.Result{State: square.Pending, ProviderID: "fixture-payment"}, nil
	}, get: func(ctx context.Context, _ string, _ square.Request) (square.Result, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return square.Result{}, ctx.Err()
		}
		return capturedFixture(), nil
	}}
	p := financialService(t, c, f)
	if _, e = p.Submit(ctx, key, guest, r.Revision, prep.ID, "cnon:fixture-worker", "192.0.2.1"); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, "UPDATE payment_attempts SET reconcile_after=clock_timestamp()-interval '1 second'"); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		n, e := p.ReconcileDue(ctx)
		if e == nil && n != 1 {
			e = errors.New("payment not resolved")
		}
		done <- e
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not reconcile")
	}
	if n, e := p.ReconcileDue(ctx); e != nil || n != 0 || f.reads.Load() != 1 {
		t.Fatal("second worker ignored lease")
	}
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if f.calls.Load() != 1 || f.reads.Load() != 1 {
		t.Fatal("worker charged or repeated observation")
	}
}

package square

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureClient(t *testing.T) *Client {
	t.Helper()
	p := filepath.Join(t.TempDir(), "token")
	if os.WriteFile(p, []byte("synthetic-api-fixture"), 0600) != nil {
		t.Fatal("fixture failed")
	}
	c, e := NewSandbox(p, "fixture-location")
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func requestFixture() Request {
	return Request{SourceToken: "cnon:synthetic-fixture", IdempotencyKey: strings.Repeat("a", 32), Reference: strings.Repeat("b", 32), AmountPence: 2498}
}
func observed(status, card string) string {
	return `{"payment":{"id":"fixture-payment","status":"` + status + `","location_id":"fixture-location","reference_id":"` + strings.Repeat("b", 32) + `","source_type":"CARD","amount_money":{"amount":2498,"currency":"GBP"},"card_details":{"status":"` + card + `","card_payment_timeline":{"captured_at":"2026-10-01T10:00:00Z"}}}}`
}
func fakeResponse(body string, code int) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func TestFixedChargeAndStableKey(t *testing.T) {
	c := fixtureClient(t)
	calls := 0
	c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.String() != sandbox+"/v2/payments" || r.Header.Get("Square-Version") != apiVersion || r.Header.Get("Authorization") != "Bearer synthetic-api-fixture" {
			t.Fatal("request escaped fixed sandbox boundary")
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("invalid payload")
		}
		details := body["customer_details"].(map[string]any)
		if body["idempotency_key"] != strings.Repeat("a", 32) || body["reference_id"] != strings.Repeat("b", 32) || body["autocomplete"] != true || details["customer_initiated"] != true || details["seller_keyed_in"] != false {
			t.Fatal("intent/key flags changed")
		}
		if body["amount_money"].(map[string]any)["amount"] != float64(2498) {
			t.Fatal("server amount lost")
		}
		return fakeResponse(observed("COMPLETED", "CAPTURED"), 200), nil
	})
	for range 2 {
		r, e := c.Charge(t.Context(), requestFixture())
		if e != nil || r.State != Paid || r.ProviderID != "fixture-payment" {
			t.Fatal("captured result lost")
		}
	}
	if calls != 2 {
		t.Fatal("unexpected automatic retry")
	}
}
func TestUncertainAndInconsistentResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		http       int
		want       State
	}{
		{"decline without payment", `{"errors":[{"code":"CVV_FAILURE"}]}`, 400, Declined},
		{"used token uncertainty", `{"errors":[{"code":"CARD_TOKEN_USED"}]}`, 400, Unknown},
		{"expired token uncertainty", `{"errors":[{"code":"CARD_TOKEN_EXPIRED"}]}`, 400, Unknown},
		{"idempotency conflict", `{"errors":[{"code":"IDEMPOTENCY_KEY_REUSED"}]}`, 400, Unknown},
		{"mixed conflict", `{"errors":[{"code":"CVV_FAILURE"},{"code":"IDEMPOTENCY_KEY_REUSED"}]}`, 400, Unknown},
		{"decline on server failure", `{"errors":[{"code":"CVV_FAILURE"}]}`, 500, Unknown},
		{"pending", observed("APPROVED", "AUTHORIZED"), 200, Pending},
		{"failed", strings.TrimSuffix(observed("FAILED", "FAILED"), "}") + `,"errors":[{"code":"CARD_DECLINED_VERIFICATION_REQUIRED","detail":"private response"}]}`, 402, Declined},
		{"cancelled", observed("CANCELED", "VOIDED"), 200, Declined},
		{"missing capture time", strings.ReplaceAll(observed("COMPLETED", "CAPTURED"), "2026-10-01T10:00:00Z", ""), 200, Unknown},
		{"malformed capture time", strings.ReplaceAll(observed("COMPLETED", "CAPTURED"), "2026-10-01T10:00:00Z", "bad-time"), 200, Unknown},
		{"not captured", observed("COMPLETED", "AUTHORIZED"), 200, Unknown},
		{"failed yet captured", observed("FAILED", "CAPTURED"), 200, Unknown},
		{"wrong money", strings.ReplaceAll(observed("COMPLETED", "CAPTURED"), "2498", "1"), 200, Unknown},
		{"wrong currency", strings.ReplaceAll(observed("COMPLETED", "CAPTURED"), "GBP", "USD"), 200, Unknown},
		{"wrong location", strings.ReplaceAll(observed("COMPLETED", "CAPTURED"), "fixture-location", "other-location"), 200, Unknown},
		{"wrong reference", strings.ReplaceAll(observed("COMPLETED", "CAPTURED"), strings.Repeat("b", 32), strings.Repeat("c", 32)), 200, Unknown},
		{"cash", strings.ReplaceAll(observed("COMPLETED", "CAPTURED"), `"CARD"`, `"CASH"`), 200, Unknown},
		{"server error", observed("COMPLETED", "CAPTURED"), 500, Unknown},
		{"unauthorized", `{"errors":[{"detail":"private response"}]}`, 401, Unknown},
		{"invalid JSON", "private response", 200, Unknown},
		{"oversized", strings.Repeat("x", 262145), 200, Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixtureClient(t)
			c.http.Transport = transport(func(*http.Request) (*http.Response, error) { return fakeResponse(tc.body, tc.http), nil })
			r, e := c.Charge(t.Context(), requestFixture())
			if e != nil || r.State != tc.want {
				t.Fatal("provider outcome misclassified")
			}
		})
	}
	c := fixtureClient(t)
	calls := 0
	c.http.Transport = transport(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("synthetic-api-fixture private response")
	})
	r, e := c.Charge(t.Context(), requestFixture())
	if e != nil || r.State != Unknown || calls != 1 {
		t.Fatal("transport error/retry boundary failed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r, e = c.Charge(ctx, requestFixture())
	if e != nil || r.State != Unknown {
		t.Fatal("cancelled submission treated as decline")
	}
}
func TestReconcileMatchesKnownIDAndNoRedirects(t *testing.T) {
	c := fixtureClient(t)
	c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.String() != sandbox+"/v2/payments/fixture-payment" || r.ContentLength != 0 {
			t.Fatal("reconciliation mutated provider")
		}
		return fakeResponse(observed("COMPLETED", "CAPTURED"), 200), nil
	})
	r, e := c.Reconcile(t.Context(), "fixture-payment", requestFixture())
	if e != nil || r.State != Paid {
		t.Fatal("reconciliation failed")
	}
	c.http.Transport = transport(func(*http.Request) (*http.Response, error) {
		return fakeResponse(strings.ReplaceAll(observed("COMPLETED", "CAPTURED"), "fixture-payment", "another-payment"), 200), nil
	})
	r, e = c.Reconcile(t.Context(), "fixture-payment", requestFixture())
	if e != nil || r.State != Unknown {
		t.Fatal("foreign payment accepted")
	}
	calls := 0
	c.http.Transport = transport(func(*http.Request) (*http.Response, error) {
		calls++
		r := fakeResponse("", 302)
		r.Header.Set("Location", "https://other.example.com/private")
		return r, nil
	})
	r, e = c.Charge(t.Context(), requestFixture())
	if e != nil || r.State != Unknown || calls != 1 {
		t.Fatal("credential redirect followed")
	}
}
func TestPrivateFormattingAndInvalidSources(t *testing.T) {
	c := fixtureClient(t)
	called := false
	c.http.Transport = transport(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected request")
	})
	for _, source := range []string{"CASH", "EXTERNAL", "ccof:stored-card", "cnon:bad\r\ninjection", "cnon:", ""} {
		req := requestFixture()
		req.SourceToken = source
		if _, e := c.Charge(t.Context(), req); !errors.Is(e, ErrRequest) || called {
			t.Fatal("invalid/nonfresh card source accepted")
		}
	}
	for _, v := range []any{c, requestFixture(), Result{State: Paid, ProviderID: "private-provider-id"}} {
		a, _ := json.Marshal(v)
		s := fmt.Sprintf("%v %#v %+v %s", v, v, v, a)
		for _, private := range []string{"synthetic-api-fixture", "cnon:synthetic-fixture", "private-provider-id"} {
			if strings.Contains(s, private) {
				t.Fatal("implicit formatting exposed private data")
			}
		}
	}
	p := filepath.Join(t.TempDir(), "token")
	os.WriteFile(p, []byte("synthetic-api-fixture"), 0644)
	if _, e := NewSandbox(p, "fixture-location"); !errors.Is(e, ErrConfig) {
		t.Fatal("public credential file accepted")
	}
	os.Chmod(p, 0600)
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(p, link)
	if _, e := NewSandbox(link, "fixture-location"); !errors.Is(e, ErrConfig) {
		t.Fatal("symlink credential accepted")
	}
}

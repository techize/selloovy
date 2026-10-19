package brevo

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

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixture(t *testing.T) *Client {
	t.Helper()
	p := filepath.Join(t.TempDir(), "token")
	if os.WriteFile(p, []byte("fixture-api-token\n"), 0600) != nil {
		t.Fatal("fixture write failed")
	}
	c, e := New(p, "sender@example.com", "Selloovy", Drop)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func request() Request {
	return Request{To: "buyer@example.com", Name: "Buyer", Subject: "Order confirmation", HTML: "<p>Order received.</p>", Text: "Order received.", IdempotencyKey: "00000000-0000-4000-8000-000000000001"}
}
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func TestPayloadAndDrop(t *testing.T) {
	c := fixture(t)
	calls := 0
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.String() != endpoint+"/smtp/email" || r.Header.Get("api-key") != "fixture-api-token" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatal("API boundary incorrect")
		}
		var v struct {
			Headers map[string]string
			To      []struct{ Email string }
			Sender  struct{ Email string }
			HTML    string `json:"htmlContent"`
			Text    string `json:"textContent"`
		}
		if json.NewDecoder(r.Body).Decode(&v) != nil || len(v.To) != 1 || v.To[0].Email != "buyer@example.com" || v.Sender.Email != "sender@example.com" || v.Headers["X-Sib-Sandbox"] != "drop" || v.Headers["IdempotencyKey"] != request().IdempotencyKey || v.HTML == "" || v.Text == "" {
			t.Fatal("payload incorrect")
		}
		return response(201, `{"messageId":"<fixture@example.com>"}`), nil
	})
	v, e := c.Send(t.Context(), request())
	if e != nil || v.State != Validated || v.MessageID == "" || calls != 1 {
		t.Fatal("drop must validate once, never indicate delivery")
	}
}
func TestOutcomeIsConservativeAndNeverRetries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   State
	}{
		{"accepted", 201, `{"messageId":"<fixture@example.com>"}`, Validated},
		{"missing ID", 201, `{}`, Unknown}, {"wrong status", 200, `{"messageId":"<fixture@example.com>"}`, Unknown},
		{"duplicate", 400, `{"code":"duplicate_parameter","message":"private-provider-detail"}`, Unknown},
		{"invalid", 400, `{"code":"invalid_parameter"}`, Rejected}, {"auth", 401, `{"code":"unauthorized"}`, Rejected},
		{"rate limit", 429, `{"code":"too_many_requests"}`, Rejected}, {"timeout", 408, `{"code":"timeout"}`, Unknown},
		{"server", 500, `{"code":"server_error"}`, Unknown}, {"malformed", 201, "not json", Unknown},
		{"trailing", 201, `{"messageId":"x"} {}`, Unknown}, {"oversize", 201, strings.Repeat("x", maxResponse+1), Unknown},
		{"control ID", 201, `{"messageId":"bad\nvalue"}`, Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture(t)
			calls := 0
			c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) { calls++; return response(tc.status, tc.body), nil })
			v, e := c.Send(t.Context(), request())
			if e != nil || v.State != tc.want || calls != 1 {
				t.Fatal("unsafe outcome/retry")
			}
		})
	}
	c := fixture(t)
	calls := 0
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("private transport details")
	})
	v, e := c.Send(context.Background(), request())
	if e != nil || v.State != Unknown || calls != 1 {
		t.Fatal("transport uncertainty must not retry")
	}
}
func TestInputRejectedBeforeNetwork(t *testing.T) {
	edits := []func(*Request){func(r *Request) { r.To = "Buyer <buyer@example.com>" }, func(r *Request) { r.To = "bad\n@example.com" }, func(r *Request) { r.Subject = "injected\r\nheader" }, func(r *Request) { r.Name = "bad\nname" }, func(r *Request) { r.IdempotencyKey = "fresh-non-uuid" }, func(r *Request) { r.HTML = "" }, func(r *Request) { r.Text = "" }, func(r *Request) { r.HTML = string([]byte{255}) }, func(r *Request) { r.Text = strings.Repeat("x", 256*1024) }}
	for _, edit := range edits {
		c := fixture(t)
		c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid request reached network")
			return nil, nil
		})
		r := request()
		edit(&r)
		if _, e := c.Send(t.Context(), r); e != ErrRequest {
			t.Fatal("invalid input accepted")
		}
	}
	c := fixture(t)
	c.mode = Deliver
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("reserved recipient reached delivery network")
		return nil, nil
	})
	for _, s := range []string{"buyer@example.com", "buyer@example.org", "buyer@shop.invalid", "buyer@example.net"} {
		r := request()
		r.To = s
		if _, e := c.Send(t.Context(), r); e != ErrRequest {
			t.Fatal("reserved delivery address accepted")
		}
	}
}
func TestKeyAndConfiguration(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "token")
	for _, tc := range []struct {
		content string
		mode    os.FileMode
	}{{"", 0600}, {"fixture", 0644}, {"bad token", 0600}, {"bad\x00token", 0600}, {strings.Repeat("x", 4097), 0600}} {
		if os.WriteFile(p, []byte(tc.content), 0600) != nil || os.Chmod(p, tc.mode) != nil {
			t.Fatal("fixture write")
		}
		if _, e := New(p, "sender@example.com", "Sender", Drop); e != ErrConfig {
			t.Fatal("invalid key accepted")
		}
	}
	if os.WriteFile(p, []byte("fixture"), 0600) != nil || os.Chmod(p, 0600) != nil {
		t.Fatal("fixture write")
	}
	link := filepath.Join(dir, "link")
	if os.Symlink(p, link) != nil {
		t.Fatal("fixture symlink")
	}
	if _, e := New(link, "sender@example.com", "Sender", Drop); e != ErrConfig {
		t.Fatal("symlink accepted")
	}
	for _, tc := range []struct {
		sender, name string
		mode         Mode
	}{{"bad", "", Drop}, {"sender@example.com", "bad\nname", Drop}, {"sender@example.com", "", Mode("unconfigured")}} {
		if _, e := New(p, tc.sender, tc.name, tc.mode); e != ErrConfig {
			t.Fatal("configuration accepted")
		}
	}
}
func TestSenderCheckIsReadOnlyAndPrivate(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{{`{"senders":[{"email":"SENDER@example.com","active":true}]}`, true}, {`{"senders":[{"email":"sender@example.com","active":false}]}`, false}, {`{"senders":[{"email":"other@example.com","active":true}]}`, false}, {`not json`, false}} {
		c := fixture(t)
		c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != "GET" || r.URL.String() != endpoint+"/senders" || r.Body != nil {
				t.Fatal("sender check changed account")
			}
			return response(200, tc.body), nil
		})
		if (c.VerifySender(t.Context()) == nil) != tc.valid {
			t.Fatal("sender eligibility incorrect")
		}
	}
	c := fixture(t)
	calls := 0
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		r := response(302, "")
		r.Header.Set("Location", "https://example.com/steal")
		return r, nil
	})
	if c.VerifySender(t.Context()) == nil || calls != 1 {
		t.Fatal("redirect followed")
	}
	values := []any{c, request(), Result{Accepted, "<private-message@example.com>"}}
	for _, v := range values {
		b, _ := json.Marshal(v)
		for _, s := range []string{fmt.Sprintf("%v", v), fmt.Sprintf("%#v", v), string(b)} {
			for _, private := range []string{"fixture-api-token", "buyer@example.com", "sender@example.com", "private-message", "Order received"} {
				if strings.Contains(s, private) {
					t.Fatal("implicit representation exposed private values")
				}
			}
		}
	}
}

func TestSenderDiagnosticsNeverExposeProviderDetails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unrecognised IP", 401, `{"message":"Unrecognised IP address 192.0.2.1; private-account-detail"}`, ErrIPBlocked},
		{"unauthorized IP", 403, `{"message":"IP address is not authorized; private-account-detail"}`, ErrIPBlocked},
		{"authentication", 401, `{"message":"Invalid API key private-account-detail"}`, ErrAuthentication},
		{"malformed auth", 401, `not-json private-account-detail`, ErrAuthentication},
		{"inactive", 200, `{"senders":[{"email":"sender@example.com","active":false}]}`, ErrSenderInactive},
		{"missing", 200, `{"senders":[{"email":"other@example.com","active":true}]}`, ErrSenderMissing},
		{"server error cannot claim IP blocking", 500, `{"message":"IP address not authorized private-account-detail"}`, ErrSender},
		{"rate limited", 429, `{"message":"private-account-detail"}`, ErrSender},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture(t)
			calls := 0
			c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) { calls++; return response(tc.status, tc.body), nil })
			e := c.VerifySender(t.Context())
			if e != tc.want || calls != 1 {
				t.Fatal("sender diagnostic incorrect")
			}
			for _, private := range []string{"private-account-detail", "192.0.2.1", "sender@example.com", "other@example.com", "fixture-api-token"} {
				if strings.Contains(e.Error(), private) {
					t.Fatal("provider response leaked")
				}
			}
		})
	}
}

package authhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/techize/selloovy/internal/auth"
)

type fakeBackend struct {
	calls   int
	fail    error
	session bool
}

func (f *fakeBackend) AllowAttempt(ctx context.Context, scope, ip string, n int32) error {
	f.calls++
	if ip != "127.0.0.1" {
		return auth.ErrCredential
	}
	return f.fail
}
func (f *fakeBackend) Login(ctx context.Context, email, password string) (auth.LoginResult, error) {
	f.calls++
	if _, ok := ctx.Deadline(); !ok {
		panic("unbounded request")
	}
	return auth.LoginResult{MFARequired: true}, f.fail
}
func (f *fakeBackend) FinishLogin(context.Context, string, string, bool) (auth.Token, error) {
	f.calls++
	return auth.Token{}, f.fail
}
func (f *fakeBackend) SessionOwner(context.Context, string) (int64, error) {
	f.calls++
	if !f.session {
		return 0, auth.ErrCredential
	}
	return 1, f.fail
}
func (f *fakeBackend) Logout(context.Context, string) error          { f.calls++; return f.fail }
func (f *fakeBackend) CancelChallenge(context.Context, string) error { f.calls++; return f.fail }
func request(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, "https://shop.example.com"+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:12345"
	r.Header.Set("Origin", "https://shop.example.com")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Selloovy-Request", "owner-auth")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	return r
}
func TestHTTPOriginAndRequestBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{"missing origin", func(r *http.Request) { r.Header.Del("Origin") }, 403},
		{"cross origin", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example.com") }, 403},
		{"wrong host", func(r *http.Request) { r.Host = "evil.example.com" }, 403},
		{"missing custom header", func(r *http.Request) { r.Header.Del("X-Selloovy-Request") }, 403},
		{"cross-site metadata", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, 403},
		{"form post", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
	} {
		for _, path := range []string{"/login", "/mfa/start", "/mfa/confirm"} {
			t.Run(tc.name+path, func(t *testing.T) {
				f := &fakeBackend{}
				h, _ := New(f, "https://shop.example.com")
				r := request("POST", path, `{"email":"owner@example.com","password":"synthetic passphrase"}`)
				tc.mutate(r)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != tc.status || f.calls != 0 {
					t.Fatal("unsafe request reached backend")
				}
			})
		}
	}
	for _, body := range []string{`{"email":"owner@example.com","extra":true}`, `{} {}`, strings.Repeat("x", 5000)} {
		f := &fakeBackend{}
		h, _ := New(f, "https://shop.example.com")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, request("POST", "/login", body))
		if w.Code != 400 || f.calls != 1 {
			t.Fatal("malformed body reached password verification")
		}
	}
}
func TestCookiesAndRedactedFailures(t *testing.T) {
	f := &fakeBackend{}
	h, _ := New(f, "https://shop.example.com")
	w := httptest.NewRecorder()
	r := request("POST", "/login", `{"email":"owner@example.com","password":"synthetic passphrase"}`)
	r.Header.Set("X-Forwarded-For", "192.0.2.9")
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("login request failed")
	}
	for _, c := range w.Result().Cookies() {
		if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" || !strings.HasPrefix(c.Name, "__Host-") {
			t.Fatal("insecure authentication cookie")
		}
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{auth.ErrCredential, 401}, {auth.ErrRateLimited, 429}, {auth.ErrStorage, 503}, {errors.New("synthetic private error"), 401}} {
		f.fail = tc.err
		w = httptest.NewRecorder()
		h.ServeHTTP(w, request("POST", "/login", `{}`))
		if w.Code != tc.status || strings.Contains(w.Body.String(), tc.err.Error()) {
			t.Fatal("error policy or redaction failed")
		}
	}
	f.fail = nil
	f.session = true
	r = request("GET", "/status", "")
	r.AddCookie(&http.Cookie{Name: "__Host-selloovy_session", Value: "synthetic"})
	r.AddCookie(&http.Cookie{Name: "__Host-selloovy_session", Value: "duplicate"})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if strings.Contains(w.Body.String(), `"authenticated":true`) {
		t.Fatal("duplicate cookie accepted")
	}
	if _, err := New(f, "http://shop.example.com"); err == nil {
		t.Fatal("nonloopback plain HTTP allowed")
	}
}

func (f *fakeBackend) SessionState(ctx context.Context, token string) (auth.SessionState, error) {
	id, err := f.SessionOwner(ctx, token)
	return auth.SessionState{OwnerID: id, MFAEnabled: true}, err
}
func (f *fakeBackend) PrepareMFA(context.Context, string, string) (auth.MFASecret, error) {
	return auth.MFASecret{}, auth.ErrCredential
}
func (f *fakeBackend) ConfirmSessionEnrollment(context.Context, string, string) ([]auth.RecoveryCode, error) {
	return nil, auth.ErrCredential
}

func TestProtectedContextDoesNotFormatSessionCredentials(t *testing.T) {
	f := &fakeBackend{session: true}
	h, _ := New(f, "https://shop.example.com")
	token := "synthetic-session-value"
	protected := h.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if SessionToken(r.Context()) != token {
			t.Fatal("protected context lost session")
		}
		if strings.Contains(fmt.Sprint(r.Context()), token) {
			t.Fatal("context formatting disclosed token")
		}
		w.WriteHeader(204)
	}))
	r := request("GET", "/shop", "")
	r.AddCookie(&http.Cookie{Name: "__Host-selloovy_session", Value: token})
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal("protected handler did not run")
	}
}

func TestMultipartIsConfinedToOwnedPhotoPut(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, origin, header string
		cookie                             bool
		want                               int
	}{
		{"photo upload", "PUT", "/api/admin/products/1/photo", "https://shop.example.com", "owner-auth", true, 204},
		{"anonymous", "PUT", "/api/admin/products/1/photo", "https://shop.example.com", "owner-auth", false, 401},
		{"cross origin", "PUT", "/api/admin/products/1/photo", "https://other.example.com", "owner-auth", true, 403},
		{"no header", "PUT", "/api/admin/products/1/photo", "https://shop.example.com", "", true, 403},
		{"wrong method", "POST", "/api/admin/products/1/photo", "https://shop.example.com", "owner-auth", true, 415},
		{"other product endpoint", "PUT", "/api/admin/products/1/maker", "https://shop.example.com", "owner-auth", true, 415},
		{"authentication", "POST", "/api/auth/login", "https://shop.example.com", "owner-auth", true, 415},
		{"suffix injection", "PUT", "/api/admin/products/1/photo/extra", "https://shop.example.com", "owner-auth", true, 415},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeBackend{session: true}
			h, _ := New(f, "https://shop.example.com")
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if SessionToken(r.Context()) == "" {
					t.Fatal("missing session boundary")
				}
				w.WriteHeader(204)
			})
			req := request(tc.method, tc.path, "")
			req.Header.Set("Content-Type", "multipart/form-data; boundary=fixture")
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("X-Selloovy-Request", tc.header)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: "__Host-selloovy_session", Value: "synthetic-session"})
			}
			w := httptest.NewRecorder()
			h.ProtectAdmin(next).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
		})
	}
}

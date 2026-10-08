package authhttp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/database"
)

func browserDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("SELLOOVY_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set SELLOOVY_TEST_DATABASE_URL for browser authentication integration")
	}
	u, err := url.Parse(base)
	if err != nil || u.Path != "/selloovy_test" || u.Query().Has("dbname") {
		t.Fatal("dedicated test database required")
	}
	admin, err := pgx.Connect(t.Context(), base)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	var suffix [8]byte
	_, _ = rand.Read(suffix[:])
	name := "selloovy_test_browser_" + hex.EncodeToString(suffix[:])
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(t.Context(), "CREATE DATABASE "+quoted); err != nil {
		t.Fatal("test database creation failed")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Error("test database cleanup failed")
		}
		_ = admin.Close(ctx)
	})
	u.Path = "/" + name
	if err = database.Migrate(t.Context(), u.String()); err != nil {
		t.Fatal(err)
	}
	pool, err := database.Open(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
func fixtureCode(secret auth.MFASecret, when time.Time) string {
	return keyCode(secret.EnrollmentKey(), when)
}
func keyCode(encoded string, when time.Time) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(encoded)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(when.Unix()/30))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[offset:offset+4])&0x7fffffff)%1000000)
}
func TestBrowserOwnerAuthentication(t *testing.T) {
	t.Run("eight-character-minimum", func(t *testing.T) { testBrowserOwnerAuthentication(t, "fixture8") })
	t.Run("long-unicode", func(t *testing.T) { testBrowserOwnerAuthentication(t, strings.Repeat("🦕", 100)) })
}
func testBrowserOwnerAuthentication(t *testing.T, password string) {
	pool := browserDatabase(t)
	ctx := t.Context()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	vault, _ := auth.NewMFAVault(key)
	store, err := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if err != nil {
		t.Fatal(err)
	}
	id, secret, err := store.CreateFirstOwner(ctx, "Synthetic Maker", "atelier-é@example.com", password)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := store.ConfirmEnrollment(ctx, id, fixtureCode(secret, time.Now().Add(-30*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(store, "https://shop.example.com")
	if err != nil {
		t.Fatal(err)
	}
	send := func(method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		r := request(method, path, string(payload))
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	cookie := func(w *httptest.ResponseRecorder, name string) *http.Cookie {
		t.Helper()
		for _, c := range w.Result().Cookies() {
			if c.Name == "__Host-selloovy_"+name && c.MaxAge > 0 {
				return c
			}
		}
		t.Fatal("authentication cookie missing")
		return nil
	}
	login := func() *http.Cookie {
		t.Helper()
		w := send("POST", "/login", map[string]string{"email": "atelier-é@example.com", "password": password})
		if w.Code != 200 {
			t.Fatal("password login failed")
		}
		return cookie(w, "challenge")
	}
	if w := send("GET", "/workspace", nil); w.Code != 401 {
		t.Fatal("anonymous protected access allowed")
	}
	challenge := login()
	if w := send("GET", "/workspace", nil, challenge); w.Code != 401 {
		t.Fatal("password-only access allowed")
	}
	if w := send("POST", "/verify", map[string]any{"code": "0000000"}, challenge); w.Code != 401 {
		t.Fatal("invalid factor accepted")
	}
	w := send("POST", "/verify", map[string]any{"code": codes[0].Reveal(), "recovery": true}, challenge)
	if w.Code != 200 {
		t.Fatal("recovery login failed")
	}
	session := cookie(w, "session")
	if w := send("GET", "/workspace", nil, session); w.Code != 200 {
		t.Fatal("owner access refused")
	}
	if w := send("POST", "/verify", map[string]any{"code": codes[1].Reveal(), "recovery": true}, challenge); w.Code != 401 {
		t.Fatal("challenge replay allowed")
	}
	if w := send("POST", "/logout", map[string]any{}, session); w.Code != 200 {
		t.Fatal("logout failed")
	}
	if w := send("GET", "/workspace", nil, session); w.Code != 401 {
		t.Fatal("logged-out session accepted")
	}
	challenge = login()
	if w := send("POST", "/verify", map[string]any{"code": codes[0].Reveal(), "recovery": true}, challenge); w.Code != 401 {
		t.Fatal("recovery code reused")
	}
	w = send("POST", "/verify", map[string]any{"code": fixtureCode(secret, time.Now())}, challenge)
	if w.Code != 200 {
		t.Fatal("authenticator login failed")
	}
	session = cookie(w, "session")
	// A new sign-in rotates away the existing session before granting a new one.
	w = send("POST", "/login", map[string]string{"email": "atelier-é@example.com", "password": password}, session)
	if w.Code != 200 {
		t.Fatal("rotation failed")
	}
	challenge = cookie(w, "challenge")
	if w := send("GET", "/workspace", nil, session); w.Code != 401 {
		t.Fatal("old session survived rotation")
	}
	if w := send("POST", "/logout", map[string]any{}, challenge); w.Code != 200 {
		t.Fatal("pending logout failed")
	}
	if w := send("POST", "/verify", map[string]any{"code": codes[1].Reveal(), "recovery": true}, challenge); w.Code != 401 {
		t.Fatal("cancelled challenge accepted")
	}
	pool.Close()
	if w := send("GET", "/workspace", nil, session); w.Code != 503 {
		t.Fatal("database outage did not fail closed")
	}
}

func TestOptionalMFAAndAuthenticatedBrowserEnrollment(t *testing.T) {
	pool := browserDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	store, err := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := store.CreateFirstOwner(ctx, "Optional MFA fixture", "owner@example.com", "fixture8")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := New(store, "https://shop.example.com")
	send := func(method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		r := request(method, path, string(payload))
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	cookie := func(w *httptest.ResponseRecorder, name string) *http.Cookie {
		t.Helper()
		for _, c := range w.Result().Cookies() {
			if c.Name == "__Host-selloovy_"+name && c.MaxAge > 0 {
				return c
			}
		}
		t.Fatal("expected cookie missing")
		return nil
	}
	login := func() *httptest.ResponseRecorder {
		return send("POST", "/login", map[string]string{"email": "owner@example.com", "password": "fixture8"})
	}
	first := login()
	if first.Code != 200 || !strings.Contains(first.Body.String(), `"authenticated":true`) {
		t.Fatal("optional owner could not use password login")
	}
	a := cookie(first, "session")
	b := cookie(login(), "session")
	if w := send("GET", "/status", nil, a); w.Code != 200 || !strings.Contains(w.Body.String(), `"mfaEnabled":false`) {
		t.Fatal("disabled MFA status not reported")
	}
	if w := send("POST", "/mfa/start", map[string]string{"password": "fixture8"}); w.Code != 401 {
		t.Fatal("anonymous enrollment allowed")
	}
	if w := send("POST", "/mfa/start", map[string]string{"password": "wrong fixture passphrase"}, a); w.Code != 401 {
		t.Fatal("enrollment did not reverify password")
	}
	start := func() string {
		t.Helper()
		w := send("POST", "/mfa/start", map[string]string{"password": "fixture8"}, a)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private enrollment failed")
		}
		var data struct {
			Key string `json:"key"`
			QR  string `json:"qr"`
		}
		if json.Unmarshal(w.Body.Bytes(), &data) != nil {
			t.Fatal("enrollment response invalid")
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data.QR, "data:image/png;base64,"))
		if err != nil {
			t.Fatal("QR encoding invalid")
		}
		image, err := png.Decode(bytes.NewReader(raw))
		if err != nil || image.Bounds().Dx() != 320 {
			t.Fatal("QR image invalid")
		}
		return data.Key
	}
	key := start()
	if w := send("POST", "/mfa/confirm", map[string]string{"code": keyCode(key, time.Now())}, b); w.Code != 401 {
		t.Fatal("different session confirmed enrollment")
	}
	if _, err = pool.Exec(ctx, "UPDATE public.owners SET mfa_enrollment_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal("expiry fixture failed")
	}
	if w := send("POST", "/mfa/confirm", map[string]string{"code": keyCode(key, time.Now())}, a); w.Code != 401 {
		t.Fatal("expired enrollment accepted")
	}
	key = start()
	if w := send("POST", "/mfa/confirm", map[string]string{"code": "0000000"}, a); w.Code != 401 {
		t.Fatal("invalid enrollment factor accepted")
	}
	if w := send("GET", "/workspace", nil, a); w.Code != 200 {
		t.Fatal("unconfirmed enrollment changed access")
	}
	confirmed := send("POST", "/mfa/confirm", map[string]string{"code": keyCode(key, time.Now())}, a)
	if confirmed.Code != 200 {
		t.Fatal("confirmed enrollment failed")
	}
	var result struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	_ = json.Unmarshal(confirmed.Body.Bytes(), &result)
	if len(result.RecoveryCodes) != 10 {
		t.Fatal("recovery codes not delivered")
	}
	for _, c := range []*http.Cookie{a, b} {
		if w := send("GET", "/workspace", nil, c); w.Code != 401 {
			t.Fatal("password-only session survived MFA activation")
		}
	}
	next := login()
	if next.Code != 200 || !strings.Contains(next.Body.String(), `"step":"mfa"`) {
		t.Fatal("enabled MFA was bypassed")
	}
	challenge := cookie(next, "challenge")
	if w := send("GET", "/workspace", nil, challenge); w.Code != 401 {
		t.Fatal("MFA challenge granted owner access")
	}
	w := send("POST", "/verify", map[string]any{"code": result.RecoveryCodes[0], "recovery": true}, challenge)
	if w.Code != 200 {
		t.Fatal("enabled owner recovery login failed")
	}
	session := cookie(w, "session")
	if w := send("GET", "/status", nil, session); !strings.Contains(w.Body.String(), `"mfaEnabled":true`) {
		t.Fatal("enabled MFA status not reported")
	}
	if w := send("POST", "/mfa/start", map[string]string{"password": "fixture8"}, session); w.Code != 401 {
		t.Fatal("enabled factor could be replaced without lifecycle flow")
	}
}

func TestConcurrentMFAActivationCommitsOnce(t *testing.T) {
	pool := browserDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	store, err := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.CreateFirstOwner(ctx, "Concurrent fixture", "owner@example.com", "fixture8")
	if err != nil {
		t.Fatal(err)
	}
	login, err := store.Login(ctx, "owner@example.com", "fixture8")
	if err != nil || login.MFARequired {
		t.Fatal("password-only fixture failed")
	}
	secret, err := store.PrepareMFA(ctx, login.Token.Reveal(), "fixture8")
	if err != nil {
		t.Fatal(err)
	}
	code := fixtureCode(secret, time.Now())
	results := make(chan error, 4)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			codes, err := store.ConfirmSessionEnrollment(ctx, login.Token.Reveal(), code)
			if err == nil && len(codes) != 10 {
				err = errors.New("missing recovery codes")
			}
			results <- err
		})
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, auth.ErrCredential) {
			t.Fatal("unexpected concurrent activation failure")
		}
	}
	if succeeded != 1 {
		t.Fatal("MFA activation did not have exactly one winner")
	}
	if _, err = store.SessionOwner(ctx, login.Token.Reveal()); !errors.Is(err, auth.ErrCredential) {
		t.Fatal("old session survived activation")
	}
	var recoveryCount int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM public.owner_recovery_codes").Scan(&recoveryCount); err != nil || recoveryCount != 10 {
		t.Fatal("activation recovery codes were not atomic")
	}
}

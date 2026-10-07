package authhttp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
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
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret.EnrollmentKey())
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

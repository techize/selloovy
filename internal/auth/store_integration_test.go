package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techize/selloovy/db/migrations"
	"github.com/techize/selloovy/internal/database"
)

func authTestDatabase(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	base := os.Getenv("SELLOOVY_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("set SELLOOVY_TEST_DATABASE_URL for isolated authentication integration tests")
	}
	u, err := url.Parse(base)
	if err != nil || u.Path != "/selloovy_test" || u.Query().Has("dbname") {
		t.Fatal("dedicated selloovy_test database required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal("could not connect to test database")
	}
	var suffix [8]byte
	_, _ = rand.Read(suffix[:])
	name := "selloovy_test_auth_" + hex.EncodeToString(suffix[:])
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		_ = admin.Close(ctx)
		t.Fatal("could not create isolated database")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Error("could not remove isolated database")
		}
		_ = admin.Close(ctx)
	})
	u.Path = "/" + name
	connection := u.String()
	conn, err := pgx.Connect(ctx, connection)
	if err != nil {
		t.Fatal("upgrade fixture connection failed")
	}
	schema, err := migrations.Files.ReadFile("001_shops.sql")
	if err != nil {
		t.Fatal("version one schema fixture missing")
	}
	if _, err = conn.Exec(ctx, string(schema)); err != nil {
		t.Fatal("version one schema fixture failed")
	}
	if _, err = conn.Exec(ctx, "CREATE TABLE public.selloovy_schema_version (version integer NOT NULL); INSERT INTO public.selloovy_schema_version VALUES (1); INSERT INTO public.shops (name) VALUES ('Upgrade Fixture')"); err != nil {
		t.Fatal("upgrade fixture failed")
	}
	_ = conn.Close(ctx)
	if err = database.Migrate(ctx, connection); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, connection); err != nil {
		t.Fatal("repeat upgrade failed")
	}
	pool, err := database.Open(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var retained int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM public.shops WHERE name='Upgrade Fixture'").Scan(&retained); err != nil || retained != 1 {
		t.Fatal("upgrade did not preserve existing shop")
	}
	if err = database.Ready(ctx, pool); err != nil {
		t.Fatal("upgraded schema not ready")
	}
	return pool, connection
}

func TestPersistentOwnerAuthentication(t *testing.T) {
	pool, connection := authTestDatabase(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	vault, _ := NewMFAVault(key)
	store, err := NewStore(ctx, pool, vault, NewPasswordHasher())
	if err != nil {
		t.Fatal(err)
	}
	secondPool, err := database.Open(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	defer secondPool.Close()
	replica, err := NewStore(ctx, secondPool, vault, NewPasswordHasher())
	if err != nil {
		t.Fatal(err)
	}
	var shop int64
	if err = pool.QueryRow(ctx, "INSERT INTO public.shops (name) VALUES ('Example Maker') RETURNING id").Scan(&shop); err != nil {
		t.Fatal("shop fixture failed")
	}
	const email = "owner@example.com"
	const password = "a synthetic owner passphrase"
	id, secret, err := store.CreateOwner(ctx, shop, email, password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.StartLogin(ctx, email, password); !errors.Is(err, ErrCredential) {
		t.Fatal("pending MFA owner was allowed to log in")
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM public.owner_sessions").Scan(&count); err != nil || count != 0 {
		t.Fatal("password alone issued a session")
	}
	var now time.Time
	if err = pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		t.Fatal("clock fixture failed")
	}
	codes, err := store.ConfirmEnrollment(ctx, id, secret.code(now.Unix()/30))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ConfirmEnrollment(ctx, id, secret.code(now.Unix()/30)); !errors.Is(err, ErrCredential) {
		t.Fatal("MFA enrollment could be replaced")
	}
	for _, credentials := range [][2]string{{email, "a wrong synthetic passphrase"}, {"missing@example.com", password}} {
		if _, err = store.StartLogin(ctx, credentials[0], credentials[1]); !errors.Is(err, ErrCredential) {
			t.Fatal("wrong/unknown credential accepted")
		}
	}
	challenge := func() Token {
		t.Helper()
		token, err := store.StartLogin(ctx, email, password)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	query := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal("fixture update failed")
		}
	}
	rejected := func(token Token, code string, recovery bool) {
		t.Helper()
		if _, err := store.FinishLogin(ctx, token.Reveal(), code, recovery); !errors.Is(err, ErrCredential) {
			t.Fatal("invalid challenge/factor accepted")
		}
	}
	// Independent stores/pools represent two app replicas using separate challenges.
	raceFactor := func(factor string, recovery bool) Token {
		t.Helper()
		a, b := challenge(), challenge()
		start := make(chan struct{})
		results := make(chan struct {
			token Token
			err   error
		}, 2)
		var wg sync.WaitGroup
		for i, s := range []*Store{store, replica} {
			token := []Token{a, b}[i]
			wg.Go(func() {
				<-start
				session, err := s.FinishLogin(ctx, token.Reveal(), factor, recovery)
				results <- struct {
					token Token
					err   error
				}{session, err}
			})
		}
		close(start)
		wg.Wait()
		close(results)
		successes := 0
		var winner Token
		for result := range results {
			if result.err == nil {
				successes++
				winner = result.token
			} else if !errors.Is(result.err, ErrCredential) {
				t.Fatal(result.err)
			}
		}
		if successes != 1 {
			t.Fatal("concurrent factor use did not yield exactly one session")
		}
		return winner
	}
	recoverySession := raceFactor(codes[0].Reveal(), true)
	if owner, err := store.SessionOwner(ctx, recoverySession.Reveal()); err != nil || owner != id {
		t.Fatal("recovery session invalid")
	}
	query("UPDATE public.owners SET last_totp_counter=-1 WHERE id=$1", id)
	if err = pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		t.Fatal("clock fixture failed")
	}
	totpSession := raceFactor(secret.code(now.Unix()/30), false)
	rejected(challenge(), secret.code(now.Unix()/30), false)
	rejected(challenge(), codes[0].Reveal(), true)
	// Failed session insert must restore BOTH factor and challenge for retry.
	for i, recovery := range []bool{true, false} {
		token := challenge()
		factor := codes[i+1].Reveal()
		if !recovery {
			query("UPDATE public.owners SET last_totp_counter=-1 WHERE id=$1", id)
			if err = pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
				t.Fatal("clock fixture failed")
			}
			factor = secret.code(now.Unix() / 30)
		}
		query("ALTER TABLE public.owner_sessions ADD CONSTRAINT test_reject_session CHECK (false) NOT VALID")
		if _, err = store.FinishLogin(ctx, token.Reveal(), factor, recovery); !errors.Is(err, ErrStorage) {
			t.Fatal("session failure was not redacted")
		}
		query("ALTER TABLE public.owner_sessions DROP CONSTRAINT test_reject_session")
		if _, err = store.FinishLogin(ctx, token.Reveal(), factor, recovery); err != nil {
			t.Fatal("factor/challenge consumed despite transaction rollback")
		}
		rejected(token, factor, recovery)
	}
	// Five failures persist across stores and exhaust the challenge only.
	limited := challenge()
	for i := range 5 {
		actor := []*Store{store, replica}[i%2]
		if _, err := actor.FinishLogin(ctx, limited.Reveal(), "invalid", true); !errors.Is(err, ErrCredential) {
			t.Fatal("invalid factor accepted")
		}
	}
	rejected(limited, codes[3].Reveal(), true)
	expired := challenge()
	expiredDigest, _ := tokenDigest(expired.Reveal(), "challenge")
	query("UPDATE public.owner_login_challenges SET expires_at=clock_timestamp()-interval '1 second' WHERE digest=$1", expiredDigest)
	rejected(expired, codes[3].Reveal(), true)
	if _, err = store.SessionOwner(ctx, expired.Reveal()); !errors.Is(err, ErrCredential) {
		t.Fatal("challenge was accepted as a session")
	}
	if _, err = store.FinishLogin(ctx, recoverySession.Reveal(), codes[3].Reveal(), true); !errors.Is(err, ErrCredential) {
		t.Fatal("session was accepted as a challenge")
	}
	// Restart application state/pool, preserving sessions and factor consumption.
	store.pool.Close()
	store.pool, err = database.Open(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	defer store.pool.Close()
	if owner, err := store.SessionOwner(ctx, recoverySession.Reveal()); err != nil || owner != id {
		t.Fatal("session did not survive pool restart")
	}
	if err = store.Logout(ctx, recoverySession.Reveal()); err != nil {
		t.Fatal(err)
	}
	if _, err = replica.SessionOwner(ctx, recoverySession.Reveal()); !errors.Is(err, ErrCredential) {
		t.Fatal("logout was not shared")
	}
	sessionDigest, _ := tokenDigest(totpSession.Reveal(), "session")
	query("UPDATE public.owner_sessions SET last_seen_at=clock_timestamp()-interval '31 minutes' WHERE digest=$1", sessionDigest)
	if _, err = store.SessionOwner(ctx, totpSession.Reveal()); !errors.Is(err, ErrCredential) {
		t.Fatal("idle session accepted")
	}
	session, err := store.FinishLogin(ctx, challenge().Reveal(), codes[3].Reveal(), true)
	if err != nil {
		t.Fatal("failed/expired challenges consumed an unused recovery code")
	}
	sessionDigest, _ = tokenDigest(session.Reveal(), "session")
	query("UPDATE public.owner_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE digest=$1", sessionDigest)
	if _, err = store.SessionOwner(ctx, session.Reveal()); !errors.Is(err, ErrCredential) {
		t.Fatal("expired session accepted")
	}
	freshSession, err := store.FinishLogin(ctx, challenge().Reveal(), codes[4].Reveal(), true)
	if err != nil {
		t.Fatal(err)
	}
	stale := challenge()
	query("UPDATE public.owners SET auth_version=auth_version+1 WHERE id=$1", id)
	rejected(stale, codes[5].Reveal(), true)
	if _, err = store.SessionOwner(ctx, freshSession.Reveal()); !errors.Is(err, ErrCredential) {
		t.Fatal("stale credential-version session accepted")
	}
	// Ciphertext and bearer-token storage contain no plaintext credentials.
	var cipher []byte
	var storedHash string
	var enabled bool
	if err = store.pool.QueryRow(ctx, "SELECT mfa_ciphertext,password_hash,mfa_enabled FROM public.owners WHERE id=$1", id).Scan(&cipher, &storedHash, &enabled); err != nil {
		t.Fatal("storage fixture failed")
	}
	if !enabled || strings.Contains(string(cipher), secret.EnrollmentKey()) || strings.Contains(storedHash, password) {
		t.Fatal("factor storage/MFA state invalid")
	}
	secondPool.Close()
	query("DROP TABLE public.owner_sessions")
	if _, err = store.SessionOwner(ctx, session.Reveal()); !errors.Is(err, ErrStorage) {
		t.Fatal("storage outage was not fail-closed/redacted")
	}
}

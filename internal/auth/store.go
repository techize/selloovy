package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techize/selloovy/internal/authdb"
)

var ErrStorage = errors.New("authentication storage is unavailable")

// Token is a 256-bit opaque bearer value. Only its purpose-bound digest is stored.
type Token struct{ value string }

func (Token) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "[redacted authentication token]") }
func (t Token) Reveal() string           { return t.value }
func newToken() Token {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return Token{base64.RawURLEncoding.EncodeToString(b[:])}
}
func tokenDigest(value, purpose string) ([]byte, error) {
	if len(value) != 43 {
		return nil, ErrCredential
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(b) != 32 {
		return nil, ErrCredential
	}
	sum := sha256.Sum256(append([]byte("selloovy/"+purpose+"/v1/"), b...))
	return sum[:], nil
}

// Store has no HTTP surface. Callers must add origin/CSRF, cookie and shared
// account/IP abuse controls before exposing it. Share the hasher per process.
type Store struct {
	pool   *pgxpool.Pool
	vault  *MFAVault
	hasher *PasswordHasher
	dummy  string
}

func NewStore(ctx context.Context, pool *pgxpool.Pool, vault *MFAVault, hasher *PasswordHasher) (*Store, error) {
	if pool == nil || vault == nil || vault.aead == nil || hasher == nil {
		return nil, ErrStorage
	}
	dummy, err := hasher.Hash(ctx, newToken().Reveal())
	if err != nil {
		return nil, err
	}
	return &Store{pool, vault, hasher, dummy}, nil
}
func normalizedEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		return "", ErrCredential
	}
	return email, nil
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// CreateOwner is an operator-controlled enrollment primitive, not registration.
func (s *Store) CreateOwner(ctx context.Context, shopID int64, email, password string) (int64, MFASecret, error) {
	email, err := normalizedEmail(email)
	if err != nil || shopID < 1 {
		return 0, MFASecret{}, ErrCredential
	}
	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return 0, MFASecret{}, err
	}
	secret, err := NewMFASecret()
	if err != nil {
		return 0, MFASecret{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	defer rollback(tx)
	id, err := authdb.New(tx).CreateOwner(ctx, authdb.CreateOwnerParams{ShopID: shopID, Email: email, PasswordHash: hash})
	if err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	encrypted, err := s.vault.Seal(id, secret)
	if err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	if err = authdb.New(tx).SaveMFASeed(ctx, authdb.SaveMFASeedParams{ID: id, MfaCiphertext: encrypted}); err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	return id, secret, nil
}

func (s *Store) ConfirmEnrollment(ctx context.Context, ownerID int64, code string) ([]RecoveryCode, error) {
	return s.confirmEnrollment(ctx, ownerID, code, "")
}
func (s *Store) confirmEnrollment(ctx context.Context, ownerID int64, code, session string) ([]RecoveryCode, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, ErrStorage
	}
	defer rollback(tx)
	enrollment, err := authdb.New(tx).LockEnrollment(ctx, ownerID)
	encrypted, enabled := enrollment.MfaCiphertext, enrollment.MfaEnabled
	if errors.Is(err, pgx.ErrNoRows) || enabled {
		return nil, ErrCredential
	}
	if err != nil {
		return nil, ErrStorage
	}
	now, err := authdb.New(tx).DatabaseTime(ctx)
	if err != nil {
		return nil, ErrStorage
	}
	if session != "" {
		digest, e := tokenDigest(session, "session")
		if e != nil || subtle.ConstantTimeCompare(digest, enrollment.MfaEnrollmentSessionDigest) != 1 || !enrollment.MfaEnrollmentExpiresAt.Valid || !now.Time.Before(enrollment.MfaEnrollmentExpiresAt.Time) {
			return nil, ErrCredential
		}
		if _, e = authdb.New(tx).TouchSession(ctx, digest); errors.Is(e, pgx.ErrNoRows) {
			return nil, ErrCredential
		} else if e != nil {
			return nil, ErrStorage
		}
	}
	secret, err := s.vault.Open(ownerID, encrypted)
	if err != nil {
		return nil, ErrStorage
	}
	counter, err := secret.MatchTOTP(code, now.Time, -1)
	if err != nil {
		return nil, ErrCredential
	}
	codes, digests, err := NewRecoveryCodes(ownerID)
	if err != nil {
		return nil, err
	}
	for _, digest := range digests {
		if err = authdb.New(tx).SaveRecoveryCode(ctx, authdb.SaveRecoveryCodeParams{OwnerID: ownerID, Digest: digest[:]}); err != nil {
			return nil, ErrStorage
		}
	}
	if err = authdb.New(tx).EnableMFA(ctx, authdb.EnableMFAParams{ID: ownerID, LastTotpCounter: counter}); err != nil {
		return nil, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, ErrStorage
	}
	return codes, nil
}

// LoginResult distinguishes a password-only session from an MFA challenge.
type LoginResult struct {
	Token       Token
	MFARequired bool
}

// StartLogin is the challenge-only primitive for MFA-enabled owners.
func (s *Store) StartLogin(ctx context.Context, email, password string) (Token, error) {
	result, err := s.beginLogin(ctx, email, password, false)
	return result.Token, err
}

// Login returns a session when MFA is off, or a challenge when it is enabled.
func (s *Store) Login(ctx context.Context, email, password string) (LoginResult, error) {
	return s.beginLogin(ctx, email, password, true)
}
func (s *Store) beginLogin(ctx context.Context, email, password string, optional bool) (LoginResult, error) {
	email, emailErr := normalizedEmail(email)
	if err := s.AllowAttempt(ctx, "password", email, 10); err != nil {
		return LoginResult{}, err
	}
	owner, err := authdb.New(s.pool).OwnerByEmail(ctx, email)
	id, hash, enabled, version := owner.ID, owner.PasswordHash, owner.MfaEnabled, owner.AuthVersion
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return LoginResult{}, ErrStorage
	}
	found := err == nil && emailErr == nil
	if !found {
		hash = s.dummy
	}
	if err = s.hasher.Verify(ctx, password, hash); err != nil {
		return LoginResult{}, err
	}
	if !found || (!enabled && !optional) {
		return LoginResult{}, ErrCredential
	}
	token := newToken()
	// Recheck the verified credential/version under lock so a concurrent reset
	// cannot create a challenge based on a stale password.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return LoginResult{}, ErrStorage
	}
	defer rollback(tx)
	current, err := authdb.New(tx).LockPassword(ctx, id)
	currentHash, currentVersion, enabled := current.PasswordHash, current.AuthVersion, current.MfaEnabled
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginResult{}, ErrCredential
	}
	if err != nil {
		return LoginResult{}, ErrStorage
	}
	if (!enabled && !optional) || hash != currentHash || version != currentVersion {
		return LoginResult{}, ErrCredential
	}
	if enabled {
		digest, _ := tokenDigest(token.value, "challenge")
		err = authdb.New(tx).CreateChallenge(ctx, authdb.CreateChallengeParams{Digest: digest, OwnerID: id, AuthVersion: version})
	} else {
		digest, _ := tokenDigest(token.value, "session")
		now, e := authdb.New(tx).DatabaseTime(ctx)
		if e != nil {
			return LoginResult{}, ErrStorage
		}
		err = authdb.New(tx).CreateSession(ctx, authdb.CreateSessionParams{Digest: digest, OwnerID: id, AuthVersion: version, IssuedAt: now})
	}
	if err != nil {
		return LoginResult{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		return LoginResult{}, ErrStorage
	}
	return LoginResult{Token: token, MFARequired: enabled}, nil
}

// FinishLogin serializes all factor consumption through the owner's row lock.
// Challenge, factor and session are committed together, or all roll back.
func (s *Store) FinishLogin(ctx context.Context, challenge, factor string, recovery bool) (Token, error) {
	digest, err := tokenDigest(challenge, "challenge")
	if err != nil {
		return Token{}, err
	}
	id, err := authdb.New(s.pool).ChallengeOwner(ctx, digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrCredential
	}
	if err != nil {
		return Token{}, ErrStorage
	}
	if err = s.AllowAttempt(ctx, "mfa", strconv.FormatInt(id, 10), 10); err != nil {
		return Token{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Token{}, ErrStorage
	}
	defer rollback(tx)
	owner, err := authdb.New(tx).LockFactor(ctx, id)
	encrypted, enabled, last, version := owner.MfaCiphertext, owner.MfaEnabled, owner.LastTotpCounter, owner.AuthVersion
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrCredential
	}
	if err != nil {
		return Token{}, ErrStorage
	}
	status, err := authdb.New(tx).LockChallenge(ctx, authdb.LockChallengeParams{Digest: digest, AuthVersion: version})
	usable := status.Valid && status.Bool
	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, ErrCredential
	}
	if err != nil {
		return Token{}, ErrStorage
	}
	if !enabled || !usable {
		return Token{}, ErrCredential
	}
	now, err := authdb.New(tx).DatabaseTime(ctx)
	if err != nil {
		return Token{}, ErrStorage
	}
	valid := false
	if recovery {
		codeDigest, digestErr := RecoveryDigest(id, factor)
		if digestErr == nil {
			result, deleteErr := authdb.New(tx).ConsumeRecoveryCode(ctx, authdb.ConsumeRecoveryCodeParams{OwnerID: id, Digest: codeDigest[:]})
			if deleteErr != nil {
				return Token{}, ErrStorage
			}
			valid = result == 1
		}
	} else {
		secret, openErr := s.vault.Open(id, encrypted)
		if openErr != nil {
			return Token{}, ErrStorage
		}
		counter, matchErr := secret.MatchTOTP(factor, now.Time, last)
		if matchErr == nil {
			if err = authdb.New(tx).ConsumeTOTP(ctx, authdb.ConsumeTOTPParams{ID: id, LastTotpCounter: counter}); err != nil {
				return Token{}, ErrStorage
			}
			valid = true
		}
	}
	if !valid {
		if err = authdb.New(tx).FailChallenge(ctx, digest); err != nil {
			return Token{}, ErrStorage
		}
		if err = tx.Commit(ctx); err != nil {
			return Token{}, ErrStorage
		}
		return Token{}, ErrCredential
	}
	token := newToken()
	sessionDigest, _ := tokenDigest(token.value, "session")
	if err = authdb.New(tx).CreateSession(ctx, authdb.CreateSessionParams{Digest: sessionDigest, OwnerID: id, AuthVersion: version, IssuedAt: now}); err != nil {
		return Token{}, ErrStorage
	}
	if err = authdb.New(tx).ConsumeChallenge(ctx, authdb.ConsumeChallengeParams{Digest: digest, ConsumedAt: now}); err != nil {
		return Token{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		return Token{}, ErrStorage
	}
	return token, nil
}

// SessionOwner touches the idle clock only for a current, valid owner session.
func (s *Store) SessionOwner(ctx context.Context, token string) (int64, error) {
	digest, err := tokenDigest(token, "session")
	if err != nil {
		return 0, err
	}
	var id int64
	row, queryErr := authdb.New(s.pool).TouchSession(ctx, digest)
	id, err = row.OwnerID, queryErr
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrCredential
	}
	if err != nil {
		return 0, ErrStorage
	}
	return id, nil
}
func (s *Store) Logout(ctx context.Context, token string) error {
	digest, err := tokenDigest(token, "session")
	if err != nil {
		return err
	}
	if err = authdb.New(s.pool).DeleteSession(ctx, digest); err != nil {
		return ErrStorage
	}
	return nil
}

// CancelChallenge makes an unfinished browser sign-in unusable after logout.
func (s *Store) CancelChallenge(ctx context.Context, token string) error {
	digest, err := tokenDigest(token, "challenge")
	if err != nil {
		return err
	}
	if err = authdb.New(s.pool).DeleteChallenge(ctx, digest); err != nil {
		return ErrStorage
	}
	return nil
}

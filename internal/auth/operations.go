package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/techize/selloovy/internal/authdb"
)

var ErrRateLimited = errors.New("authentication attempt limit reached")
var ErrAlreadySetup = errors.New("an owner already exists")

// AllowAttempt uses keyed digests rather than retaining client IPs or emails.
// Limits count all attempts, including successes, within a shared 15-minute window.
func (s *Store) AllowAttempt(ctx context.Context, scope, value string, maximum int32) error {
	mac := hmac.New(sha256.New, s.vault.limitKey[:])
	_, _ = mac.Write([]byte(scope + "\x00" + value))
	count, err := authdb.New(s.pool).TakeAuthAttempt(ctx, mac.Sum(nil))
	if err != nil {
		return ErrStorage
	}
	if count > maximum {
		return ErrRateLimited
	}
	return nil
}

func LoadKeyFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != 32 {
		return nil, errors.New("authentication key file must be a private regular file containing 32 bytes")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("authentication key file is unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0077 != 0 {
		return nil, errors.New("authentication key file changed or is not private")
	}
	key, err := io.ReadAll(io.LimitReader(file, 33))
	if err != nil || len(key) != 32 {
		return nil, errors.New("authentication key file is unavailable")
	}
	return key, nil
}

// CreateFirstOwner is available only to an operator command. Creation is atomic
// and concurrent commands cannot create multiple initial owners or stray shops.
func (s *Store) CreateFirstOwner(ctx context.Context, name, email, password string) (int64, MFASecret, error) {
	name = strings.TrimSpace(name)
	email, err := normalizedEmail(email)
	if err != nil || !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 120 {
		return 0, MFASecret{}, ErrCredential
	}
	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return 0, MFASecret{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	defer rollback(tx)
	q := authdb.New(tx)
	if err = q.LockOwnerSetup(ctx); err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	count, err := q.OwnerCount(ctx)
	if err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	if count != 0 {
		return 0, MFASecret{}, ErrAlreadySetup
	}
	shop, err := q.CreateSetupShop(ctx, name)
	if err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	id, err := q.CreateOwner(ctx, authdb.CreateOwnerParams{ShopID: shop, Email: email, PasswordHash: hash})
	if err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	secret, err := NewMFASecret()
	if err != nil {
		return 0, MFASecret{}, err
	}
	encrypted, err := s.vault.Seal(id, secret)
	if err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	if err = q.SaveMFASeed(ctx, authdb.SaveMFASeedParams{ID: id, MfaCiphertext: encrypted}); err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	return id, secret, nil
}
func (s *Store) ResumeEnrollment(ctx context.Context, email, password string) (int64, MFASecret, error) {
	email, err := normalizedEmail(email)
	if err != nil {
		return 0, MFASecret{}, ErrCredential
	}
	if err = s.AllowAttempt(ctx, "enrollment", email, 10); err != nil {
		return 0, MFASecret{}, err
	}
	row, err := authdb.New(s.pool).PendingOwnerByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, MFASecret{}, ErrCredential
	}
	if err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	if err = s.hasher.Verify(ctx, password, row.PasswordHash); err != nil {
		return 0, MFASecret{}, err
	}
	if row.MfaEnabled {
		return 0, MFASecret{}, ErrCredential
	}
	secret, err := s.vault.Open(row.ID, row.MfaCiphertext)
	if err != nil {
		return 0, MFASecret{}, ErrStorage
	}
	return row.ID, secret, nil
}

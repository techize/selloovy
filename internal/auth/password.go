// Package auth contains credential primitives. It does not expose login routes.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

var (
	ErrPassword   = errors.New("password must contain 15 to 128 characters without control characters")
	ErrCredential = errors.New("credential is invalid")
	ErrBusy       = errors.New("credential verification is busy")
)

// Fixed, versioned parameters bound the cost even for a corrupted stored hash.
// Changing them requires an explicit migration/rehash strategy.
const passwordPrefix = "$argon2id$v=19$m=65536,t=3,p=1$"

// PasswordHasher bounds memory-intensive work. Share one instance per process.
// This is resource protection, not account/IP login rate limiting.
type PasswordHasher struct{ slots chan struct{} }

func NewPasswordHasher() *PasswordHasher {
	return &PasswordHasher{slots: make(chan struct{}, 2)}
}

func validPassword(password string) bool {
	if !utf8.ValidString(password) || len(password) > 512 {
		return false
	}
	n := utf8.RuneCountInString(password)
	if n < 15 || n > 128 {
		return false
	}
	for _, r := range password {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func (h *PasswordHasher) acquire(ctx context.Context) error {
	if h == nil || h.slots == nil {
		return ErrBusy
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case h.slots <- struct{}{}:
		return nil
	default:
		return ErrBusy
	}
}

func (h *PasswordHasher) Hash(ctx context.Context, password string) (string, error) {
	if !validPassword(password) {
		return "", ErrPassword
	}
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	defer func() { <-h.slots }()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", ErrCredential
	}
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	// Argon2 cannot be interrupted mid-call; do not return a result after cancellation.
	defer clear(key)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return passwordPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}

// Verify never trims, normalizes or silently truncates the supplied password.
// A mismatch or malformed stored record returns the same credential error.
func (h *PasswordHasher) Verify(ctx context.Context, password, encoded string) error {
	if !validPassword(password) || !strings.HasPrefix(encoded, passwordPrefix) || len(encoded) > 128 {
		return ErrCredential
	}
	parts := strings.Split(strings.TrimPrefix(encoded, passwordPrefix), "$")
	if len(parts) != 2 {
		return ErrCredential
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[0])
	if err != nil || len(salt) != 16 {
		return ErrCredential
	}
	want, err := base64.RawStdEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(want) != 32 {
		return ErrCredential
	}
	if err := h.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-h.slots }()
	got := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	defer clear(got)
	if err := ctx.Err(); err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrCredential
	}
	return nil
}

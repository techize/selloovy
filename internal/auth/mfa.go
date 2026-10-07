package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// MFASecret must only be revealed to the owner during secure enrollment.
// Formatting is always redacted, including debug verbs; fields are unexported.
type MFASecret struct{ key [20]byte }

func (MFASecret) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "[redacted MFA secret]") }

func NewMFASecret() (MFASecret, error) {
	var secret MFASecret
	if _, err := rand.Read(secret.key[:]); err != nil {
		return MFASecret{}, ErrCredential
	}
	return secret, nil
}

func (s MFASecret) EnrollmentKey() string {
	if s.key == ([20]byte{}) {
		return ""
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(s.key[:])
}

// MatchTOTP returns the counter that MUST be atomically persisted with issuance
// of an authenticated session. Passing a stale lastUsed is not replay protection.
// Compatible profile: HMAC-SHA1, six digits, 30 seconds, at most one step drift.
func (s MFASecret) MatchTOTP(code string, now time.Time, lastUsed int64) (int64, error) {
	if s.key == ([20]byte{}) || len(code) != 6 || now.Unix() < 0 || lastUsed < -1 {
		return 0, ErrCredential
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return 0, ErrCredential
		}
	}
	current := now.Unix() / 30
	matched := int64(-1)
	// Check every candidate, accepting the newest match if codes collide.
	for _, step := range []int64{current - 1, current, current + 1} {
		if step < 0 {
			continue
		}
		expected := s.code(step)
		if subtle.ConstantTimeCompare([]byte(code), []byte(expected)) == 1 && step > lastUsed {
			matched = step
		}
	}
	if matched < 0 {
		return 0, ErrCredential
	}
	return matched, nil
}

func (s MFASecret) code(counter int64) string {
	var moving [8]byte
	binary.BigEndian.PutUint64(moving[:], uint64(counter))
	mac := hmac.New(sha1.New, s.key[:])
	_, _ = mac.Write(moving[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000)
}

// MFAVault encrypts seeds with an externally supplied 32-byte key, never a
// password or a key stored alongside ciphertext in the merchant database.
type MFAVault struct{ aead cipher.AEAD }

func NewMFAVault(key []byte) (*MFAVault, error) {
	if len(key) != 32 {
		return nil, ErrCredential
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrCredential
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, ErrCredential
	}
	return &MFAVault{aead: aead}, nil
}

func ownerContext(ownerID int64) []byte {
	return []byte(fmt.Sprintf("selloovy/mfa/v1/owner/%d", ownerID))
}

func (v *MFAVault) Seal(ownerID int64, secret MFASecret) ([]byte, error) {
	if v == nil || v.aead == nil || ownerID < 1 || secret.key == ([20]byte{}) {
		return nil, ErrCredential
	}
	// NewGCMWithRandomNonce prepends a fresh nonce; nil is required here.
	return v.aead.Seal(nil, nil, secret.key[:], ownerContext(ownerID)), nil
}

func (v *MFAVault) Open(ownerID int64, ciphertext []byte) (MFASecret, error) {
	if v == nil || v.aead == nil || ownerID < 1 || len(ciphertext) != 20+v.aead.Overhead() {
		return MFASecret{}, ErrCredential
	}
	plain, err := v.aead.Open(nil, nil, ciphertext, ownerContext(ownerID))
	if err != nil {
		return MFASecret{}, ErrCredential
	}
	defer clear(plain)
	var secret MFASecret
	copy(secret.key[:], plain)
	return secret, nil
}

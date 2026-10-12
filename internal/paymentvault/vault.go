// Package paymentvault protects unresolved payment source tokens. It is not an
// order lifecycle: only a durable attempt may supply the authenticated context.
package paymentvault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var ErrVault = errors.New("payment envelope unavailable")
var keyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var locationPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

type Context struct {
	ShopID, AmountPence                            int64
	AttemptID, IdempotencyKey, Location, QuoteHash string
}

func (Context) String() string               { return "Payment envelope context [private]" }
func (Context) GoString() string             { return "Payment envelope context [private]" }
func (Context) MarshalJSON() ([]byte, error) { return []byte(`{"private":true}`), nil }

type Vault struct{ aead cipher.AEAD }

func (*Vault) String() string   { return "Payment vault [private]" }
func (*Vault) GoString() string { return "Payment vault [private]" }
func New(installationKey []byte) (*Vault, error) {
	if len(installationKey) != 32 {
		return nil, ErrVault
	}
	key, e := hkdf.Key(sha256.New, installationKey, nil, "selloovy/payment-envelope/v1", 32)
	if e != nil {
		return nil, ErrVault
	}
	defer clear(key)
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, ErrVault
	}
	aead, e := cipher.NewGCMWithRandomNonce(block)
	if e != nil {
		return nil, ErrVault
	}
	return &Vault{aead: aead}, nil
}
func aad(c Context) ([]byte, error) {
	if c.ShopID < 1 || c.AmountPence < 1 || c.AmountPence > 1_000_000_000_000 || !keyPattern.MatchString(c.AttemptID) || !keyPattern.MatchString(c.IdempotencyKey) || !locationPattern.MatchString(c.Location) || !hashPattern.MatchString(c.QuoteHash) {
		return nil, ErrVault
	}
	// Each string field has a delimiter-free validated alphabet. GBP and version
	// are fixed, binding both the provider request and its saved checkout quote.
	return []byte(fmt.Sprintf("selloovy/payment-envelope/v1/GBP/%d/%d/%s/%s/%s/%s", c.ShopID, c.AmountPence, c.AttemptID, c.IdempotencyKey, c.Location, c.QuoteHash)), nil
}
func validToken(token string) bool {
	if !strings.HasPrefix(token, "cnon:") || (len(token) <= 5 || len(token) > 4096) {
		return false
	}
	for _, b := range []byte(token) {
		if b < 33 || b > 126 {
			return false
		}
	}
	return true
}
func (v *Vault) Seal(c Context, token string) ([]byte, error) {
	if !validToken(token) {
		return nil, ErrVault
	}
	associated, e := aad(c)
	if e != nil {
		return nil, e
	}
	data := []byte(token)
	defer clear(data)
	return v.aead.Seal(nil, nil, data, associated), nil
}
func (v *Vault) Open(c Context, encrypted []byte) (string, error) {
	associated, e := aad(c)
	if e != nil || len(encrypted) < 28 || len(encrypted) > 4124 {
		return "", ErrVault
	}
	plain, e := v.aead.Open(nil, nil, encrypted, associated)
	if e != nil {
		return "", ErrVault
	}
	defer clear(plain)
	token := string(plain)
	if !validToken(token) {
		return "", ErrVault
	}
	return token, nil
}

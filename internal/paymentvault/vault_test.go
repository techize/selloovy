package paymentvault

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func fixture() Context {
	return Context{ShopID: 1, AmountPence: 2498, AttemptID: strings.Repeat("a", 32), IdempotencyKey: strings.Repeat("b", 32), Location: "fixture-location", QuoteHash: strings.Repeat("c", 64)}
}
func TestEnvelopeBindsIntentAndInstallation(t *testing.T) {
	v, e := New(make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	c := fixture()
	token := "cnon:synthetic-private-source"
	sealed, e := v.Seal(c, token)
	if e != nil || bytes.Contains(sealed, []byte(token)) {
		t.Fatal("source not encrypted")
	}
	got, e := v.Open(c, sealed)
	if e != nil || got != token {
		t.Fatal("round trip failed")
	}
	second, e := v.Seal(c, token)
	if e != nil || bytes.Equal(second, sealed) {
		t.Fatal("nonce reused")
	}
	for _, change := range []func(*Context){func(c *Context) { c.ShopID++ }, func(c *Context) { c.AmountPence++ }, func(c *Context) { c.AttemptID = strings.Repeat("d", 32) }, func(c *Context) { c.IdempotencyKey = strings.Repeat("d", 32) }, func(c *Context) { c.Location = "other-location" }, func(c *Context) { c.QuoteHash = strings.Repeat("d", 64) }} {
		altered := c
		change(&altered)
		if _, e = v.Open(altered, sealed); !errors.Is(e, ErrVault) {
			t.Fatal("different financial context accepted")
		}
	}
	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	if _, e = v.Open(c, tampered); !errors.Is(e, ErrVault) {
		t.Fatal("tampered ciphertext accepted")
	}
	other, _ := New(bytes.Repeat([]byte{1}, 32))
	if _, e = other.Open(c, sealed); !errors.Is(e, ErrVault) {
		t.Fatal("different installation key accepted")
	}
	restored, _ := New(make([]byte, 32))
	if got, e = restored.Open(c, sealed); e != nil || got != token {
		t.Fatal("same-key restore failed")
	}
	for _, size := range []int{0, 16, 31, 33} {
		if _, e = New(make([]byte, size)); !errors.Is(e, ErrVault) {
			t.Fatal("invalid key accepted")
		}
	}
	for _, bad := range []string{"CASH", "ccof:stored", "cnon:header\r\ninjection", strings.Repeat("x", 5000)} {
		if _, e = v.Seal(c, bad); !errors.Is(e, ErrVault) {
			t.Fatal("invalid source accepted")
		}
	}
	for _, bad := range [][]byte{nil, make([]byte, 27), make([]byte, 4125)} {
		if _, e = v.Open(c, bad); !errors.Is(e, ErrVault) {
			t.Fatal("invalid envelope accepted")
		}
	}
	for _, value := range []any{v, c} {
		j, _ := json.Marshal(value)
		s := fmt.Sprintf("%v %#v %s", value, value, j)
		if strings.Contains(s, c.Location) || strings.Contains(s, c.IdempotencyKey) {
			t.Fatal("implicit formatting exposed intent")
		}
	}
}

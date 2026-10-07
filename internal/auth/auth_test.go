package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPasswordHashAndVerify(t *testing.T) {
	h := NewPasswordHasher()
	password := "a synthetic café passphrase"
	one, err := h.Hash(t.Context(), password)
	if err != nil {
		t.Fatal(err)
	}
	two, err := h.Hash(t.Context(), password)
	if err != nil {
		t.Fatal(err)
	}
	if one == two || strings.Contains(one, password) {
		t.Fatal("salts must differ; plaintext must not be retained")
	}
	if err := h.Verify(t.Context(), password, one); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []string{password + " ", "a different synthetic passphrase", "short"} {
		if !errors.Is(h.Verify(t.Context(), wrong, one), ErrCredential) {
			t.Fatal("invalid password was accepted")
		}
	}
	for _, damaged := range []string{"", "$argon2id$v=19$m=999999999,t=3,p=1$bad$bad", strings.Replace(one, "v=19", "v=16", 1), one + "$extra", strings.Replace(one, "$", "!", 1)} {
		if !errors.Is(h.Verify(t.Context(), password, damaged), ErrCredential) {
			t.Fatal("unsupported/corrupt hash accepted")
		}
	}
}

func TestPasswordPolicyAndResourceBound(t *testing.T) {
	h := NewPasswordHasher()
	for _, password := range []string{"", strings.Repeat("x", 7), strings.Repeat("🦕", 7), strings.Repeat("x", 129), "a synthetic\npassphrase", string([]byte{0xff}) + strings.Repeat("x", 20)} {
		if _, err := h.Hash(t.Context(), password); !errors.Is(err, ErrPassword) {
			t.Fatal("invalid password policy")
		}
	}
	for _, password := range []string{strings.Repeat("界", 8), strings.Repeat("x", 8), strings.Repeat("x", 128), " spaces are preserved "} {
		if !validPassword(password) {
			t.Fatal("valid passphrase rejected")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := h.Hash(ctx, "a synthetic passphrase"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
	// Occupied slots must fail fast; no unbounded queue holding passwords.
	h.slots <- struct{}{}
	h.slots <- struct{}{}
	if _, err := h.Hash(t.Context(), "a synthetic passphrase"); !errors.Is(err, ErrBusy) {
		t.Fatal("resource limit ignored")
	}
	<-h.slots
	<-h.slots
	// Exercise shared-instance synchronization under the race detector.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := h.acquire(t.Context()); err == nil {
				<-h.slots
			} else if !errors.Is(err, ErrBusy) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	var missing *PasswordHasher
	if _, err := missing.Hash(t.Context(), "a synthetic passphrase"); !errors.Is(err, ErrBusy) {
		t.Fatal("missing hasher did not fail closed")
	}
}

func TestTOTPRFCVectorsAndReplayBoundary(t *testing.T) {
	// Public RFC 6238 Appendix B SHA1 test key, never a deployed credential.
	var secret MFASecret
	copy(secret.key[:], []byte("12345678901234567890"))
	// RFC's eight-digit results reduced modulo 1,000,000 for our six-digit profile.
	for _, tc := range []struct {
		second int64
		code   string
	}{
		{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"},
		{1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"},
	} {
		now := time.Unix(tc.second, 0)
		step, err := secret.MatchTOTP(tc.code, now, -1)
		if err != nil || step != tc.second/30 {
			t.Fatal("RFC vector failed")
		}
		if _, err := secret.MatchTOTP(tc.code, now, step); !errors.Is(err, ErrCredential) {
			t.Fatal("consumed step accepted")
		}
	}
	now := time.Unix(1234567890, 0)
	current := now.Unix() / 30
	for _, step := range []int64{current - 1, current, current + 1} {
		got, err := secret.MatchTOTP(secret.code(step), now, -1)
		if err != nil || got != step {
			t.Fatal("one-step drift rejected")
		}
	}
	for _, code := range []string{"", "12345", "1234567", "１２３４５６", "12x456", " 005924", secret.code(current - 2), secret.code(current + 2)} {
		if _, err := secret.MatchTOTP(code, now, -1); !errors.Is(err, ErrCredential) {
			t.Fatal("invalid/outside-window code accepted")
		}
	}
	if _, err := secret.MatchTOTP("005924", time.Unix(-1, 0), -1); err == nil {
		t.Fatal("negative time accepted")
	}
	if _, err := secret.MatchTOTP("005924", now, -2); err == nil {
		t.Fatal("invalid watermark accepted")
	}
	if _, err := secret.MatchTOTP(secret.code(current-1), now, current); err == nil {
		t.Fatal("older step accepted after newer step")
	}
}

func TestMFAVaultBindingAndTampering(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	vault, err := NewMFAVault(key)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := NewMFASecret()
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewMFASecret()
	if err != nil {
		t.Fatal(err)
	}
	if secret.EnrollmentKey() == other.EnrollmentKey() || len(secret.EnrollmentKey()) != 32 {
		t.Fatal("seed generation failed")
	}
	one, err := vault.Seal(1, secret)
	if err != nil {
		t.Fatal(err)
	}
	two, err := vault.Seal(1, secret)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(one, two) || bytes.Contains(one, secret.key[:]) {
		t.Fatal("nonce reuse/plaintext storage")
	}
	decoded, err := vault.Open(1, one)
	if err != nil || decoded.EnrollmentKey() != secret.EnrollmentKey() {
		t.Fatal("round trip failed")
	}
	if _, err := vault.Open(2, one); err == nil {
		t.Fatal("owner binding ignored")
	}
	wrongKey := bytes.Clone(key)
	wrongKey[0] ^= 1
	wrongVault, _ := NewMFAVault(wrongKey)
	if _, err := wrongVault.Open(1, one); err == nil {
		t.Fatal("wrong key accepted")
	}
	for i := range one {
		damaged := bytes.Clone(one)
		damaged[i] ^= 1
		if _, err := vault.Open(1, damaged); err == nil {
			t.Fatal("modified ciphertext accepted")
		}
	}
	if _, err := vault.Seal(0, secret); err == nil {
		t.Fatal("invalid owner accepted")
	}
	if _, err := vault.Open(0, one); err == nil {
		t.Fatal("invalid owner accepted")
	}
	if _, err := NewMFAVault(key[:31]); err == nil {
		t.Fatal("short key accepted")
	}
	var empty MFASecret
	if empty.EnrollmentKey() != "" {
		t.Fatal("uninitialized seed revealed")
	}
	if _, err := vault.Seal(1, empty); err == nil {
		t.Fatal("uninitialized seed accepted")
	}
	if _, err := empty.MatchTOTP("123456", time.Now(), -1); err == nil {
		t.Fatal("uninitialized seed accepted")
	}
	var missing *MFAVault
	if _, err := missing.Seal(1, secret); err == nil {
		t.Fatal("missing vault did not fail closed")
	}
	if _, err := missing.Open(1, one); err == nil {
		t.Fatal("missing vault did not fail closed")
	}
}

func TestRecoveryDigestsAndRedaction(t *testing.T) {
	codes, digests, err := NewRecoveryCodes(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 10 || len(digests) != 10 {
		t.Fatal("wrong recovery count")
	}
	seen := make(map[[32]byte]bool)
	for i, code := range codes {
		got, err := RecoveryDigest(1, strings.ToLower(code.Reveal()))
		if err != nil || got != digests[i] || seen[got] {
			t.Fatal("invalid/duplicate recovery code")
		}
		seen[got] = true
		other, err := RecoveryDigest(2, code.Reveal())
		if err != nil || other == got {
			t.Fatal("recovery digest owner binding ignored")
		}
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			if strings.Contains(fmt.Sprintf(verb, code), code.Reveal()) {
				t.Fatal("recovery code leaked through formatting")
			}
		}
		encoded, _ := json.Marshal(code)
		if string(encoded) != "{}" {
			t.Fatal("recovery code serialized implicitly")
		}
	}
	secret, _ := NewMFASecret()
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		if fmt.Sprintf(verb, secret) != "[redacted MFA secret]" {
			t.Fatal("seed leaked through formatting")
		}
	}
	encoded, _ := json.Marshal(secret)
	if string(encoded) != "{}" {
		t.Fatal("seed serialized implicitly")
	}
	for _, invalid := range []string{"", "not a recovery code", strings.Repeat("Z", 32), strings.Repeat("0", 38)} {
		if _, err := RecoveryDigest(1, invalid); err == nil {
			t.Fatal("invalid recovery syntax accepted")
		}
	}
	if _, _, err := NewRecoveryCodes(0); err == nil {
		t.Fatal("invalid owner accepted")
	}
}

func FuzzMFAVaultOpen(f *testing.F) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	vault, _ := NewMFAVault(key)
	secret, _ := NewMFASecret()
	valid, _ := vault.Seal(1, secret)
	f.Add([]byte{})
	f.Add([]byte("synthetic malformed ciphertext"))
	f.Add(valid)
	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := vault.Open(1, data)
		if err == nil && got.EnrollmentKey() != secret.EnrollmentKey() {
			t.Fatal("unexpected authenticated plaintext")
		}
	})
}

func TestAuthenticationTokenBoundaries(t *testing.T) {
	token := newToken()
	challenge, err := tokenDigest(token.Reveal(), "challenge")
	if err != nil {
		t.Fatal(err)
	}
	session, err := tokenDigest(token.Reveal(), "session")
	if err != nil || bytes.Equal(challenge, session) {
		t.Fatal("token purposes were not separated")
	}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		if strings.Contains(fmt.Sprintf(verb, token), token.Reveal()) {
			t.Fatal("bearer token leaked through formatting")
		}
	}
	encoded, _ := json.Marshal(token)
	if string(encoded) != "{}" {
		t.Fatal("bearer token serialized implicitly")
	}
	for _, invalid := range []string{"", "not a token", strings.Repeat("!", 43), strings.Repeat("A", 44)} {
		if _, err := tokenDigest(invalid, "session"); err == nil {
			t.Fatal("malformed token accepted")
		}
	}
}

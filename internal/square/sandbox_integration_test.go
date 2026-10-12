package square

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

// Explicit opt-in: real sandbox API calls using official synthetic source tokens.
// This tests the HTTP adapter only, not browser SCA or a Selloovy checkout order.
func TestActualSandboxAdapter(t *testing.T) {
	if os.Getenv("SELLOOVY_RUN_SQUARE_SANDBOX_TESTS") != "yes" {
		t.Skip("explicit sandbox opt-in required")
	}
	c, e := NewSandbox(os.Getenv("SELLOOVY_SQUARE_SANDBOX_ACCESS_TOKEN_FILE"), os.Getenv("SELLOOVY_SQUARE_SANDBOX_LOCATION_ID"))
	if e != nil {
		t.Fatal("private sandbox configuration unavailable")
	}
	for _, tc := range []struct {
		source string
		want   State
	}{{"cnon:card-nonce-ok", Paid}, {"cnon:card-nonce-declined", Declined}} {
		var ids [32]byte
		if _, e = rand.Read(ids[:]); e != nil {
			t.Fatal("fixture generation failed")
		}
		req := Request{SourceToken: tc.source, IdempotencyKey: hex.EncodeToString(ids[:16]), Reference: hex.EncodeToString(ids[16:]), AmountPence: 100}
		r, e := c.Charge(t.Context(), req)
		if e != nil || r.State != tc.want || r.ProviderID == "" {
			t.Fatal("sandbox adapter outcome did not match fixture")
		}
		again, e := c.Charge(t.Context(), req)
		if e != nil || again.State != r.State || again.ProviderID != r.ProviderID {
			t.Fatal("stable-key replay changed provider outcome")
		}
		got, e := c.Reconcile(t.Context(), r.ProviderID, req)
		if e != nil || got.State != r.State || got.ProviderID != r.ProviderID {
			t.Fatal("sandbox reconciliation mismatch")
		}
	}
}

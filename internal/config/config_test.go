package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		present, invalid  bool
	}{
		{name: "default", want: "127.0.0.1:8080"},
		{name: "explicit IPv4", input: "127.0.0.1:9000", present: true, want: "127.0.0.1:9000"},
		{name: "IPv6", input: "[::1]:8080", present: true, want: "[::1]:8080"},
		{name: "blank", present: true, invalid: true},
		{name: "missing host", input: ":8080", present: true, invalid: true},
		{name: "zero port", input: "127.0.0.1:0", present: true, invalid: true},
		{name: "large port", input: "127.0.0.1:65536", present: true, invalid: true},
		{name: "invalid input is redacted", input: "private-runtime-value", present: true, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(func(key string) (string, bool) {
				if key == "SELLOOVY_HTTP_ADDR" {
					return tc.input, tc.present
				}
				return "", false
			})
			if tc.invalid {
				if err == nil {
					t.Fatal("expected invalid configuration to fail")
				}
				if tc.input != "" && strings.Contains(err.Error(), tc.input) {
					t.Fatal("error exposed configuration value")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.HTTPAddr != tc.want {
				t.Fatalf("address = %q, want %q", cfg.HTTPAddr, tc.want)
			}
		})
	}
}

func TestBlankDatabaseURLIsRejected(t *testing.T) {
	_, err := Load(func(key string) (string, bool) {
		if key == "SELLOOVY_DATABASE_URL" {
			return " ", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("explicitly blank database configuration must fail")
	}
}

func TestAuthenticationConfiguration(t *testing.T) {
	for _, tc := range []struct {
		origin, addr string
		valid        bool
	}{
		{"http://127.0.0.1:8080", "127.0.0.1:8080", true},
		{"https://shop.example.com", "0.0.0.0:8080", true},
		{"http://shop.example.com", "127.0.0.1:8080", false},
		{"http://127.0.0.1:8080", "0.0.0.0:8080", false},
		{"https://shop.example.com/path", "127.0.0.1:8080", false},
	} {
		cfg, err := Load(func(k string) (string, bool) {
			values := map[string]string{"SELLOOVY_AUTH_KEY_FILE": "local/auth.key", "SELLOOVY_PUBLIC_ORIGIN": tc.origin, "SELLOOVY_DATABASE_URL": "synthetic database configuration", "SELLOOVY_HTTP_ADDR": tc.addr}
			v, ok := values[k]
			return v, ok
		})
		if (err == nil) != tc.valid {
			t.Fatal("incorrect authentication configuration policy")
		}
		if tc.valid && cfg.PublicOrigin != tc.origin {
			t.Fatal("origin changed")
		}
	}
}

func TestLocalhostAuthOriginRequiresLoopbackListener(t *testing.T) {
	for _, tc := range []struct {
		origin, bind string
		valid        bool
	}{
		{"http://localhost:8080", "127.0.0.1:8080", true},
		{"http://localhost:8080", "[::1]:8080", true},
		{"http://localhost:8080", "0.0.0.0:8080", false},
		{"http://localhost.example.com:8080", "127.0.0.1:8080", false},
		{"http://localhost.:8080", "127.0.0.1:8080", false},
	} {
		vars := map[string]string{"SELLOOVY_HTTP_ADDR": tc.bind, "SELLOOVY_DATABASE_URL": "postgres://fixture@127.0.0.1/shop?sslmode=disable", "SELLOOVY_AUTH_KEY_FILE": "fixture.key", "SELLOOVY_PUBLIC_ORIGIN": tc.origin}
		_, e := Load(func(k string) (string, bool) { v, ok := vars[k]; return v, ok })
		if (e == nil) != tc.valid {
			t.Fatal("localhost origin/listener boundary failed")
		}
	}
}

func TestSandboxPaymentConfigurationBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, origin, app string
		omit              string
		valid             bool
	}{
		{"localhost", "http://localhost:8080", "sandbox-fixture", "", true},
		{"https", "https://shop.example.com", "sandbox-fixture", "", true},
		{"IP HTTP unsupported by SDK", "http://127.0.0.1:8080", "sandbox-fixture", "", false},
		{"live application rejected", "http://localhost:8080", "live-fixture", "", false},
		{"partial missing token", "http://localhost:8080", "sandbox-fixture", "SELLOOVY_SQUARE_SANDBOX_ACCESS_TOKEN_FILE", false},
		{"partial missing location", "http://localhost:8080", "sandbox-fixture", "SELLOOVY_SQUARE_SANDBOX_LOCATION_ID", false},
		{"partial missing app", "http://localhost:8080", "sandbox-fixture", "SELLOOVY_SQUARE_SANDBOX_APP_ID", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]string{"SELLOOVY_DATABASE_URL": "fixture", "SELLOOVY_AUTH_KEY_FILE": "fixture.key", "SELLOOVY_PUBLIC_ORIGIN": tc.origin, "SELLOOVY_SQUARE_SANDBOX_ACCESS_TOKEN_FILE": "fixture.token", "SELLOOVY_SQUARE_SANDBOX_LOCATION_ID": "fixture-location", "SELLOOVY_SQUARE_SANDBOX_APP_ID": tc.app}
			delete(vars, tc.omit)
			_, e := Load(func(k string) (string, bool) { v, ok := vars[k]; return v, ok })
			if (e == nil) != tc.valid {
				t.Fatal("sandbox configuration policy incorrect")
			}
		})
	}
}

func TestBrevoWorkerRequiresExplicitMode(t *testing.T) {
	for _, tc := range []struct {
		mode, omit string
		valid      bool
	}{{"drop", "", true}, {"deliver", "", false}, {"", "", false}, {"drop", "SELLOOVY_BREVO_API_KEY_FILE", false}, {"drop", "SELLOOVY_BREVO_SENDER_EMAIL", false}, {"drop", "SELLOOVY_DATABASE_URL", false}, {"drop", "SELLOOVY_BREVO_MODE", false}} {
		vars := map[string]string{"SELLOOVY_DATABASE_URL": "fixture", "SELLOOVY_BREVO_API_KEY_FILE": "fixture.token", "SELLOOVY_BREVO_SENDER_EMAIL": "sender@example.com", "SELLOOVY_BREVO_MODE": tc.mode}
		delete(vars, tc.omit)
		cfg, e := Load(func(k string) (string, bool) { v, ok := vars[k]; return v, ok })
		if (e == nil) != tc.valid {
			t.Fatal("email activation policy incorrect")
		}
		if tc.valid && cfg.BrevoKeyFile != "fixture.token" {
			t.Fatal("configuration missing")
		}
	}
}

func TestBrevoDeliveryRequiresActivationBoundary(t *testing.T) {
	for _, tc := range []struct {
		mode, after string
		valid       bool
	}{
		{"deliver", "2026-10-01T12:00:00Z", true},
		{"deliver", "2026-10-01T13:00:00+01:00", true},
		{"deliver", "", false}, {"deliver", "today", false},
		{"deliver", "0001-01-01T00:00:00Z", false},
		{"drop", "2026-10-01T12:00:00Z", false},
	} {
		vars := map[string]string{"SELLOOVY_DATABASE_URL": "fixture", "SELLOOVY_BREVO_API_KEY_FILE": "fixture.token", "SELLOOVY_BREVO_SENDER_EMAIL": "sender@example.com", "SELLOOVY_BREVO_MODE": tc.mode, "SELLOOVY_BREVO_DELIVERY_AFTER": tc.after, "SELLOOVY_BREVO_SENDER_NAME": "Fixture shop"}
		cfg, e := Load(func(k string) (string, bool) { v, ok := vars[k]; return v, ok })
		if (e == nil) != tc.valid {
			t.Fatalf("activation boundary policy incorrect for %q", tc.after)
		}
		if tc.valid && (cfg.BrevoDeliveryAfter.Format("2006-01-02T15:04:05Z07:00") != "2026-10-01T12:00:00Z" || cfg.BrevoSenderName != "Fixture shop" || cfg.BrevoMode != "deliver") {
			t.Fatal("activation configuration lost")
		}
	}
	for _, key := range []string{"SELLOOVY_BREVO_DELIVERY_AFTER", "SELLOOVY_BREVO_SENDER_NAME"} {
		if _, e := Load(func(k string) (string, bool) { return "fixture", k == key }); e == nil {
			t.Fatal("orphan email configuration accepted")
		}
	}
}

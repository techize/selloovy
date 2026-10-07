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

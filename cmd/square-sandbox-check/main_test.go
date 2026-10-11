package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func privateToken(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "token")
	if os.WriteFile(p, []byte("synthetic-fixture-value\n"), 0600) != nil {
		t.Fatal("fixture failed")
	}
	return p
}

const eligible = `{"id":"fixture-location","country":"GB","currency":"GBP","status":"ACTIVE","capabilities":["CREDIT_CARD_PROCESSING"]}`

func TestReadOnlySandboxBoundary(t *testing.T) {
	path := privateToken(t)
	calls := 0
	transport := roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.String() != endpoint || r.Body != nil {
			t.Fatal("unexpected destination/mutation")
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-fixture-value" || r.Header.Get("Square-Version") != apiVersion {
			t.Fatal("request configuration missing")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"locations":[` + eligible + `]}`)), Header: make(http.Header)}, nil
	})
	if e := check(t.Context(), path, "", transport); e != nil || calls != 1 {
		t.Fatal("account check failed")
	}
	if e := check(t.Context(), path, "other-location", transport); e == nil {
		t.Fatal("foreign location accepted")
	}
}
func TestRejectedResponsesAndNoDisclosure(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"wrong region", strings.ReplaceAll(`{"locations":[`+eligible+`]}`, `"GB"`, `"US"`), 200},
		{"wrong currency", strings.ReplaceAll(`{"locations":[`+eligible+`]}`, `"GBP"`, `"USD"`), 200},
		{"inactive", strings.ReplaceAll(`{"locations":[`+eligible+`]}`, `"ACTIVE"`, `"INACTIVE"`), 200},
		{"no processing", `{"locations":[{"id":"fixture-location","country":"GB","currency":"GBP","status":"ACTIVE"}]}`, 200},
		{"ambiguous", `{"locations":[` + eligible + `,` + strings.ReplaceAll(eligible, "fixture-location", "second-location") + `]}`, 200},
		{"API errors", `{"errors":[{"detail":"private response"}],"locations":[` + eligible + `]}`, 200},
		{"malformed", "private response", 200},
		{"oversized", strings.Repeat("private response", maxResponse), 200},
		{"unauthorized", "private response", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := check(t.Context(), privateToken(t), "", roundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			}))
			if e == nil || strings.Contains(e.Error(), "private response") {
				t.Fatal("failed response accepted or disclosed")
			}
		})
	}
	e := check(t.Context(), privateToken(t), "", roundTrip(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("synthetic-fixture-value private response")
	}))
	if e == nil || strings.Contains(e.Error(), "synthetic-fixture-value") || strings.Contains(e.Error(), "private response") {
		t.Fatal("transport error leaked")
	}
}
func TestTokenFileProtectionAndRedirects(t *testing.T) {
	path := privateToken(t)
	called := false
	transport := roundTrip(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected request")
	})
	for _, mode := range []os.FileMode{0644, 0660} {
		if os.Chmod(path, mode) != nil {
			t.Fatal("fixture failed")
		}
		if e := check(t.Context(), path, "", transport); e == nil || called {
			t.Fatal("unsafe token used")
		}
	}
	if os.Chmod(path, 0600) != nil {
		t.Fatal("fixture failed")
	}
	link := filepath.Join(t.TempDir(), "link")
	if os.Symlink(path, link) != nil {
		t.Fatal("fixture failed")
	}
	if e := check(t.Context(), link, "", transport); e == nil || called {
		t.Fatal("symlink token accepted")
	}
	if os.WriteFile(path, []byte("header\r\ninjection"), 0600) != nil {
		t.Fatal("fixture failed")
	}
	if e := check(t.Context(), path, "", transport); e == nil || called {
		t.Fatal("header injection accepted")
	}
	path = privateToken(t)
	calls := 0
	e := check(t.Context(), path, "", roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != endpoint {
			t.Fatal("credential redirect followed")
		}
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://other.example.com/private"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if e == nil || calls != 1 || strings.Contains(e.Error(), "other.example.com") {
		t.Fatal("redirect boundary failed")
	}
}

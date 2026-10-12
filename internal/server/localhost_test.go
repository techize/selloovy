package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalhostDocumentAliasKeepsAPIsAndMutationsGuarded(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) })
	h := LocalhostAlias(next, "http://localhost:8080")
	for _, tc := range []struct {
		method, host, path string
		want               int
	}{
		{"GET", "127.0.0.1:8080", "/admin/", 307},
		{"GET", "[::1]:8080", "/shop/", 307},
		{"GET", "localhost:8080", "/admin/", 403},
		{"GET", "localhost.example.com:8080", "/admin/", 403},
		{"GET", "127.0.0.1:8081", "/admin/", 403},
		{"GET", "127.0.0.1:8080", "/api/auth/session", 403},
		{"POST", "127.0.0.1:8080", "/shop/checkout", 403},
	} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+tc.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatal("local alias boundary failed")
		}
		if tc.want == 307 && w.Header().Get("Location") != "http://localhost:8080"+tc.path {
			t.Fatal("redirect escaped configured origin")
		}
	}
}

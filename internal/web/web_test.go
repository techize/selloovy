package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		check            ReadinessCheck
		code             int
	}{
		{name: "alive without persistence", path: "/health/live", code: 200, body: `{"status":"alive"}`},
		{name: "not ready without persistence", path: "/health/ready", code: 503, body: `{"status":"not_ready"}`},
		{name: "dependency failed without exposure", path: "/health/ready", code: 503, body: `{"status":"not_ready"}`, check: func(context.Context) error { return errors.New("private dependency detail") }},
		{name: "dependency succeeded", path: "/health/ready", code: 200, body: `{"status":"ready"}`, check: func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("readiness check must have a deadline")
			}
			return nil
		}},
		{name: "liveness independent of dependency", path: "/health/live", code: 200, body: `{"status":"alive"}`, check: func(context.Context) error { t.Error("liveness queried dependency"); return errors.New("unavailable") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewHandler(tc.check)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != tc.code || strings.TrimSpace(response.Body.String()) != tc.body {
				t.Fatalf("unexpected health response: %d %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("health response can be cached")
			}
		})
	}
}

func TestPublicRouteBoundary(t *testing.T) {
	handler, err := NewHandler(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{http.MethodGet, "/", 200},
		{http.MethodPost, "/", 405},
		{http.MethodGet, "/admin", 404},
		{http.MethodPost, "/payments", 404},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != tc.code {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, response.Code, tc.code)
		}
		if tc.path == "/" && tc.code == 200 && !strings.Contains(response.Body.String(), "Foundation preview") {
			t.Error("preview status missing")
		}
	}
}

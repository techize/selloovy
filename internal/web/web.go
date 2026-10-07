// Package web serves the public application and health endpoints.
package web

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

//go:embed templates/home.html
var templates embed.FS

// ReadinessCheck must verify dependencies using the supplied bounded context.
// A nil check keeps readiness disabled until persistence is wired up.
type ReadinessCheck func(context.Context) error

func NewHandler(check ReadinessCheck) (http.Handler, error) {
	home, err := template.ParseFS(templates, "templates/home.html")
	if err != nil {
		return nil, err
	}
	router := chi.NewRouter()
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Static, embedded template with no external data or I/O while rendering.
		_ = home.ExecuteTemplate(w, "home.html", nil)
	})
	router.Get("/health/live", func(w http.ResponseWriter, r *http.Request) {
		writeHealth(w, http.StatusOK, `{"status":"alive"}`)
	})
	router.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if check == nil {
			writeHealth(w, http.StatusServiceUnavailable, `{"status":"not_ready"}`)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := check(ctx); err != nil {
			// Never expose dependency errors: they can contain connection details.
			writeHealth(w, http.StatusServiceUnavailable, `{"status":"not_ready"}`)
			return
		}
		writeHealth(w, http.StatusOK, `{"status":"ready"}`)
	})
	return router, nil
}

func writeHealth(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body + "\n"))
}

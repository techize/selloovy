// Package web serves the public application and health endpoints.
package web

import (
	"context"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

//go:embed templates/home.html
var templates embed.FS

// ReadinessCheck must verify dependencies using the supplied bounded context.
// A nil check keeps readiness disabled until persistence is wired up.
type ReadinessCheck func(context.Context) error

func NewHandler(check ReadinessCheck, admin fs.FS, authentication ...http.Handler) (http.Handler, error) {
	home, err := template.ParseFS(templates, "templates/home.html")
	if err != nil {
		return nil, err
	}
	router := chi.NewRouter()
	if len(authentication) > 0 && authentication[0] != nil {
		router.Mount("/api/auth", authentication[0])
	}
	if len(authentication) > 1 && authentication[1] != nil {
		router.Mount("/api/admin", authentication[1])
	}
	if len(authentication) > 2 && authentication[2] != nil {
		router.Mount("/shop", authentication[2])
	}
	router.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusPermanentRedirect)
	})
	router.Get("/admin/", func(w http.ResponseWriter, r *http.Request) {
		if admin == nil {
			http.Error(w, "Admin preview is unavailable", http.StatusServiceUnavailable)
			return
		}
		index, err := fs.ReadFile(admin, "index.html")
		if err != nil {
			http.Error(w, "Admin preview is unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		_, _ = w.Write(index)
	})
	router.Get("/admin/assets/*", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/admin/")
		if admin == nil || !fs.ValidPath(name) || !strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
		info, err := fs.Stat(admin, name)
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Revalidation avoids assumptions about filenames in arbitrary operator builds.
		w.Header().Set("Cache-Control", "no-cache")
		http.StripPrefix("/admin/", http.FileServer(http.FS(admin))).ServeHTTP(w, r)
	})
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

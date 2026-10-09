// Package storefronthttp serves deliberately published catalogue projections.
package storefronthttp

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/catalog"
)

//go:embed templates/*
var files embed.FS
var views = template.Must(template.New("shop").Funcs(template.FuncMap{
	"money": func(n int64) string { return fmt.Sprintf("£%d.%02d", n/100, n%100) },
	"minimum": func(v []catalog.PublicVariant) int64 {
		var min int64
		for _, p := range v {
			if min == 0 || p.PricePence < min {
				min = p.PricePence
			}
		}
		return min
	},
}).ParseFS(files, "templates/*.html"))

type Backend interface {
	PublicList(context.Context, string, int64) (catalog.PublicPage, error)
	PublicRead(context.Context, string, int64) (catalog.PublicShop, catalog.PublicProduct, error)
}
type page struct {
	Key       string
	Shop      catalog.PublicShop
	Products  []catalog.PublicProduct
	NextAfter int64
	Product   catalog.PublicProduct
	Selected  catalog.PublicVariant
	Detail    bool
}

func headers(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
}
func failure(w http.ResponseWriter, r *http.Request, e error) {
	headers(w)
	if errors.Is(e, catalog.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	http.Error(w, "Shop temporarily unavailable. Please try again.", http.StatusServiceUnavailable)
}
func queryNumber(r *http.Request, name string) (int64, bool) {
	q := r.URL.Query()
	if len(q) == 0 {
		return 0, true
	}
	if len(q) != 1 || len(q[name]) != 1 {
		return 0, false
	}
	n, e := strconv.ParseInt(q.Get(name), 10, 64)
	return n, e == nil && n > 0 && n <= 9007199254740991
}
func render(w http.ResponseWriter, r *http.Request, p page) {
	var body bytes.Buffer
	if views.ExecuteTemplate(&body, "shop.html", p) != nil {
		failure(w, r, catalog.ErrStorage)
		return
	}
	headers(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body.Bytes())
}
func New(b Backend) http.Handler {
	router := chi.NewRouter()
	router.Get("/assets/storefront.css", func(w http.ResponseWriter, r *http.Request) {
		css, e := files.ReadFile("templates/storefront.css")
		if e != nil {
			failure(w, r, e)
			return
		}
		headers(w)
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(css)
	})
	router.Get("/{key}", func(w http.ResponseWriter, r *http.Request) {
		after, ok := queryNumber(r, "after")
		if !ok {
			failure(w, r, catalog.ErrNotFound)
			return
		}
		key := chi.URLParam(r, "key")
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		data, e := b.PublicList(ctx, key, after)
		if e != nil {
			failure(w, r, e)
			return
		}
		render(w, r, page{Key: key, Shop: data.Shop, Products: data.Products, NextAfter: data.NextAfter})
	})
	router.Get("/{key}/products/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if e != nil || id < 1 || id > 9007199254740991 {
			failure(w, r, catalog.ErrNotFound)
			return
		}
		selected, ok := queryNumber(r, "variant")
		if !ok {
			failure(w, r, catalog.ErrNotFound)
			return
		}
		key := chi.URLParam(r, "key")
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		sh, p, e := b.PublicRead(ctx, key, id)
		if e != nil {
			failure(w, r, e)
			return
		}
		if len(p.Variants) == 0 {
			failure(w, r, catalog.ErrStorage)
			return
		}
		v := p.Variants[0]
		if selected > 0 {
			found := false
			for _, candidate := range p.Variants {
				if candidate.ID == selected {
					v = candidate
					found = true
					break
				}
			}
			if !found {
				failure(w, r, catalog.ErrNotFound)
				return
			}
		}
		render(w, r, page{Key: key, Shop: sh, Product: p, Selected: v, Detail: true})
	})
	return router
}

// Package cataloghttp exposes draft products behind the owner-session boundary.
package cataloghttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/catalog"
)

type Backend interface {
	List(context.Context, string, int64) (catalog.Page, error)
	Read(context.Context, string, int64) (catalog.Product, error)
	Save(context.Context, string, int64, catalog.Input) (catalog.Product, error)
	ReadMaker(context.Context, string, int64) (catalog.MakerSettings, error)
	SaveMaker(context.Context, string, int64, catalog.MakerInput) (catalog.MakerSettings, error)
}

func number(v string) (int64, bool) {
	n, e := strconv.ParseInt(v, 10, 64)
	return n, e == nil && n > 0 && n <= 9007199254740991
}
func New(b Backend) http.Handler {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		var after int64
		query := r.URL.Query()
		if len(query) > 1 || (len(query) == 1 && !query.Has("after")) {
			reply(w, 400, map[string]string{"error": "Invalid product list request."})
			return
		}
		if query.Has("after") {
			var ok bool
			after, ok = number(query.Get("after"))
			if !ok || len(query["after"]) != 1 {
				reply(w, 400, map[string]string{"error": "Invalid list cursor."})
				return
			}
		}
		data, e := b.List(r.Context(), authhttp.SessionToken(r.Context()), after)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, data)
	})
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := number(chi.URLParam(r, "id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, e := b.Read(r.Context(), authhttp.SessionToken(r.Context()), id)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, data)
	})
	save := func(w http.ResponseWriter, r *http.Request) {
		var id int64
		if r.Method == "PUT" {
			var ok bool
			id, ok = number(chi.URLParam(r, "id"))
			if !ok {
				http.NotFound(w, r)
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var in catalog.Input
		if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF {
			reply(w, 400, map[string]string{"error": "Invalid product request."})
			return
		}
		data, e := b.Save(r.Context(), authhttp.SessionToken(r.Context()), id, in)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, data)
	}
	r.Get("/{id}/maker", func(w http.ResponseWriter, r *http.Request) {
		id, ok := number(chi.URLParam(r, "id"))
		if !ok {
			reply(w, 404, map[string]string{"error": "Product not found."})
			return
		}
		data, e := b.ReadMaker(r.Context(), authhttp.SessionToken(r.Context()), id)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, data)
	})
	r.Put("/{id}/maker", func(w http.ResponseWriter, r *http.Request) {
		id, ok := number(chi.URLParam(r, "id"))
		if !ok {
			reply(w, 404, map[string]string{"error": "Product not found."})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 96*1024)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var in catalog.MakerInput
		if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF {
			reply(w, 400, map[string]string{"error": "Invalid variant request."})
			return
		}
		data, e := b.SaveMaker(r.Context(), authhttp.SessionToken(r.Context()), id, in)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, data)
	})
	r.Post("/", save)
	r.Put("/{id}", save)
	return r
}
func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func failure(w http.ResponseWriter, e error) {
	var v *catalog.ValidationError
	switch {
	case errors.As(e, &v):
		reply(w, 422, map[string]any{"error": "Check the highlighted fields.", "fields": v.Fields})
	case errors.Is(e, catalog.ErrNotFound):
		reply(w, 404, map[string]string{"error": "Product not found."})
	case errors.Is(e, catalog.ErrConflict):
		reply(w, 409, map[string]string{"error": "Reload the saved products before trying again."})
	case errors.Is(e, auth.ErrCredential):
		reply(w, 401, map[string]string{"error": "Your session ended. Sign in again."})
	default:
		reply(w, 503, map[string]string{"error": "Products are temporarily unavailable."})
	}
}

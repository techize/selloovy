// Package shophttp exposes settings behind the owner-session protection boundary.
package shophttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/shop"
)

type Backend interface {
	Read(context.Context, string) (shop.Settings, error)
	Save(context.Context, string, shop.Input) (shop.Settings, error)
}

func New(backend Backend) http.Handler {
	r := chi.NewRouter()
	r.Get("/shop", func(w http.ResponseWriter, r *http.Request) {
		data, err := backend.Read(r.Context(), authhttp.SessionToken(r.Context()))
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, data)
	})
	r.Put("/shop", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var input shop.Input
		if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF {
			reply(w, 400, map[string]string{"error": "Invalid shop settings request."})
			return
		}
		data, err := backend.Save(r.Context(), authhttp.SessionToken(r.Context()), input)
		if err != nil {
			failure(w, err)
			return
		}
		reply(w, 200, data)
	})
	return r
}
func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func failure(w http.ResponseWriter, err error) {
	var validation *shop.ValidationError
	switch {
	case errors.As(err, &validation):
		reply(w, 422, map[string]any{"error": "Check the highlighted fields.", "fields": validation.Fields})
	case errors.Is(err, shop.ErrConflict):
		reply(w, 409, map[string]string{"error": "Your shop changed in another tab. Reload the saved details before trying again."})
	case errors.Is(err, auth.ErrCredential):
		reply(w, 401, map[string]string{"error": "Your session ended. Sign in again."})
	default:
		reply(w, 503, map[string]string{"error": "Shop settings are temporarily unavailable."})
	}
}

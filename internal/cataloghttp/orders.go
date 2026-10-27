package cataloghttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/catalog"
)

type OrdersBackend interface {
	ListOrders(context.Context, string, string) (catalog.OrdersPage, error)
	ListAttentionOrders(context.Context, string, string) (catalog.OrdersPage, error)
	ListNotificationAttentionOrders(context.Context, string, string) (catalog.OrdersPage, error)
	ReadOrder(context.Context, string, string) (catalog.OrderDetail, error)
	SaveDispatch(context.Context, string, string, catalog.DispatchInput) (catalog.OrderDetail, error)
}
type OrderReconciler interface {
	ReconcileOwned(context.Context, string, string, string) (catalog.PaymentState, error)
}

func orderFailure(w http.ResponseWriter, e error) {
	var v *catalog.ValidationError
	switch {
	case errors.As(e, &v):
		reply(w, 422, map[string]any{"error": "Check the highlighted fields.", "fields": v.Fields})
	case errors.Is(e, catalog.ErrNotFound):
		reply(w, 404, map[string]string{"error": "Order or checkout not found."})
	case errors.Is(e, catalog.ErrConflict):
		reply(w, 409, map[string]string{"error": "The order changed or Square could not confirm the matching payment. Reload before trying again."})
	case errors.Is(e, auth.ErrCredential):
		reply(w, 401, map[string]string{"error": "Your session ended. Sign in again."})
	default:
		reply(w, 503, map[string]string{"error": "Orders are temporarily unavailable."})
	}
}
func orderDecode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		reply(w, 400, map[string]string{"error": "Invalid order request."})
		return false
	}
	return true
}

// Mount behind the authenticated exact-origin owner boundary.
func NewOrders(b OrdersBackend, p OrderReconciler) http.Handler {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {

		q := r.URL.Query()
		for name, values := range q {
			if (name != "after" && name != "attention") || len(values) != 1 || (name == "attention" && values[0] != "1" && values[0] != "notifications") {
				reply(w, 400, map[string]string{"error": "Invalid order list request."})
				return
			}
		}
		var page catalog.OrdersPage
		var e error
		if q.Get("attention") == "1" {
			page, e = b.ListAttentionOrders(r.Context(), authhttp.SessionToken(r.Context()), q.Get("after"))
		} else if q.Get("attention") == "notifications" {
			page, e = b.ListNotificationAttentionOrders(r.Context(), authhttp.SessionToken(r.Context()), q.Get("after"))
		} else {
			page, e = b.ListOrders(r.Context(), authhttp.SessionToken(r.Context()), q.Get("after"))
		}
		if e != nil {
			orderFailure(w, e)
			return
		}
		reply(w, 200, page)
	})
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			reply(w, 400, map[string]string{"error": "Invalid order request."})
			return
		}
		order, e := b.ReadOrder(r.Context(), authhttp.SessionToken(r.Context()), chi.URLParam(r, "id"))
		if e != nil {
			orderFailure(w, e)
			return
		}
		reply(w, 200, order)
	})
	r.Put("/{id}/dispatch", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			reply(w, 400, map[string]string{"error": "Invalid dispatch request."})
			return
		}
		var in catalog.DispatchInput
		if !orderDecode(w, r, &in) {
			return
		}
		order, e := b.SaveDispatch(r.Context(), authhttp.SessionToken(r.Context()), chi.URLParam(r, "id"), in)
		if e != nil {
			orderFailure(w, e)
			return
		}
		reply(w, 200, order)
	})
	r.Post("/{id}/reconcile", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			reply(w, 400, map[string]string{"error": "Invalid reconciliation request."})
			return
		}
		if p == nil {
			reply(w, 503, map[string]string{"error": "Square sandbox reconciliation is not configured."})
			return
		}
		var in struct {
			ProviderID string `json:"providerId"`
		}
		if !orderDecode(w, r, &in) {
			return
		}
		state, e := p.ReconcileOwned(r.Context(), authhttp.SessionToken(r.Context()), chi.URLParam(r, "id"), in.ProviderID)
		if e != nil {
			orderFailure(w, e)
			return
		}
		reply(w, 200, map[string]string{"id": state.ID, "state": state.State})
	})
	return r
}

package storefronthttp

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/delivery"
)

type CheckoutBackend interface {
	ReadCheckout(context.Context, string, string) (catalog.Checkout, error)
	SaveCheckout(context.Context, string, string, catalog.CheckoutChange) (catalog.Checkout, error)
}
type checkoutPage struct {
	Key, CSRF, Error string
	Fields           map[string]string
	Checkout         catalog.Checkout
	Customer         catalog.Customer
	Summary          catalog.CheckoutSnapshot
	Estimate         delivery.Estimate
}

func renderCheckout(w http.ResponseWriter, r *http.Request, p checkoutPage, status int) {
	var body bytes.Buffer
	if views.ExecuteTemplate(&body, "checkout.html", p) != nil {
		failure(w, r, catalog.ErrStorage)
		return
	}
	headers(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}
func checkoutRoutes(router chi.Router, b CheckoutBackend, origin string, calendar *delivery.Calendar) {
	load := func(w http.ResponseWriter, r *http.Request, key, token string, customer *catalog.Customer, message string, fields map[string]string, status int) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		c, e := b.ReadCheckout(ctx, key, token)
		if e != nil {
			failure(w, r, e)
			return
		}
		p := checkoutPage{Key: key, CSRF: basketCSRF(token), Checkout: c, Customer: c.Customer, Error: message, Fields: fields}
		if customer != nil {
			p.Customer = *customer
		}
		if p.Customer.Country == "" {
			p.Customer.Country = "GB"
		}
		p.Summary = c.Current
		if c.PaymentPending && c.Saved != nil {
			p.Summary = *c.Saved
		}
		if c.Ready && !c.PaymentPending {
			p.Estimate = calendar.Estimate(time.Now(), c.Current.DispatchDaysMin, c.Current.DispatchDaysMax, c.Current.Shipping)
		}
		renderCheckout(w, r, p, status)
	}
	router.Get("/{key}/checkout", func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		if !keyPattern.MatchString(key) || r.URL.RawQuery != "" {
			failure(w, r, catalog.ErrNotFound)
			return
		}
		token, e := basketToken(w, r, key, origin, false)
		if e != nil {
			failure(w, r, e)
			return
		}
		load(w, r, key, token, nil, "", nil, 200)
	})
	router.Post("/{key}/checkout", func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		if !keyPattern.MatchString(key) || r.URL.RawQuery != "" {
			failure(w, r, catalog.ErrNotFound)
			return
		}
		token, e := basketToken(w, r, key, origin, false)
		if e != nil || r.Header.Get("Origin") != origin || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") {
			headers(w)
			http.Error(w, "Open this shop before saving checkout.", 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.ParseForm() != nil {
			headers(w)
			http.Error(w, "Invalid checkout form.", 400)
			return
		}
		allowed := map[string]bool{"csrf": true, "revision": true, "quote": true, "email": true, "name": true, "address1": true, "address2": true, "town": true, "postcode": true, "country": true}
		for name, values := range r.PostForm {
			if !allowed[name] || len(values) != 1 {
				headers(w)
				http.Error(w, "Invalid checkout field.", 400)
				return
			}
		}
		if subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(basketCSRF(token))) != 1 {
			headers(w)
			http.Error(w, "Reload checkout before saving.", 403)
			return
		}
		rev, e := strconv.ParseInt(r.PostForm.Get("revision"), 10, 64)
		if e != nil || rev < 0 {
			headers(w)
			http.Error(w, "Invalid checkout revision.", 400)
			return
		}
		customer := catalog.Customer{Email: r.PostForm.Get("email"), Name: r.PostForm.Get("name"), Address1: r.PostForm.Get("address1"), Address2: r.PostForm.Get("address2"), Town: r.PostForm.Get("town"), Postcode: r.PostForm.Get("postcode"), Country: r.PostForm.Get("country")}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		_, e = b.SaveCheckout(ctx, key, token, catalog.CheckoutChange{Revision: rev, QuoteHash: r.PostForm.Get("quote"), Customer: customer})
		if e != nil {
			var v *catalog.ValidationError
			switch {
			case errors.As(e, &v):
				message := "Check the details below."
				if v.Fields["basket"] != "" {
					message = v.Fields["basket"]
				}
				load(w, r, key, token, &customer, message, v.Fields, 422)
			case errors.Is(e, catalog.ErrConflict):
				load(w, r, key, token, &customer, "Checkout or your basket changed. Review the current total and details, then save again.", nil, 409)
			default:
				basketError(w, r, e, key)
			}
			return
		}
		headers(w)
		refreshBasketCookie(w, key, origin, token)
		http.Redirect(w, r, "/shop/"+key+"/checkout", http.StatusSeeOther)
	})
}

package storefronthttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/delivery"
)

type BasketBackend interface {
	ReadBasket(context.Context, string, string) (catalog.Basket, error)
	ChangeBasket(context.Context, string, string, catalog.BasketChange) (catalog.Basket, error)
}
type basketPage struct {
	Key, CSRF string
	Basket    catalog.Basket
	Estimate  delivery.Estimate
}

func basketToken(w http.ResponseWriter, r *http.Request, key, origin string, create bool) (string, error) {
	u, e := url.Parse(origin)
	if e != nil || u.Host == "" || r.Host != u.Host {
		return "", catalog.ErrNotFound
	}
	name := "selloovy_basket"
	if u.Scheme == "https" {
		name = "__Secure-selloovy_basket"
	}
	var token string
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == name {
			token = c.Value
			count++
		}
	}
	if count > 1 {
		return "", catalog.ErrNotFound
	}
	if token != "" {
		b, e := hex.DecodeString(token)
		if e != nil || len(b) != 32 || len(token) != 64 {
			return "", catalog.ErrNotFound
		}
		return token, nil
	}
	if !create {
		return "", catalog.ErrNotFound
	}
	var b [32]byte
	if _, e = rand.Read(b[:]); e != nil {
		return "", catalog.ErrStorage
	}
	token = hex.EncodeToString(b[:])
	http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: "/shop/" + key, Secure: u.Scheme == "https", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 7 * 24 * 60 * 60, Expires: time.Now().Add(7 * 24 * time.Hour)})
	return token, nil
}
func basketCSRF(token string) string {
	d := sha256.Sum256([]byte("selloovy-basket-form:" + token))
	return hex.EncodeToString(d[:])
}
func basketError(w http.ResponseWriter, r *http.Request, e error, key string) {
	headers(w)
	status := http.StatusServiceUnavailable
	message := "Your basket could not be confirmed. Open it again before retrying."
	var validation *catalog.ValidationError
	if errors.As(e, &validation) {
		status = 422
		message = validation.Fields["basket"]
	}
	if errors.Is(e, catalog.ErrConflict) {
		status = 409
		message = "Your basket or delivery settings changed, or this form has already been submitted. Reload the product or basket before trying again."
	}
	if errors.Is(e, catalog.ErrNotFound) {
		status = 404
		message = "That basket or product option is unavailable."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	// Both strings are escaped; personalisation and bearer tokens never enter URLs.
	fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>Basket update</title><link rel=\"stylesheet\" href=\"/shop/assets/storefront.css\"></head><body><main><h1>Basket update</h1><p role=\"alert\">%s</p><p><a href=\"/shop/%s/basket\">Open your basket</a></p><p>Return to the product to correct your selection or certificate name.</p></main></body></html>", templateEscape(message), templateEscape(key))
}
func templateEscape(s string) string { return template.HTMLEscapeString(s) }
func basketRoutes(router chi.Router, b BasketBackend, origin string, calendar *delivery.Calendar) {
	router.Get("/{key}/basket", func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		if !keyPattern.MatchString(key) || r.URL.RawQuery != "" {
			failure(w, r, catalog.ErrNotFound)
			return
		}
		token, e := basketToken(w, r, key, origin, true)
		if e != nil {
			failure(w, r, e)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		basket, e := b.ReadBasket(ctx, key, token)
		if e != nil {
			failure(w, r, e)
			return
		}
		var body bytes.Buffer
		estimate := delivery.Estimate{}
		if basket.Complete && basket.ShippingSelected && len(basket.Lines) > 0 {
			estimate = calendar.Estimate(time.Now(), basket.DispatchDaysMin, basket.DispatchDaysMax, basket.SelectedShipping)
		}
		if views.ExecuteTemplate(&body, "basket.html", basketPage{Key: key, CSRF: basketCSRF(token), Basket: basket, Estimate: estimate}) != nil {
			failure(w, r, catalog.ErrStorage)
			return
		}
		headers(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(body.Bytes())
	})
	router.Post("/{key}/basket", func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "key")
		if !keyPattern.MatchString(key) || r.URL.RawQuery != "" {
			failure(w, r, catalog.ErrNotFound)
			return
		}
		token, e := basketToken(w, r, key, origin, false)
		if e != nil || r.Header.Get("Origin") != origin || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") {
			headers(w)
			http.Error(w, "Open this shop before updating your basket.", 403)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.ParseForm() != nil {
			headers(w)
			http.Error(w, "Invalid basket form.", 400)
			return
		}
		allowed := map[string]bool{"csrf": true, "revision": true, "product": true, "variant": true, "quantity": true, "ownerName": true, "line": true, "service": true, "shippingRevision": true}
		for k, v := range r.PostForm {
			if !allowed[k] || len(v) != 1 {
				headers(w)
				http.Error(w, "Invalid basket field.", 400)
				return
			}
		}
		if subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(basketCSRF(token))) != 1 {
			headers(w)
			http.Error(w, "Reload the shop before updating your basket.", 403)
			return
		}

		rev, e1 := strconv.ParseInt(r.PostForm.Get("revision"), 10, 64)
		if e1 != nil {
			headers(w)
			http.Error(w, "Invalid basket revision.", 400)
			return
		}
		change := catalog.BasketChange{Revision: rev}
		if r.PostForm.Has("service") {
			for _, field := range []string{"quantity", "ownerName", "line", "product", "variant"} {
				if r.PostForm.Has(field) {
					headers(w)
					http.Error(w, "Invalid shipping form.", 400)
					return
				}
			}
			change.ServiceID = r.PostForm.Get("service")
			change.ShippingRevision, e1 = strconv.ParseInt(r.PostForm.Get("shippingRevision"), 10, 64)
			if e1 != nil || !delivery.IDPattern.MatchString(change.ServiceID) {
				headers(w)
				http.Error(w, "Choose a valid shipping service.", 400)
				return
			}
		} else {
			if r.PostForm.Has("shippingRevision") {
				headers(w)
				http.Error(w, "Invalid basket form.", 400)
				return
			}
			change.Quantity, e1 = strconv.ParseInt(r.PostForm.Get("quantity"), 10, 64)
			change.OwnerName = r.PostForm.Get("ownerName")
			change.LineID = r.PostForm.Get("line")
			if e1 != nil {
				headers(w)
				http.Error(w, "Invalid quantity.", 400)
				return
			}
			if change.LineID == "" {
				var e2 error
				change.ProductID, e1 = strconv.ParseInt(r.PostForm.Get("product"), 10, 64)
				change.VariantID, e2 = strconv.ParseInt(r.PostForm.Get("variant"), 10, 64)
				if e1 != nil || e2 != nil || change.ProductID < 1 || change.VariantID < 1 {
					headers(w)
					http.Error(w, "Invalid product option.", 400)
					return
				}
			} else if !keyPattern.MatchString(change.LineID) || r.PostForm.Has("product") || r.PostForm.Has("variant") {
				headers(w)
				http.Error(w, "Invalid basket line.", 400)
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if _, e = b.ChangeBasket(ctx, key, token, change); e != nil {
			basketError(w, r, e, key)
			return
		}
		headers(w)
		http.Redirect(w, r, "/shop/"+key+"/basket", http.StatusSeeOther)
	})
}

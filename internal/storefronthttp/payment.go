package storefronthttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/delivery"
)

type PreparationBackend interface {
	PreparePayment(context.Context, string, string, int64) (catalog.PaymentPreparation, error)
	ReadPreparedPayment(context.Context, string, string, string) (catalog.PaymentPreparation, error)
	CancelPaymentPreparation(context.Context, string, string, string) error
}
type PaymentBackend interface {
	Submit(context.Context, string, string, int64, string, string, string) (catalog.PaymentState, error)
	GuestView(context.Context, string, string, string) (catalog.PaymentView, error)
}
type SandboxCard struct{ AppID, LocationID string }

func NewWithPayments(b Backend, origin string, calendar *delivery.Calendar, p PaymentBackend, card SandboxCard, pages ...PagesBackend) (http.Handler, error) {
	u, e := url.Parse(origin)
	_, preparations := b.(PreparationBackend)
	_, baskets := b.(BasketBackend)
	if e != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || !(u.Scheme == "https" || (u.Scheme == "http" && u.Hostname() == "localhost")) || !preparations || !baskets || p == nil || !regexp.MustCompile(`^sandbox-[A-Za-z0-9_-]{1,100}$`).MatchString(card.AppID) || !regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`).MatchString(card.LocationID) {
		return nil, errors.New("sandbox checkout configuration unavailable")
	}
	return newWithCalendar(b, origin, calendar, p, card, pages...), nil
}

type paymentPage struct {
	Key, ID, CSRF, Amount string
	Card                  SandboxCard
	Preparation           *catalog.PaymentPreparation
	Result                *catalog.PaymentView
	Estimate              delivery.Estimate
	Delayed               bool
	Dispatched            bool
}

func paymentHeaders(w http.ResponseWriter) {
	headers(w)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' https://sandbox.web.squarecdn.com; style-src 'self' 'unsafe-inline' https://sandbox.web.squarecdn.com; frame-src https://sandbox.web.squarecdn.com https://api.squareupsandbox.com; connect-src 'self' https://sandbox.web.squarecdn.com https://pci-connect.squareupsandbox.com https://o160250.ingest.sentry.io; font-src https://square-fonts-production-f.squarecdn.com https://d1g145x70srn7h.cloudfront.net; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none'; form-action 'self' https://api.squareupsandbox.com")
}
func receiptToken(r *http.Request, origin string) string {
	u, _ := url.Parse(origin)
	name := "selloovy_receipt"
	if u.Scheme == "https" {
		name = "__Secure-selloovy_receipt"
	}
	cookies := r.CookiesNamed(name)
	if len(cookies) != 1 {
		return ""
	}
	return cookies[0].Value
}
func receiptCookie(w http.ResponseWriter, key, id, origin, token string) {
	u, _ := url.Parse(origin)
	name := "selloovy_receipt"
	if u.Scheme == "https" {
		name = "__Secure-selloovy_receipt"
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: "/shop/" + key + "/payment/" + id, Secure: u.Scheme == "https", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 60 * 60, Expires: time.Now().Add(30 * 24 * time.Hour)})
}
func paymentReply(w http.ResponseWriter, status int, message string) {
	headers(w)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
func paymentRoutes(router chi.Router, b PreparationBackend, p PaymentBackend, origin string, calendar *delivery.Calendar, card SandboxCard) {
	guard := func(w http.ResponseWriter, r *http.Request) (string, string, string, bool) {
		key, id := chi.URLParam(r, "key"), chi.URLParam(r, "id")
		if !keyPattern.MatchString(key) || (id != "" && !keyPattern.MatchString(id)) || r.URL.RawQuery != "" {
			paymentReply(w, 404, "Payment unavailable.")
			return "", "", "", false
		}
		token, e := basketToken(w, r, key, origin, false)
		u, _ := url.Parse(origin)
		if r.Host != u.Host {
			paymentReply(w, 404, "Payment unavailable.")
			return "", "", "", false
		}
		if id != "" {
			if receipt := receiptToken(r, origin); receipt != "" {
				token = receipt
				e = nil
			}
		}
		if e != nil {
			paymentReply(w, 404, "Open your checkout in this browser.")
			return "", "", "", false
		}
		if r.Method != "GET" && (r.Header.Get("Origin") != origin || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin")) {
			paymentReply(w, 403, "Request origin is not allowed.")
			return "", "", "", false
		}
		return key, id, token, true
	}
	form := func(w http.ResponseWriter, r *http.Request, token string, fields ...string) bool {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.ParseForm() != nil {
			paymentReply(w, 400, "Invalid payment form.")
			return false
		}
		allowed := map[string]bool{"csrf": true}
		for _, name := range fields {
			allowed[name] = true
		}
		for name, values := range r.PostForm {
			if !allowed[name] || len(values) != 1 {
				paymentReply(w, 400, "Invalid payment field.")
				return false
			}
		}
		if subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(basketCSRF(token))) != 1 {
			paymentReply(w, 403, "Reload checkout before continuing.")
			return false
		}
		return true
	}
	router.Get("/assets/payment.js", func(w http.ResponseWriter, r *http.Request) {
		data, e := files.ReadFile("templates/payment.js")
		if e != nil {
			failure(w, r, catalog.ErrStorage)
			return
		}
		headers(w)
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(data)
	})
	router.Post("/{key}/checkout/payment", func(w http.ResponseWriter, r *http.Request) {
		key, _, token, ok := guard(w, r)
		if !ok || !form(w, r, token, "revision") {
			return
		}
		revision, e := strconv.ParseInt(r.PostForm.Get("revision"), 10, 64)
		if e != nil || revision < 1 {
			paymentReply(w, 400, "Save checkout before continuing.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		prep, e := b.PreparePayment(ctx, key, token, revision)
		if e != nil {
			basketError(w, r, e, key)
			return
		}
		headers(w)
		http.Redirect(w, r, "/shop/"+key+"/payment/"+prep.ID, 303)
	})
	router.Get("/{key}/payment/{id}", func(w http.ResponseWriter, r *http.Request) {
		key, id, token, ok := guard(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		page := paymentPage{Key: key, ID: id, CSRF: basketCSRF(token), Card: card}
		result, e := p.GuestView(ctx, key, token, id)
		if e == nil {
			page.Result = &result
			page.Delayed = time.Since(result.CreatedAt) > 5*time.Minute
			if result.State == "paid" {
				page.Estimate = calendar.Estimate(result.PaidAt, result.Snapshot.DispatchDaysMin, result.Snapshot.DispatchDaysMax, result.Snapshot.Shipping)
				if result.Fulfilment != nil && result.Fulfilment.State == "dispatched" && result.Fulfilment.DispatchedAt != nil {
					page.Dispatched = true
					page.Estimate = calendar.FromDispatch(*result.Fulfilment.DispatchedAt, result.Snapshot.Shipping)
				}
				receiptCookie(w, key, id, origin, token)
			}
		} else if errors.Is(e, catalog.ErrNotFound) {
			prep, e := b.ReadPreparedPayment(ctx, key, token, id)
			if e != nil {
				paymentReply(w, 404, "This payment preparation has expired or changed. Return to checkout to review it.")
				return
			}
			page.Preparation = &prep
			page.Amount = strconv.FormatInt(prep.Snapshot.TotalPence/100, 10) + "." + fmtPence(prep.Snapshot.TotalPence%100)
		} else {
			paymentReply(w, 503, "Payment status temporarily unavailable. Do not start another payment.")
			return
		}
		var body bytes.Buffer
		if views.ExecuteTemplate(&body, "payment.html", page) != nil {
			failure(w, r, catalog.ErrStorage)
			return
		}
		paymentHeaders(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(body.Bytes())
	})
	router.Post("/{key}/payment/{id}", func(w http.ResponseWriter, r *http.Request) {
		key, id, token, ok := guard(w, r)
		if !ok {
			return
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-Selloovy-Request") != "checkout-payment" {
			paymentReply(w, 415, "Use the payment form.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var in struct {
			CSRF     string `json:"csrf"`
			Revision int64  `json:"revision"`
			Source   string `json:"source"`
		}
		if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF || in.Revision < 1 {
			paymentReply(w, 400, "Invalid payment request.")
			return
		}
		if subtle.ConstantTimeCompare([]byte(in.CSRF), []byte(basketCSRF(token))) != 1 {
			paymentReply(w, 403, "Reload payment before continuing.")
			return
		}
		peer, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil {
			paymentReply(w, 400, "Connection identity unavailable.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		state, e := p.Submit(ctx, key, token, in.Revision, id, in.Source, peer)
		if e != nil {
			status := 503
			message := "Payment could not be confirmed. Reload its status; do not start another payment."
			if errors.Is(e, catalog.ErrNotFound) {
				status = 404
				message = "Payment unavailable."
			}
			if errors.Is(e, catalog.ErrConflict) {
				status = 409
				message = "Checkout changed or expired. Review checkout before trying again."
			}
			if errors.Is(e, catalog.ErrPaymentLimited) {
				status = 429
				message = "Too many payment attempts. Wait 15 minutes before trying again."
				w.Header().Set("Retry-After", "900")
			}
			paymentReply(w, status, message)
			return
		}
		headers(w)
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": state.ID, "state": state.State})
	})
	router.Post("/{key}/payment/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		key, id, token, ok := guard(w, r)
		if !ok || !form(w, r, token) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if e := b.CancelPaymentPreparation(ctx, key, token, id); e != nil {
			basketError(w, r, e, key)
			return
		}
		headers(w)
		http.Redirect(w, r, "/shop/"+key+"/checkout", 303)
	})
	router.Post("/{key}/payment/{id}/new-basket", func(w http.ResponseWriter, r *http.Request) {
		key, id, token, ok := guard(w, r)
		if !ok || !form(w, r, token) {
			return
		}
		result, e := p.GuestView(r.Context(), key, token, id)
		if e != nil || result.State != "paid" {
			paymentReply(w, 409, "Confirm payment before starting a new basket.")
			return
		}
		// An old receipt must not replace a newer active shopping basket.
		current, currentErr := basketToken(w, r, key, origin, false)
		if currentErr == nil && current != token {
			headers(w)
			http.Redirect(w, r, "/shop/"+key, 303)
			return
		}
		var value [32]byte
		if _, e = rand.Read(value[:]); e != nil {
			paymentReply(w, 503, "New basket unavailable.")
			return
		}
		headers(w)
		refreshBasketCookie(w, key, origin, hex.EncodeToString(value[:]))
		http.Redirect(w, r, "/shop/"+key, 303)
	})
}
func fmtPence(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

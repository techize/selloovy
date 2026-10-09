// Package authhttp serves cookie-authenticated owner sign-in and MFA setup.
package authhttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"
	"github.com/techize/selloovy/internal/auth"
)

type Backend interface {
	AllowAttempt(context.Context, string, string, int32) error
	Login(context.Context, string, string) (auth.LoginResult, error)
	SessionState(context.Context, string) (auth.SessionState, error)
	PrepareMFA(context.Context, string, string) (auth.MFASecret, error)
	ConfirmSessionEnrollment(context.Context, string, string) ([]auth.RecoveryCode, error)
	FinishLogin(context.Context, string, string, bool) (auth.Token, error)
	SessionOwner(context.Context, string) (int64, error)
	Logout(context.Context, string) error
	CancelChallenge(context.Context, string) error
}
type Handler struct {
	router       http.Handler
	backend      Backend
	origin, host string
	secure       bool
	prefix       string
}

func New(backend Backend, origin string) (*Handler, error) {
	u, err := url.Parse(origin)
	if backend == nil || err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid authentication HTTP configuration")
	}
	secure := u.Scheme == "https"
	if !secure {
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return nil, errors.New("authentication HTTP requires HTTPS")
		}
	}
	h := Handler{backend: backend, origin: origin, host: u.Host, secure: secure, prefix: "selloovy_"}
	if secure {
		h.prefix = "__Host-selloovy_"
	}
	r := chi.NewRouter()
	r.Use(h.guard)
	r.Get("/status", h.status)
	r.With(h.requireOwner).Get("/workspace", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"role": "owner"}) })
	r.Post("/login", h.login)
	r.Post("/verify", h.verify)
	r.Post("/logout", h.logout)
	r.Post("/mfa/start", h.startEnrollment)
	r.Post("/mfa/confirm", h.confirmEnrollment)
	h.router = r
	return &h, nil
}
func reply(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func failure(w http.ResponseWriter, err error) {
	status := http.StatusUnauthorized
	message := "Sign-in could not be completed. Check your details and try again."
	if errors.Is(err, auth.ErrRateLimited) || errors.Is(err, auth.ErrBusy) {
		status = 429
		message = "Too many attempts. Please wait before trying again."
		w.Header().Set("Retry-After", "900")
	}
	if errors.Is(err, auth.ErrStorage) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		status = 503
		message = "Sign-in is temporarily unavailable."
	}
	reply(w, status, map[string]string{"error": message})
}

var photoUploadPath = regexp.MustCompile("^/api/admin/products/[1-9][0-9]*/photo$")

func (h Handler) guard(next http.Handler) http.Handler { return h.guardUploads(next, false) }
func (h Handler) guardUploads(next http.Handler, allowUploads bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Host != h.host {
			reply(w, 403, map[string]string{"error": "Request origin is not allowed."})
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			// Require exact configured Origin and a browser custom-header request.
			// No CORS permission is emitted, including for preflight requests.
			if r.Header.Get("Origin") != h.origin || r.Header.Get("X-Selloovy-Request") != "owner-auth" || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") {
				reply(w, 403, map[string]string{"error": "Request origin is not allowed."})
				return
			}
			contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || (contentType != "application/json" && !(allowUploads && contentType == "multipart/form-data" && r.Method == "PUT" && photoUploadPath.MatchString(r.URL.Path))) {
				reply(w, 415, map[string]string{"error": "Use the supported request format."})
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		reply(w, 400, map[string]string{"error": "Invalid sign-in request."})
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		reply(w, 400, map[string]string{"error": "Invalid sign-in request."})
		return false
	}
	return true
}
func (h Handler) cookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: h.prefix + name, Value: value, Path: "/", MaxAge: age, HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteStrictMode})
}
func (h Handler) value(r *http.Request, name string) string {
	cookies := r.CookiesNamed(h.prefix + name)
	if len(cookies) != 1 {
		return ""
	}
	return cookies[0].Value
}
func (h Handler) ipLimit(r *http.Request, scope string) error {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(host) == nil {
		return auth.ErrCredential
	}
	return h.backend.AllowAttempt(r.Context(), "http-"+scope, host, 60)
}
func (h Handler) status(w http.ResponseWriter, r *http.Request) {
	token := h.value(r, "session")
	if token == "" {
		reply(w, 200, map[string]bool{"authenticated": false})
		return
	}
	state, err := h.backend.SessionState(r.Context(), token)
	if errors.Is(err, auth.ErrCredential) {
		h.cookie(w, "session", "", -1)
		reply(w, 200, map[string]bool{"authenticated": false})
		return
	}
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, map[string]bool{"authenticated": true, "mfaEnabled": state.MFAEnabled})
}
func (h Handler) login(w http.ResponseWriter, r *http.Request) {
	if err := h.ipLimit(r, "login"); err != nil {
		failure(w, err)
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	result, err := h.backend.Login(r.Context(), body.Email, body.Password)
	body.Password = ""
	if err != nil {
		failure(w, err)
		return
	}
	// A new password-proven challenge revokes an old browser session before rotation.
	if old := h.value(r, "session"); old != "" {
		if err = h.backend.Logout(r.Context(), old); err != nil {
			failure(w, err)
			return
		}
	}
	h.cookie(w, "session", "", -1)
	if result.MFARequired {
		h.cookie(w, "challenge", result.Token.Reveal(), 300)
		reply(w, 200, map[string]string{"step": "mfa"})
	} else {
		h.cookie(w, "challenge", "", -1)
		h.cookie(w, "session", result.Token.Reveal(), 8*60*60)
		reply(w, 200, map[string]bool{"authenticated": true, "mfaEnabled": false})
	}
}
func (h Handler) verify(w http.ResponseWriter, r *http.Request) {
	if err := h.ipLimit(r, "verify"); err != nil {
		failure(w, err)
		return
	}
	var body struct {
		Code     string `json:"code"`
		Recovery bool   `json:"recovery"`
	}
	if !decode(w, r, &body) {
		return
	}
	challenge := h.value(r, "challenge")
	if challenge == "" {
		failure(w, auth.ErrCredential)
		return
	}
	token, err := h.backend.FinishLogin(r.Context(), challenge, strings.TrimSpace(body.Code), body.Recovery)
	body.Code = ""
	if err != nil {
		failure(w, err)
		return
	}
	h.cookie(w, "challenge", "", -1)
	h.cookie(w, "session", token.Reveal(), 8*60*60)
	reply(w, 200, map[string]bool{"authenticated": true, "mfaEnabled": true})
}
func (h Handler) logout(w http.ResponseWriter, r *http.Request) {
	var body struct{}
	if !decode(w, r, &body) {
		return
	}
	if token := h.value(r, "session"); token != "" {
		if err := h.backend.Logout(r.Context(), token); err != nil {
			failure(w, err)
			return
		}
	}
	if token := h.value(r, "challenge"); token != "" {
		if err := h.backend.CancelChallenge(r.Context(), token); err != nil {
			failure(w, err)
			return
		}
	}
	h.cookie(w, "session", "", -1)
	h.cookie(w, "challenge", "", -1)
	reply(w, 200, map[string]bool{"authenticated": false})
}

// requireOwner checks server-side session validity on each protected request.
// Browser UI state never grants permission to a merchant API.
func (h Handler) requireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := h.value(r, "session")
		if token == "" {
			failure(w, auth.ErrCredential)
			return
		}
		if _, err := h.backend.SessionOwner(r.Context(), token); err != nil {
			failure(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, sessionValue(token))))
	})
}

func (h Handler) startEnrollment(w http.ResponseWriter, r *http.Request) {
	if err := h.ipLimit(r, "enrollment"); err != nil {
		failure(w, err)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	secret, err := h.backend.PrepareMFA(r.Context(), h.value(r, "session"), body.Password)
	body.Password = ""
	if err != nil {
		failure(w, err)
		return
	}
	key := secret.EnrollmentKey()
	uri := "otpauth://totp/Selloovy:Owner?" + url.Values{"secret": {key}, "issuer": {"Selloovy"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()
	png, err := qrcode.Encode(uri, qrcode.Medium, 320)
	if err != nil {
		failure(w, auth.ErrStorage)
		return
	}
	reply(w, 200, map[string]string{"key": key, "qr": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
}
func (h Handler) confirmEnrollment(w http.ResponseWriter, r *http.Request) {
	if err := h.ipLimit(r, "enrollment"); err != nil {
		failure(w, err)
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	codes, err := h.backend.ConfirmSessionEnrollment(r.Context(), h.value(r, "session"), strings.TrimSpace(body.Code))
	body.Code = ""
	if err != nil {
		failure(w, err)
		return
	}
	values := make([]string, len(codes))
	for i, code := range codes {
		values[i] = code.Reveal()
	}
	h.cookie(w, "session", "", -1)
	h.cookie(w, "challenge", "", -1)
	reply(w, 200, map[string]any{"mfaEnabled": true, "recoveryCodes": values, "signInAgain": true})
}

type sessionKey struct{}
type sessionValue string

func (sessionValue) String() string { return "[redacted owner session]" }
func (sessionValue) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[redacted owner session]")
}

// SessionToken returns a sensitive credential for owned SQL checks; never log it.
func SessionToken(ctx context.Context) string {
	token, _ := ctx.Value(sessionKey{}).(sessionValue)
	return string(token)
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.router.ServeHTTP(w, r) }

// Protect applies the same session, origin, JSON and request deadline rules to merchant APIs.
func (h *Handler) Protect(next http.Handler) http.Handler { return h.guard(h.requireOwner(next)) }

// ProtectAdmin permits multipart only at the exact product-photo PUT endpoint.
// The host, Origin, custom-header, session and deadline checks remain identical.
func (h *Handler) ProtectAdmin(next http.Handler) http.Handler {
	return h.guardUploads(h.requireOwner(next), true)
}

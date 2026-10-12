package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// LocalhostAlias redirects local document navigation to the SDK-compatible
// canonical hostname. API and mutation requests retain their exact-origin guards.
func LocalhostAlias(next http.Handler, origin string) http.Handler {
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "http" || u.Hostname() != "localhost" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, port, e := net.SplitHostPort(r.Host)
		ip := net.ParseIP(host)
		if e == nil && ip != nil && ip.IsLoopback() && port == u.Port() && (r.Method == "GET" || r.Method == "HEAD") && !strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "same-origin")
			http.Redirect(w, r, origin+r.URL.RequestURI(), http.StatusTemporaryRedirect)
			return
		}
		next.ServeHTTP(w, r)
	})
}

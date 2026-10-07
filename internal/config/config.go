// Package config validates the application's runtime settings.
package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr     string
	DatabaseURL  string
	AdminDir     string
	AuthKeyFile  string
	PublicOrigin string
}

// Load accepts a lookup function so tests do not modify the process environment.
func Load(lookup func(string) (string, bool)) (Config, error) {
	addr := "127.0.0.1:8080"
	if value, present := lookup("SELLOOVY_HTTP_ADDR"); present {
		addr = strings.TrimSpace(value)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" || net.ParseIP(host) == nil {
		return Config{}, errors.New("SELLOOVY_HTTP_ADDR must contain an IP address and port")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return Config{}, errors.New("SELLOOVY_HTTP_ADDR port must be between 1 and 65535")
	}
	cfg := Config{HTTPAddr: addr, AdminDir: "web/admin/dist"}
	if value, present := lookup("SELLOOVY_ADMIN_DIR"); present {
		cfg.AdminDir = strings.TrimSpace(value)
		if cfg.AdminDir == "" {
			return Config{}, errors.New("SELLOOVY_ADMIN_DIR must not be blank when set")
		}
	}
	if value, present := lookup("SELLOOVY_DATABASE_URL"); present {
		cfg.DatabaseURL = strings.TrimSpace(value)
		if cfg.DatabaseURL == "" {
			return Config{}, errors.New("SELLOOVY_DATABASE_URL must not be blank when set")
		}
	}
	key, keySet := lookup("SELLOOVY_AUTH_KEY_FILE")
	origin, originSet := lookup("SELLOOVY_PUBLIC_ORIGIN")
	if keySet || originSet {
		if !keySet || !originSet || strings.TrimSpace(key) == "" || cfg.DatabaseURL == "" {
			return Config{}, errors.New("authentication requires a key file, public origin and database")
		}
		u, e := url.Parse(origin)
		if e != nil || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return Config{}, errors.New("invalid authentication public origin")
		}
		if u.Scheme == "http" {
			originIP := net.ParseIP(u.Hostname())
			bindIP := net.ParseIP(host)
			if originIP == nil || !originIP.IsLoopback() || !bindIP.IsLoopback() {
				return Config{}, errors.New("authentication requires HTTPS outside explicit loopback development")
			}
		}
		cfg.AuthKeyFile = key
		cfg.PublicOrigin = origin
	}
	return cfg, nil
}

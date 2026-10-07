// Package config validates the application's runtime settings.
package config

import (
	"errors"
	"net"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr string
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
	return Config{HTTPAddr: addr}, nil
}

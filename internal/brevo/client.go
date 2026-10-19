// Package brevo owns the fixed transactional-email API boundary. It never retries
// a submission: an uncertain result needs durable reconciliation by its caller.
package brevo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const endpoint = "https://api.brevo.com/v3"
const maxResponse = 128 * 1024

var ErrConfig = errors.New("Brevo configuration unavailable; use an owner-only regular API key file and a sender address")
var ErrRequest = errors.New("Brevo email request invalid")
var ErrSender = errors.New("Brevo sender check unavailable; verify API access and the selected active sender")
var ErrIPBlocked = errors.New("Brevo blocked this connection's IP address; review the recent API attempt in Settings > Security > Authorized IPs")
var ErrAuthentication = errors.New("Brevo API access rejected; check the API key and account permissions")
var ErrSenderMissing = errors.New("selected Brevo sender is not registered; choose an existing sender in the account")
var ErrSenderInactive = errors.New("selected Brevo sender is inactive; complete its verification in Brevo")
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Mode string

const (
	Drop    Mode = "drop"
	Deliver Mode = "deliver"
)

type State string

const (
	Accepted  State = "accepted"
	Validated State = "validated"
	Rejected  State = "rejected"
	Unknown   State = "unknown"
)

type Request struct{ To, Name, Subject, HTML, Text, IdempotencyKey string }

func (Request) String() string               { return "Brevo email [private]" }
func (r Request) GoString() string           { return r.String() }
func (Request) MarshalJSON() ([]byte, error) { return []byte(`{"private":true}`), nil }

type Result struct {
	State     State
	MessageID string
}

func (r Result) String() string   { return "Brevo result " + string(r.State) }
func (r Result) GoString() string { return r.String() }
func (r Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		State State `json:"state"`
	}{r.State})
}

type Client struct {
	key, sender, name string
	mode              Mode
	http              *http.Client
}

func (*Client) String() string     { return "Brevo client [private]" }
func (c *Client) GoString() string { return c.String() }

func New(keyFile, sender, name string, mode Mode) (*Client, error) {
	if !address(sender) || !line(name, 160, true) || (mode != Drop && mode != Deliver) {
		return nil, ErrConfig
	}
	before, e := os.Lstat(keyFile)
	if e != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() < 1 || before.Size() > 4096 {
		return nil, ErrConfig
	}
	f, e := os.Open(keyFile)
	if e != nil {
		return nil, ErrConfig
	}
	defer f.Close()
	after, e := f.Stat()
	if e != nil || !os.SameFile(before, after) || after.Mode().Perm()&0077 != 0 {
		return nil, ErrConfig
	}
	b, e := io.ReadAll(io.LimitReader(f, 4097))
	defer clear(b)
	if e != nil || len(b) > 4096 {
		return nil, ErrConfig
	}
	key := strings.TrimSpace(string(b))
	if !ascii(key, 4096) {
		return nil, ErrConfig
	}
	return &Client{key: key, sender: sender, name: name, mode: mode, http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect disabled") }}}, nil
}
func ascii(s string, max int) bool {
	if len(s) < 1 || len(s) > max {
		return false
	}
	for _, b := range []byte(s) {
		if b < 33 || b > 126 {
			return false
		}
	}
	return true
}
func line(s string, max int, empty bool) bool {
	if !utf8.ValidString(s) || len(s) > max || (!empty && s == "") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func address(s string) bool {
	a, e := mail.ParseAddress(s)
	return e == nil && a.Address == s && ascii(s, 254) && strings.Count(s, "@") == 1 && strings.Contains(strings.SplitN(s, "@", 2)[1], ".")
}
func reserved(s string) bool {
	d := strings.ToLower(strings.SplitN(s, "@", 2)[1])
	for _, r := range []string{"example.com", "example.net", "example.org", "example", "invalid", "test", "localhost"} {
		if d == r || strings.HasSuffix(d, "."+r) {
			return true
		}
	}
	return false
}
func (c *Client) call(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, e := http.NewRequestWithContext(ctx, method, endpoint+path, reader)
	if e != nil {
		return 0, nil, ErrSender
	}
	req.Header.Set("api-key", c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, e := c.http.Do(req)
	if e != nil {
		return 0, nil, ErrSender
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if e != nil || len(b) > maxResponse {
		clear(b)
		return 0, nil, ErrSender
	}
	return res.StatusCode, b, nil
}

// VerifySender reads account configuration without creating a sender or sending.
// It confirms the selected sender is active, not domain authentication/delivery.
func (c *Client) VerifySender(ctx context.Context) error {
	status, b, e := c.call(ctx, http.MethodGet, "/senders", nil)
	defer clear(b)
	if e != nil {
		return ErrSender
	}
	if status != 200 {
		// Classify only provider authentication responses. Never surface raw
		// messages, which can contain account details, addresses or credentials.
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			var failure struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(b, &failure) == nil {
				message := strings.ToLower(failure.Message)
				mentionsIP := strings.Contains(message, "ip address") || strings.Contains(message, "ip ")
				for _, marker := range []string{"unrecogn", "unauthor", "not author", "not allowed", "whitelist", "allowlist", "authoriz"} {
					if mentionsIP && strings.Contains(message, marker) {
						return ErrIPBlocked
					}
				}
			}
			return ErrAuthentication
		}
		return ErrSender
	}
	var v struct {
		Senders []struct {
			Email  string `json:"email"`
			Active bool   `json:"active"`
		} `json:"senders"`
	}
	if json.Unmarshal(b, &v) != nil {
		return ErrSender
	}
	found := false
	for _, s := range v.Senders {
		if strings.EqualFold(s.Email, c.sender) {
			found = true
			if s.Active {
				return nil
			}
		}
	}
	if found {
		return ErrSenderInactive
	}
	return ErrSenderMissing
}

// Send makes one POST using the caller's persisted UUIDv4. Drop validates only;
// Accepted means API acceptance, never proof of inbox delivery. No automatic retry.
func (c *Client) Send(ctx context.Context, r Request) (Result, error) {
	if !address(r.To) || !line(r.Name, 160, true) || !line(r.Subject, 200, false) || !uuidPattern.MatchString(r.IdempotencyKey) || r.HTML == "" || r.Text == "" || !utf8.ValidString(r.HTML+r.Text) || len(r.HTML)+len(r.Text) > 256*1024 || (c.mode == Deliver && reserved(r.To)) {
		return Result{}, ErrRequest
	}
	type contact struct {
		Email string `json:"email"`
		Name  string `json:"name,omitempty"`
	}
	headers := map[string]string{"IdempotencyKey": r.IdempotencyKey}
	if c.mode == Drop {
		headers["X-Sib-Sandbox"] = "drop"
	}
	b, _ := json.Marshal(struct {
		Sender  contact           `json:"sender"`
		To      []contact         `json:"to"`
		Subject string            `json:"subject"`
		HTML    string            `json:"htmlContent"`
		Text    string            `json:"textContent"`
		Headers map[string]string `json:"headers"`
	}{contact{c.sender, c.name}, []contact{{r.To, r.Name}}, r.Subject, r.HTML, r.Text, headers})
	defer clear(b)
	status, data, e := c.call(ctx, http.MethodPost, "/smtp/email", b)
	defer clear(data)
	if e != nil {
		return Result{State: Unknown}, nil
	}
	var v struct {
		MessageID string `json:"messageId"`
		Code      string `json:"code"`
	}
	if json.Unmarshal(data, &v) != nil {
		return Result{State: Unknown}, nil
	}
	if status == 201 && v.Code == "" && ascii(v.MessageID, 512) {
		state := Accepted
		if c.mode == Drop {
			state = Validated
		}
		return Result{state, v.MessageID}, nil
	}
	// Duplicate keys may refer to an earlier accepted submission. Never call them
	// rejected or resubmit using a fresh key. 5xx/408 can also hide acceptance.
	if status >= 400 && status < 500 && status != 408 && v.Code != "" && v.Code != "duplicate_parameter" {
		return Result{State: Rejected}, nil
	}
	return Result{State: Unknown}, nil
}

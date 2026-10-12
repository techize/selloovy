// Package square implements the fixed sandbox card-processing boundary. It is
// connected only to explicitly configured sandbox checkout.
package square

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const apiVersion = "2026-09-16"
const sandbox = "https://connect.squareupsandbox.com"

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
var keyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var ErrConfig = errors.New("Square sandbox configuration unavailable")
var ErrRequest = errors.New("Square sandbox request invalid")

type State string

const (
	Paid     State = "paid"
	Declined State = "declined"
	Pending  State = "pending"
	Unknown  State = "unknown"
)

type Request struct {
	SourceToken, IdempotencyKey, Reference string
	AmountPence                            int64
}

func (Request) String() string               { return "Square request [private]" }
func (Request) GoString() string             { return "Square request [private]" }
func (Request) MarshalJSON() ([]byte, error) { return []byte(`{"private":true}`), nil }

type Result struct {
	State      State
	ProviderID string
	CapturedAt time.Time
}

func (r Result) String() string   { return "Square result " + string(r.State) }
func (r Result) GoString() string { return r.String() }
func (r Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		State State `json:"state"`
	}{r.State})
}

type Client struct {
	token, location string
	http            *http.Client
}

func (c *Client) Location() string { return c.location }

func (*Client) String() string   { return "Square sandbox client [private]" }
func (*Client) GoString() string { return "Square sandbox client [private]" }

func NewSandbox(tokenFile, location string) (*Client, error) {
	if !idPattern.MatchString(location) {
		return nil, ErrConfig
	}
	before, e := os.Lstat(tokenFile)
	if e != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() < 1 || before.Size() > 4096 {
		return nil, ErrConfig
	}
	f, e := os.Open(tokenFile)
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
	token := strings.TrimSpace(string(b))
	if !plain(token, 4096) {
		return nil, ErrConfig
	}
	return &Client{token: token, location: location, http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect disabled") }}}, nil
}
func plain(s string, max int) bool {
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
func valid(r Request) bool {
	return keyPattern.MatchString(r.IdempotencyKey) && keyPattern.MatchString(r.Reference) && strings.HasPrefix(r.SourceToken, "cnon:") && len(r.SourceToken) > 5 && plain(r.SourceToken, 4096) && r.AmountPence > 0 && r.AmountPence <= 1_000_000_000_000
}

// Charge makes exactly one request. Call only after persisting the same intent,
// request envelope and key; an Unknown result never permits a fresh-key retry.
func (c *Client) Charge(ctx context.Context, r Request) (Result, error) {
	if !valid(r) {
		return Result{}, ErrRequest
	}
	body, _ := json.Marshal(struct {
		Source    string `json:"source_id"`
		Key       string `json:"idempotency_key"`
		Location  string `json:"location_id"`
		Reference string `json:"reference_id"`
		Amount    struct {
			Amount   int64  `json:"amount"`
			Currency string `json:"currency"`
		} `json:"amount_money"`
		Complete bool `json:"autocomplete"`
		Customer struct {
			Initiated bool `json:"customer_initiated"`
			Keyed     bool `json:"seller_keyed_in"`
		} `json:"customer_details"`
	}{Source: r.SourceToken, Key: r.IdempotencyKey, Location: c.location, Reference: r.Reference, Amount: struct {
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}{r.AmountPence, "GBP"}, Complete: true, Customer: struct {
		Initiated bool `json:"customer_initiated"`
		Keyed     bool `json:"seller_keyed_in"`
	}{true, false}})
	defer clear(body)
	return c.call(ctx, http.MethodPost, "/v2/payments", body, r)
}

// Reconcile reads a known payment; observations must match the persisted intent.
func (c *Client) Reconcile(ctx context.Context, id string, r Request) (Result, error) {
	if !idPattern.MatchString(id) || !keyPattern.MatchString(r.Reference) || r.AmountPence < 1 {
		return Result{}, ErrRequest
	}
	return c.call(ctx, http.MethodGet, "/v2/payments/"+id, nil, r)
}
func (c *Client) call(ctx context.Context, method, path string, body []byte, expected Request) (Result, error) {
	req, e := http.NewRequestWithContext(ctx, method, sandbox+path, bytes.NewReader(body))
	if e != nil {
		return Result{State: Unknown}, nil
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Square-Version", apiVersion)
	req.Header.Set("Content-Type", "application/json")
	response, e := c.http.Do(req)
	if e != nil {
		return Result{State: Unknown}, nil
	}
	defer response.Body.Close()
	data, e := io.ReadAll(io.LimitReader(response.Body, 262145))
	defer clear(data)
	if e != nil || len(data) > 262144 {
		return Result{State: Unknown}, nil
	}
	var result struct {
		Payment struct {
			ID, Status string
			Location   string `json:"location_id"`
			Reference  string `json:"reference_id"`
			Source     string `json:"source_type"`
			Amount     struct {
				Amount   int64
				Currency string
			} `json:"amount_money"`
			Card struct {
				Status   string
				Timeline struct {
					CapturedAt string `json:"captured_at"`
				} `json:"card_payment_timeline"`
			} `json:"card_details"`
		} `json:"payment"`
		Errors []json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(data, &result) != nil {
		return Result{State: Unknown}, nil
	}
	p := result.Payment
	// Explicit card-method declines without a payment record are definitive for
	// this request. Idempotency/conflict/configuration errors remain uncertain.
	if method == http.MethodPost && (response.StatusCode == 400 || response.StatusCode == 402) && p.ID == "" && p.Status == "" && definiteCardDecline(result.Errors) {
		return Result{State: Declined}, nil
	}
	if !idPattern.MatchString(p.ID) || p.Location != c.location || p.Reference != expected.Reference || p.Amount.Amount != expected.AmountPence || p.Amount.Currency != "GBP" || p.Source != "CARD" {
		return Result{State: Unknown}, nil
	}
	if method == http.MethodGet && p.ID != strings.TrimPrefix(path, "/v2/payments/") {
		return Result{State: Unknown}, nil
	}
	out := Result{State: Unknown, ProviderID: p.ID}
	if response.StatusCode == 200 && len(result.Errors) == 0 {
		switch p.Status {
		case "COMPLETED":
			if p.Card.Status == "CAPTURED" {
				captured, err := time.Parse(time.RFC3339Nano, p.Card.Timeline.CapturedAt)
				if err == nil && !captured.IsZero() {
					out.State = Paid
					out.CapturedAt = captured.UTC()
				}
			}
		case "APPROVED", "PENDING":
			out.State = Pending
		case "FAILED", "CANCELED":
			if p.Card.Status == "FAILED" || p.Card.Status == "VOIDED" {
				out.State = Declined
			}
		}
	} else if (response.StatusCode == 400 || response.StatusCode == 402) && p.Status == "FAILED" && p.Card.Status == "FAILED" && len(result.Errors) > 0 {
		out.State = Declined
	}
	return out, nil
}

func definiteCardDecline(raw []json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	for _, data := range raw {
		var e struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(data, &e) != nil {
			return false
		}
		switch e.Code {
		case "CARD_DECLINED_VERIFICATION_REQUIRED", "CVV_FAILURE", "EXPIRATION_FAILURE", "ADDRESS_VERIFICATION_FAILURE", "CARD_NOT_SUPPORTED", "INVALID_CARD", "INVALID_CARD_DATA", "GENERIC_DECLINE", "INSUFFICIENT_FUNDS", "CARD_EXPIRED":
		default:
			return false
		}
	}
	return true
}

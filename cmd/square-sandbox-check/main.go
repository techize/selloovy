// square-sandbox-check performs a read-only account check, never a payment.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const endpoint = "https://connect.squareupsandbox.com/v2/locations"
const apiVersion = "2026-09-16"
const maxResponse = 128 * 1024

type location struct {
	ID           string   `json:"id"`
	Country      string   `json:"country"`
	Currency     string   `json:"currency"`
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities"`
}

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "This command accepts configuration through sandbox environment variables only.")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := check(ctx, os.Getenv("SELLOOVY_SQUARE_SANDBOX_ACCESS_TOKEN_FILE"), os.Getenv("SELLOOVY_SQUARE_SANDBOX_LOCATION_ID"), http.DefaultTransport); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Square sandbox access verified: one selected active UK/GBP card-processing location. Account access only; checkout submission remains disabled until financial integration is complete.")
}

func readToken(path string) ([]byte, error) {
	fail := errors.New("sandbox token requires a private regular file (owner access only); its contents are never printed")
	before, e := os.Lstat(path)
	if e != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() < 1 || before.Size() > 4096 {
		return nil, fail
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, fail
	}
	defer f.Close()
	after, e := f.Stat()
	if e != nil || !os.SameFile(before, after) || after.Mode().Perm()&0077 != 0 {
		return nil, fail
	}
	data, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || len(data) > 4096 {
		clear(data)
		return nil, fail
	}
	token := bytes.TrimSpace(data)
	if len(token) == 0 {
		clear(data)
		return nil, fail
	}
	for _, b := range token {
		if b < 33 || b > 126 {
			clear(data)
			return nil, fail
		}
	}
	// Return the full allocation so the caller can clear even trimmed bytes.
	return data, nil
}

func check(ctx context.Context, path, selected string, transport http.RoundTripper) error {
	data, e := readToken(path)
	if e != nil {
		return e
	}
	defer clear(data)
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if e != nil {
		return errors.New("sandbox request unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+string(bytes.TrimSpace(data)))
	req.Header.Set("Square-Version", apiVersion)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("sandbox redirects are disabled") }}
	response, e := client.Do(req)
	if e != nil {
		return errors.New("Square sandbox connection failed; check sandbox access and network availability")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Square sandbox account check failed (HTTP %d); response details withheld", response.StatusCode)
	}
	body, e := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if e != nil || len(body) > maxResponse {
		return errors.New("Square sandbox response unavailable or too large")
	}
	defer clear(body)
	var result struct {
		Locations []location        `json:"locations"`
		Errors    []json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(body, &result) != nil || len(result.Errors) > 0 {
		return errors.New("Square sandbox returned an invalid account response")
	}
	count := 0
	for _, l := range result.Locations {
		if l.ID == "" || l.Country != "GB" || l.Currency != "GBP" || l.Status != "ACTIVE" || (selected != "" && selected != l.ID) {
			continue
		}
		for _, capability := range l.Capabilities {
			if capability == "CREDIT_CARD_PROCESSING" {
				count++
				break
			}
		}
	}
	if count == 0 {
		return errors.New("no matching active UK/GBP sandbox card-processing location; verify the selected test account/location")
	}
	if count != 1 {
		return errors.New("multiple eligible sandbox locations; set SELLOOVY_SQUARE_SANDBOX_LOCATION_ID explicitly")
	}
	return nil
}

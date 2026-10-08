package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/techize/selloovy/internal/auth"
)

func TestSetupRefusesArgumentsAndNoninteractiveInput(t *testing.T) {
	oldArgs, oldInput := os.Args, os.Stdin
	t.Cleanup(func() { os.Args = oldArgs; os.Stdin = oldInput })
	input, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	os.Stdin = input
	for _, args := range [][]string{{"owner-setup"}, {"owner-setup", "sensitive argument must not be echoed"}} {
		os.Args = args
		err := run()
		if err == nil || strings.Contains(err.Error(), "sensitive argument") {
			t.Fatal("unsafe setup input accepted or disclosed")
		}
	}
}

type promptStore struct {
	contexts []context.Context
	fail     error
}

func (s *promptStore) record(ctx context.Context) error {
	s.contexts = append(s.contexts, ctx)
	if ctx.Err() != nil {
		return auth.ErrStorage
	}
	if _, ok := ctx.Deadline(); !ok {
		return auth.ErrStorage
	}
	return s.fail
}
func (s *promptStore) ResumeEnrollment(ctx context.Context, email, password string) (int64, auth.MFASecret, error) {
	if err := s.record(ctx); err != nil {
		return 0, auth.MFASecret{}, err
	}
	return 0, auth.MFASecret{}, auth.ErrCredential
}
func (s *promptStore) CreateFirstOwner(ctx context.Context, name, email, password string) (int64, auth.MFASecret, error) {
	err := s.record(ctx)
	return 1, auth.MFASecret{}, err
}
func (s *promptStore) ConfirmEnrollment(ctx context.Context, id int64, code string) ([]auth.RecoveryCode, error) {
	return nil, s.record(ctx)
}
func TestSlowInteractivePromptsDoNotExpireSetupOperations(t *testing.T) {
	store := &promptStore{}
	prompts := 0
	timeout := 100 * time.Millisecond
	read := func(prompt string) (string, error) {
		// The previous operation must release its context before waiting for input.
		if len(store.contexts) == 0 || store.contexts[len(store.contexts)-1].Err() != context.Canceled {
			t.Fatal("operation context retained during prompt")
		}
		time.Sleep(2 * timeout)
		prompts++
		if prompts == 1 {
			return "Synthetic maker", nil
		}
		if prompts == 2 {
			return "yes", nil
		}
		return "123456", nil
	}
	if err := completeSetup(store, read, io.Discard, "owner@example.com", "Synthetic fixture passphrase", timeout); err != nil {
		t.Fatalf("slow input prevented setup: %v", err)
	}
	if prompts != 3 || len(store.contexts) != 3 {
		t.Fatal("setup did not finish its expected operations")
	}
	for _, ctx := range store.contexts {
		if ctx.Err() != context.Canceled {
			t.Fatal("operation context not released")
		}
	}
}
func TestStorageFailureStopsSetupWithoutShowingCredentials(t *testing.T) {
	store := &promptStore{fail: auth.ErrStorage}
	var output bytes.Buffer
	read := func(string) (string, error) { t.Fatal("storage failure proceeded to enrollment"); return "", nil }
	err := completeSetup(store, read, &output, "owner@example.com", "Synthetic fixture passphrase", time.Second)
	if !errors.Is(err, auth.ErrStorage) || output.Len() != 0 || len(store.contexts) != 1 {
		t.Fatal("storage failure was not closed and redacted")
	}
}

func TestInitialSetupDefaultsToOptionalMFAWithoutRevealingSecrets(t *testing.T) {
	store := &promptStore{}
	var output bytes.Buffer
	prompts := 0
	read := func(prompt string) (string, error) {
		prompts++
		if prompts == 1 {
			return "Synthetic maker", nil
		}
		return "", nil
	}
	if err := completeSetup(store, read, &output, "owner@example.com", "Synthetic fixture passphrase", time.Second); err != nil {
		t.Fatal(err)
	}
	if prompts != 2 || len(store.contexts) != 2 || strings.Contains(output.String(), "Private enrollment key") || strings.Contains(output.String(), "recovery codes") {
		t.Fatal("skip unexpectedly enrolled or revealed factors")
	}
	if !strings.Contains(output.String(), "enable it later") {
		t.Fatal("recommendation missing")
	}
}

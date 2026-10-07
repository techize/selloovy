// owner-setup is an interactive operator command, never an HTTP registration route.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/config"
	"github.com/techize/selloovy/internal/database"
	"golang.org/x/term"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 1 || !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("Owner setup requires an interactive terminal and accepts no arguments")
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	if cfg.AuthKeyFile == "" {
		return errors.New("Configure the database, authentication key file and public origin first")
	}
	key, err := auth.LoadKeyFile(cfg.AuthKeyFile)
	if err != nil {
		return err
	}
	defer clear(key)
	vault, err := auth.NewMFAVault(key)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = database.Ready(ctx, pool); err != nil {
		return err
	}
	store, err := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if err != nil {
		return err
	}
	cancel() // Do not count time spent at interactive prompts against database work.
	reader := bufio.NewReader(os.Stdin)
	read := func(prompt string) (string, error) {
		fmt.Print(prompt)
		value, e := reader.ReadString('\n')
		return strings.TrimSpace(value), e
	}
	email, err := read("Owner email: ")
	if err != nil {
		return errors.New("Could not read owner email")
	}
	fmt.Print("Password (15–128 characters; hidden): ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return errors.New("Could not read password")
	}
	password := string(pw)
	clear(pw)
	defer func() { password = "" }()
	err = completeSetup(store, read, os.Stdout, email, password, 30*time.Second)
	if err == nil {
		fmt.Println("Sign in at", cfg.PublicOrigin+"/admin/")
	}
	return err
}

type setupStore interface {
	ResumeEnrollment(context.Context, string, string) (int64, auth.MFASecret, error)
	CreateFirstOwner(context.Context, string, string, string) (int64, auth.MFASecret, error)
	ConfirmEnrollment(context.Context, int64, string) ([]auth.RecoveryCode, error)
}

// Each operation starts its deadline after input is collected. Human interaction
// has no shared timeout; database work remains bounded.
func completeSetup(store setupStore, read func(string) (string, error), output io.Writer, email, password string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	id, secret, err := store.ResumeEnrollment(ctx, email, password)
	cancel()
	if errors.Is(err, auth.ErrCredential) {
		name, e := read("Shop name (new installation only): ")
		if e != nil {
			return errors.New("Could not read shop name")
		}
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		id, secret, err = store.CreateFirstOwner(ctx, name, email, password)
		cancel()
	}
	if err != nil {
		return err
	}
	password = ""
	fmt.Fprintln(output, "Add an authenticator account: issuer Selloovy, time-based, six digits, SHA1, 30 seconds.")
	fmt.Fprintln(output, "Private enrollment key:", secret.EnrollmentKey())
	var codes []auth.RecoveryCode
	for range 5 {
		code, e := read("Current authenticator code: ")
		if e != nil {
			return errors.New("Could not read authenticator code")
		}
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		codes, err = store.ConfirmEnrollment(ctx, id, code)
		cancel()
		if err == nil {
			break
		}
		if !errors.Is(err, auth.ErrCredential) {
			return err
		}
		fmt.Fprintln(output, "Code did not match. Check your device clock and try again.")
	}
	if err != nil {
		return errors.New("Enrollment not completed; rerun this command to resume")
	}
	fmt.Fprintln(output, "Store these one-use recovery codes privately. They require your password:")
	for _, code := range codes {
		fmt.Fprintln(output, code.Reveal())
	}
	fmt.Fprintln(output, "Owner enrolled.")
	return nil
}

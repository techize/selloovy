// owner-setup is an interactive operator command, never an HTTP registration route.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
	id, secret, err := store.ResumeEnrollment(ctx, email, password)
	if errors.Is(err, auth.ErrCredential) {
		name, e := read("Shop name (new installation only): ")
		if e != nil {
			return errors.New("Could not read shop name")
		}
		id, secret, err = store.CreateFirstOwner(ctx, name, email, password)
	}
	if err != nil {
		return err
	}
	password = ""
	fmt.Println("Add an authenticator account: issuer Selloovy, time-based, six digits, SHA1, 30 seconds.")
	fmt.Println("Private enrollment key:", secret.EnrollmentKey())
	var codes []auth.RecoveryCode
	for range 5 {
		code, e := read("Current authenticator code: ")
		if e != nil {
			return errors.New("Could not read authenticator code")
		}
		codes, err = store.ConfirmEnrollment(ctx, id, code)
		if err == nil {
			break
		}
		if !errors.Is(err, auth.ErrCredential) {
			return err
		}
		fmt.Println("Code did not match. Check your device clock and try again.")
	}
	if err != nil {
		return errors.New("Enrollment not completed; rerun this command to resume")
	}
	fmt.Println("Store these one-use recovery codes privately. They require your password:")
	for _, code := range codes {
		fmt.Println(code.Reveal())
	}
	fmt.Println("Owner enrolled. Sign in at", cfg.PublicOrigin+"/admin/")
	return nil
}

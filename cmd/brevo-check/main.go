// brevo-check reads the selected sender status; it never sends an email.
package main

import (
	"context"
	"fmt"
	"github.com/techize/selloovy/internal/brevo"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "Use SELLOOVY_BREVO_API_KEY_FILE and SELLOOVY_BREVO_SENDER_EMAIL; this command takes no arguments.")
		os.Exit(1)
	}
	c, e := brevo.New(os.Getenv("SELLOOVY_BREVO_API_KEY_FILE"), os.Getenv("SELLOOVY_BREVO_SENDER_EMAIL"), "Selloovy", brevo.Drop)
	if e == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		e = c.VerifySender(ctx)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	fmt.Println("Brevo API access and selected active sender verified. No email sent; domain authentication and inbox delivery still need verification.")
}

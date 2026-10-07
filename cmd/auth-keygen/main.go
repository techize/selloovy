// auth-keygen writes a new installation key without printing its contents.
package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: auth-keygen KEY_FILE (parent directory must exist)")
		os.Exit(1)
	}
	if err := createKey(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Authentication key created. Keep it private and back it up separately.")
}
func createKey(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("key file cannot be created; existing files are never overwritten")
	}
	var key [32]byte
	_, _ = rand.Read(key[:])
	_, err = file.Write(key[:])
	clear(key[:])
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(path)
		return errors.New("key file could not be saved")
	}
	return nil
}

package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKeyFileBoundaries(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "vault.key")
	if err := os.WriteFile(path, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKeyFile(path); err != nil {
		t.Fatal("private key refused")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKeyFile(path); err == nil {
		t.Fatal("public-readable key accepted")
	}
	_ = os.Chmod(path, 0600)
	link := filepath.Join(root, "linked.key")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKeyFile(link); err == nil {
		t.Fatal("symlink accepted")
	}
	_ = os.WriteFile(path, make([]byte, 31), 0600)
	if _, err := LoadKeyFile(path); err == nil {
		t.Fatal("wrong length accepted")
	}
	if _, err := LoadKeyFile(root); err == nil {
		t.Fatal("directory accepted")
	}
}

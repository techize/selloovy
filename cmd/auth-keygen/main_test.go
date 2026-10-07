package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestKeyProvisioningDoesNotReplaceExistingMaterial(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "installation.key")
	if err := createKey(path); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil || len(first) != 32 || bytes.Equal(first, make([]byte, 32)) {
		t.Fatal("invalid generated key")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("key is not private")
	}
	if err := createKey(path); err == nil {
		t.Fatal("existing key overwritten")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(first, after) {
		t.Fatal("existing key changed")
	}
	link := filepath.Join(root, "link.key")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := createKey(link); err == nil {
		t.Fatal("symlink followed")
	}
}

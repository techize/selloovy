package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLicenceExceptionDoesNotPermitChangedOrMovedContent(t *testing.T) {
	tool := filepath.Join(t.TempDir(), "publicguard")
	if err := exec.Command("go", "build", "-o", tool, "publicguard.go").Run(); err != nil {
		t.Fatal("could not build publication guard")
	}
	repo := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if err := cmd.Run(); err != nil {
			t.Fatal("could not prepare synthetic Git fixture")
		}
	}
	runGit("init", "--quiet")
	name := "third_party/npm_postcss.LICENSE"
	content, err := os.ReadFile(filepath.Join("..", name))
	if err != nil {
		t.Fatal("reviewed licence fixture missing")
	}
	if err := os.MkdirAll(filepath.Join(repo, "third_party"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(repo, name)
	if err := os.WriteFile(file, content, 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", name)
	check := func(pass bool, args ...string) {
		cmd := exec.Command(tool, args...)
		cmd.Dir = repo
		if (cmd.Run() == nil) != pass {
			t.Fatalf("unexpected publication result for %v", args)
		}
	}
	check(true)
	check(true, "--staged")
	// Construct a synthetic unapproved address without publishing an email literal.
	changed := append(append([]byte{}, content...), []byte("\nfixture"+"@"+"person.example\n")...)
	if err := os.WriteFile(file, changed, 0600); err != nil {
		t.Fatal(err)
	}
	check(false)
	check(true, "--staged") // The clean index must be checked independently.
	runGit("add", name)
	check(false, "--staged")
	if err := os.WriteFile(file, content, 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", name)
	if err := os.WriteFile(filepath.Join(repo, "copied.LICENSE"), content, 0600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "copied.LICENSE")
	check(false, "--staged") // Identical bytes at another path do not qualify.
}

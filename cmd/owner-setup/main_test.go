package main

import (
	"os"
	"strings"
	"testing"
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

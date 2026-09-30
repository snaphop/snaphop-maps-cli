package main

import (
	"os"
	"testing"
)

// TestMain runs the real entry point with the real environment, as a shell would, and checks the
// exit status it ends with.
func TestMainExitsWithTheStatus(t *testing.T) {
	arguments, ended := os.Args, exit
	defer func() { os.Args, exit = arguments, ended }()
	code := -1
	exit = func(status int) { code = status }
	for args, want := range map[string]int{"version": 0, "frobnicate": 2} {
		os.Args = []string{"snaphop-maps", args}
		main()
		if code != want {
			t.Fatalf("snaphop-maps %s exited %d, want %d", args, code, want)
		}
	}
}

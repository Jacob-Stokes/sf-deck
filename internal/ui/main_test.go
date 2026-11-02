package ui

import (
	"fmt"
	"os"
	"testing"
)

// UI tests must never open the developer's real settings or databases.
func TestMain(m *testing.M) {
	testHome, err := os.MkdirTemp("", "sf-deck-ui-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, name := range []string{"HOME", "USERPROFILE"} {
		if err := os.Setenv(name, testHome); err != nil {
			fmt.Fprintln(os.Stderr, err)
			_ = os.RemoveAll(testHome)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(testHome)
	os.Exit(code)
}

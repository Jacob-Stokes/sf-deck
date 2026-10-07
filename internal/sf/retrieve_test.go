package sf

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunSFInDirWithTimeoutKillsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake sf not supported on Windows")
	}
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available on PATH")
	}

	binDir := t.TempDir()
	sfPath := filepath.Join(binDir, "sf")
	if err := os.WriteFile(sfPath, []byte("#!/bin/sh\nsleep 10\n"), 0o755); err != nil {
		t.Fatalf("write fake sf: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	start := time.Now()
	_, err := runSFInDirWithTimeout(t.TempDir(), "org@example.com", 50*time.Millisecond, "project", "deploy", "start")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > time.Second {
		t.Fatalf("expected timeout to kill process quickly, took %v", elapsed)
	}
	if got := err.Error(); !strings.Contains(got, "sf project deploy start timed out after 50ms") {
		t.Fatalf("timeout error = %q", got)
	}
}
